// Command server is the Orbit API.
//
// It serves the /api/v1 REST surface and the embedded Vue SPA from a single
// binary, on plain HTTP (TLS terminated upstream) or HTTPS with TLS_CERT_FILE /
// TLS_KEY_FILE.
//
//	@title			Orbit API
//	@version		0.4.0
//	@description	Workspaces for school, work and life: pages, tasks, boards and calendars.
//	@BasePath		/api/v1
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				"Bearer <access_token>"
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/api"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/audit"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/backup"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/caldav"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/db"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/gitsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/jobs"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/tlsfiles"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// healthcheck probes the local API's liveness endpoint and exits non-zero on
// failure. It targets the configured HTTP_ADDR port on the loopback interface.
func healthcheck() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	port := addr[strings.LastIndex(addr, ":")+1:]
	// With in-app TLS the listener only speaks HTTPS; the probe is to this
	// container's own loopback, so the certificate (issued for the public
	// name) isn't checked.
	scheme := "http"
	if os.Getenv("TLS_CERT_FILE") != "" && os.Getenv("TLS_KEY_FILE") != "" {
		scheme = "https"
	}
	os.Exit(probeHealth(scheme + "://127.0.0.1:" + port + "/api/v1/healthz"))
}

// probeHealth GETs url and returns a process exit code: 0 when the endpoint
// answers 200, 1 otherwise. Split out from healthcheck so it's testable.
func probeHealth(url string) int {
	client := &http.Client{
		Timeout: 3 * time.Second,
		// Loopback self-check only (see healthcheck): skip certificate checks.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // loopback health probe
	}
	resp, err := client.Get(url)
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// restoreBackup writes the database of an encrypted backup to DB_URL. The
// passphrase (passphrase archives) comes from BACKUP_PASSPHRASE (or
// BACKUP_PASSPHRASE_FILE); ORBIT_MASTER_KEY opens master-key archives.
func restoreBackup(path string, force bool) int {
	archive, err := os.ReadFile(filepath.Clean(path)) // the operator's own file
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: restore:", err)
		return 1
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: configuration error:", err)
		return 1
	}
	if cfg.DBDriver != "sqlite" {
		fmt.Fprintln(os.Stderr, "orbit: restore: the backups here are of SQLite; restore PostgreSQL with its own tools (pg_restore)")
		return 1
	}
	pass := os.Getenv("BACKUP_PASSPHRASE")
	if f := os.Getenv("BACKUP_PASSPHRASE_FILE"); pass == "" && f != "" {
		b, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			fmt.Fprintln(os.Stderr, "orbit: restore: BACKUP_PASSPHRASE_FILE:", err)
			return 1
		}
		pass = strings.TrimRight(string(b), "\r\n")
	}
	env, kept, err := backup.Restore(archive, pass, cfg.MasterKey, cfg.DBURL, force, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: restore:", err)
		return 1
	}
	fmt.Printf("orbit: restored the %s backup of %s (Orbit %s) to %s\n", env.Mode, env.CreatedAt, env.AppVersion, cfg.DBURL)
	if kept != "" {
		fmt.Printf("orbit: the previous database is kept as %s\n", kept)
	}
	if env.NeedsPassphrase() {
		fmt.Println("orbit: secrets inside (two-factor, SSO and notification secrets, calendar addresses) open only with the ORBIT_MASTER_KEY they were sealed with")
	}
	fmt.Println("orbit: start Orbit now")
	return 0
}

// storeOf is the file store of a kind (db, dir or s3), from the configuration.
func storeOf(cfg *config.Config, kind string) (uploads.Store, error) {
	switch kind {
	case uploads.StoreDB:
		return uploads.DBStore{}, nil
	case uploads.StoreDir:
		return uploads.DirStore{Root: cfg.UploadDir}, nil
	case uploads.StoreS3:
		return uploads.NewS3(uploads.S3Config{
			Bucket: cfg.UploadS3Bucket, Prefix: cfg.UploadS3Prefix, Endpoint: cfg.UploadS3Endpoint, Region: cfg.UploadS3Region,
			AccessKey: cfg.UploadS3Access, SecretKey: cfg.UploadS3Secret, Insecure: cfg.UploadS3Insecure,
		})
	}
	return nil, fmt.Errorf("unknown file store %q (db, dir or s3)", kind)
}

// moveFiles copies every file's bytes from where they're kept now to
// another store (`orbit move-files <db|dir|s3> [--clean]`), with Orbit
// stopped; --clean then removes the old copies.
func moveFiles(to string, clean bool) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: configuration error:", err)
		return 1
	}
	gdb, err := db.Open(cfg)
	if err == nil {
		err = db.Migrate(gdb)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: move-files: database:", err)
		return 1
	}
	from := uploads.Recorded(gdb)
	if from == "" {
		from = uploads.StoreDB
	}
	if from == to {
		fmt.Printf("orbit: the files are already kept in %q\n", to)
		return 0
	}
	src, err := storeOf(cfg, from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: move-files: from:", err)
		return 1
	}
	dst, err := storeOf(cfg, to)
	if err != nil {
		fmt.Fprintln(os.Stderr, "orbit: move-files: to:", err)
		return 1
	}
	// Bytes still kept the old way go to the old store first.
	uploads.Use(src)
	if err := uploads.Migrate(gdb, nil); err != nil {
		fmt.Fprintln(os.Stderr, "orbit: move-files:", err)
		return 1
	}
	fmt.Printf("orbit: moving the files from %q to %q…\n", from, to)
	if err := uploads.Move(gdb, src, dst, clean, func(n int) { fmt.Printf("orbit: %d files\n", n) }); err != nil {
		fmt.Fprintln(os.Stderr, "orbit: move-files:", err)
		return 1
	}
	if clean && from == uploads.StoreDB && cfg.DBDriver == "sqlite" {
		_ = gdb.Exec("VACUUM").Error // give the space back
	}
	fmt.Printf("orbit: done; set UPLOAD_STORE=%s and start Orbit\n", to)
	if !clean {
		fmt.Printf("orbit: the copies in %q are kept; run again with --clean to remove them\n", from)
	}
	return 0
}

func main() {
	// `orbit healthcheck` is an in-container liveness probe used by the
	// image HEALTHCHECK. The runtime image is distroless (no shell or curl), so
	// the probe lives in the binary itself.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck()
		return
	}
	// `orbit restore-backup <archive> [--force]` writes the database inside
	// an encrypted backup to DB_URL, with Orbit stopped, then exits.
	if len(os.Args) > 2 && os.Args[1] == "restore-backup" {
		os.Exit(restoreBackup(os.Args[2], len(os.Args) > 3 && os.Args[3] == "--force"))
	}
	// `orbit move-files <db|dir|s3> [--clean]` moves the files' bytes to
	// another store, with Orbit stopped, then exits.
	if len(os.Args) > 2 && os.Args[1] == "move-files" {
		os.Exit(moveFiles(os.Args[2], len(os.Args) > 3 && os.Args[3] == "--clean"))
	}

	cfg, err := config.Load()
	if err != nil {
		// A config error is the operator's to fix (e.g. a missing or short
		// secret): say what's wrong, no stack trace.
		fmt.Fprintln(os.Stderr, "orbit: configuration error:", err)
		os.Exit(1)
	}

	log := newLogger(cfg.GinMode)
	defer func() { _ = log.Sync() }()

	gdb, err := db.Open(cfg)
	if err != nil {
		log.Fatal("database connection failed", zap.Error(err))
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatal("migration failed", zap.Error(err))
	}
	log.Info("database ready", zap.String("driver", cfg.DBDriver))

	// Where files' bytes are kept; the one recorded must be the one set, and
	// bytes kept the old way (a row per file) move into it, once.
	store, err := storeOf(cfg, cfg.UploadStore)
	if err != nil {
		log.Fatal("file store", zap.Error(err))
	}
	uploads.Use(store)
	if err := uploads.Check(gdb); err != nil {
		log.Fatal("file store", zap.Error(err))
	}
	if err := uploads.Migrate(gdb, func(n int) { log.Info("files moved to the store", zap.Int("files", n)) }); err != nil {
		log.Fatal("moving files to the store", zap.Error(err))
	}
	log.Info("files kept", zap.String("store", store.Kind()))
	// What files from before search read them say (once).
	if err := uploads.FillTexts(gdb); err != nil {
		log.Warn("reading the files' text failed", zap.Error(err))
	}

	// Keystore (seals secrets at rest: TOTP secrets, the OIDC client secret,
	// calendar subscription addresses). The KEK is derived with Argon2id over a stored salt.
	sealer, err := db.OpenSealer(gdb, cfg.MasterKey)
	if err != nil {
		log.Fatal("keystore init failed", zap.Error(err))
	}

	// Auth + audit wiring.
	authMgr := auth.NewManager(gdb, cfg.JWTSecret, cfg.LoginLockoutThreshold, cfg.LoginLockoutWindow)
	auditRec := audit.New(gdb, log)
	if cfg.SyslogAddr != "" {
		if err := auditRec.ForwardToSyslog(cfg.SyslogAddr); err != nil {
			log.Fatal("audit forwarding", zap.Error(err))
		}
		log.Info("audit entries also go to syslog", zap.String("addr", cfg.SyslogAddr))
	}
	// Establish the tamper-evidence hash chain over any pre-chain audit rows
	// (one-time on upgrade; idempotent).
	if err := auditRec.BackfillChain(); err != nil {
		log.Warn("audit chain backfill failed", zap.Error(err))
	}
	updates := version.NewChecker(cfg.RepoURL, cfg.UpdateCheck)
	notifier := notify.New(gdb, log).UseSealer(sealer)
	// The integration webhook carries the audit events (accounts, roles,
	// workspaces and their members, settings) — never workspace content,
	// which is private to its members.
	auditRec.OnRecord(func(e models.AuditLog) {
		notifier.FireWebhook("audit."+e.Action, map[string]any{
			"id": e.ID, "action": e.Action, "user_id": e.UserID, "impersonator_id": e.ImpersonatorID,
			"target_type": e.TargetType, "target_id": e.TargetID, "ip": e.IP, "detail": json.RawMessage(orJSON(e.Detail)),
		})
	})
	if err := notifier.SealStored(); err != nil {
		log.Warn("sealing the stored notification configuration failed", zap.Error(err))
	}
	oidcSvc := auth.NewOIDC(gdb, sealer)
	waMgr, err := auth.NewWebAuthn(gdb, cfg)
	if err != nil {
		log.Fatal("webauthn init failed", zap.Error(err))
	}

	// Seed built-in roles, then bootstrap the initial admin (first run only).
	if err := rbac.SeedBuiltins(gdb); err != nil {
		log.Fatal("seeding built-in roles failed", zap.Error(err))
	}
	if err := auth.Bootstrap(gdb, log, cfg.AdminUser, cfg.AdminPassword); err != nil {
		log.Warn("admin bootstrap skipped", zap.Error(err))
	}
	// Ensure an instance owner exists (backfills pre-owner installs).
	if err := auth.EnsureOwner(gdb, log); err != nil {
		log.Warn("ensure-owner failed", zap.Error(err))
	}

	// Record the running version in the install history.
	recordVersionHistory(gdb, log)

	hub := events.New().UseTicketStore(ephemeral.New(gdb))
	// Notifications: the inbox, Web Push and the user's channels.
	notes := inbox.New(gdb, hub, notifier, sealer, log, cfg.PublicURL)
	// Git: lists that hold a repository's issues, kept in step both ways.
	git := gitsync.New(gdb, sealer, cfg.GitServers, log)
	git.NotifyHub(hub)
	// CalDAV: workspace calendars kept in step with iCloud / Nextcloud ones.
	dav := caldav.New(gdb, sealer, log).NotifyHub(hub)
	router := api.New(api.Deps{
		Cfg:      cfg,
		DB:       gdb,
		Log:      log,
		Auth:     authMgr,
		Audit:    auditRec,
		Updates:  updates,
		Notify:   notifier,
		Sealer:   sealer,
		OIDC:     oidcSvc,
		WebAuthn: waMgr,
		Events:   hub,
		Inbox:    notes,
		Git:      git,
		CalDAV:   dav,
	})

	// Background scheduler: update checks, token sweeps, backups, calendar
	// subscriptions and reminders.
	scheduler := jobs.NewScheduler(gdb, notifier, updates, log).UseBackups(sealer).UsePlanner(sealer, hub).UseInbox(notes).UseGit(git).UseCalDAV(dav)
	scheduler.Start()
	defer scheduler.Stop()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout bounds slow-body uploads; IdleTimeout reaps idle keep-alive
		// connections. No WriteTimeout: the /auth/events SSE stream is long-lived.
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	// Start server. If TLS_CERT_FILE / TLS_KEY_FILE are set the app terminates
	// HTTPS itself (run with nothing in front, or to encrypt the proxy→app hop);
	// otherwise it serves plain HTTP and TLS is terminated upstream.
	tlsEnabled := cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if (cfg.TLSCertFile != "") != (cfg.TLSKeyFile != "") {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	if tlsEnabled {
		// The certificate is loaded again when its files change, so a renewal
		// needs no restart.
		certs, err := tlsfiles.NewLoader(cfg.TLSCertFile, cfg.TLSKeyFile, "", "")
		if err != nil {
			log.Fatal("TLS certificate", zap.Error(err))
		}
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: certs.ServerCertificate}
	}
	go func() {
		log.Info("listening", zap.String("addr", cfg.HTTPAddr), zap.Bool("tls", tlsEnabled))
		var err error
		if tlsEnabled {
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}
}

// recordVersionHistory appends the running version to the install history in
// the settings table when it differs from the last recorded version.
func recordVersionHistory(gdb *gorm.DB, log *zap.Logger) {
	const key = "version_history"
	type entry struct {
		Version     string `json:"version"`
		InstalledAt string `json:"installed_at"`
	}
	var row models.Setting
	hist := []entry{}
	if err := gdb.Where(&models.Setting{Key: key}).First(&row).Error; err == nil {
		_ = json.Unmarshal([]byte(row.Value), &hist)
	}
	if len(hist) > 0 && hist[len(hist)-1].Version == version.Version {
		return // already recorded
	}
	hist = append(hist, entry{Version: version.Version, InstalledAt: time.Now().UTC().Format(time.RFC3339)})
	b, _ := json.Marshal(hist)
	if err := gdb.Save(&models.Setting{Key: key, Value: string(b), UpdatedAt: time.Now()}).Error; err != nil {
		log.Warn("could not record version history", zap.Error(err))
	}
}

// orJSON returns s, or "null" when empty (for embedding as raw JSON).
func orJSON(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func newLogger(mode string) *zap.Logger {
	if mode == "release" {
		l, _ := zap.NewProduction()
		return l
	}
	l, _ := zap.NewDevelopment()
	return l
}

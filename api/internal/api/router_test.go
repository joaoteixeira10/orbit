package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/linkpreview"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/audit"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/calsync"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	dbpkg "gitea.ubonito.org/jteixeira/orbit/api/internal/db"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/version"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestRouter wires the full engine over an in-memory DB with a seeded admin,
// so tests exercise the real middleware + auth path end to end.
func newTestRouter(t *testing.T) *gin.Engine { return newTestRouterCfg(t, false) }

func newTestRouterCfg(t *testing.T, metrics bool) *gin.Engine {
	t.Helper()
	r, _ := newTestRouterDB(t, metrics)
	return r
}

// newTestRouterDB is newTestRouterCfg that also returns the backing DB, for
// tests that need to seed or inspect state directly.
func newTestRouterDB(t *testing.T, metrics bool) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	log := zap.NewNop()
	cfg := &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000, MetricsEnabled: metrics}
	if err := rbac.SeedBuiltins(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := auth.Bootstrap(db, log, "admin", "adminpass"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	// The bootstrap admin must change its password at first sign-in; tests
	// that aren't about that start from an admin who already did.
	db.Model(&models.User{}).Where("username = ?", "admin").Update("must_change_password", false)
	return routerOn(t, db, cfg, nil), db
}

// routerOn builds the engine over an existing test database (same secrets,
// so tokens issued by another routerOn over it stay valid). syncer nil = the
// guarded default.
func routerOn(t *testing.T, db *gorm.DB, cfg *config.Config, syncer *calsync.Syncer) *gin.Engine {
	t.Helper()
	log := zap.NewNop()
	sealer, err := dbpkg.OpenSealer(db, "test-master-key-0123456789-abcdef")
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	wa, _ := auth.NewWebAuthn(db, cfg)
	return New(Deps{
		Cfg: cfg, DB: db, Log: log,
		CalSync:  syncer,
		OIDC:     auth.NewOIDC(db, sealer),
		Auth:     auth.NewManager(db, "test-jwt-secret-0123456789-abcdef", 5, 15*time.Minute),
		Audit:    audit.New(db, log),
		Updates:  version.NewChecker("", true),
		Notify:   notify.New(db, log),
		Sealer:   sealer,
		WebAuthn: wa,
		Events:   events.New(),
		Links:    testLinks,
	})
}

// testLinks, when set, reads bookmark previews (tests reach httptest on
// loopback, which the guarded fetcher refuses).
var testLinks *linkpreview.Fetcher

// routerWithOpenClient is the test instance over db with a calendar syncer
// that may reach loopback (httptest servers).
func routerWithOpenClient(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	sealer, err := dbpkg.OpenSealer(db, "test-master-key-0123456789-abcdef")
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	syncer := calsync.New(db, sealer, zap.NewNop()).UseClient(&http.Client{Timeout: 5 * time.Second})
	return routerOn(t, db, &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000}, syncer)
}

func do(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	return w
}

func TestHealthAndReadyz(t *testing.T) {
	r := newTestRouter(t)
	if w := do(r, "GET", "/api/v1/healthz", ""); w.Code != 200 {
		t.Errorf("healthz: got %d", w.Code)
	}
	if w := do(r, "GET", "/api/v1/readyz", ""); w.Code != 200 {
		t.Errorf("readyz (db up): got %d", w.Code)
	}
}

func TestSecurityHeadersAndRequestID(t *testing.T) {
	r := newTestRouter(t)
	w := do(r, "GET", "/api/v1/healthz", "")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff header")
	}
	if w.Header().Get("X-Request-Id") == "" {
		t.Error("missing X-Request-Id header")
	}
}

func TestLoginFlow(t *testing.T) {
	r := newTestRouter(t)

	ok := do(r, "POST", "/api/v1/auth/login", `{"username":"admin","password":"adminpass"}`)
	if ok.Code != 200 {
		t.Fatalf("valid login: got %d, body=%s", ok.Code, ok.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(ok.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Fatal("expected an access token on successful login")
	}

	bad := do(r, "POST", "/api/v1/auth/login", `{"username":"admin","password":"wrong"}`)
	if bad.Code != 401 {
		t.Errorf("invalid login: got %d, want 401", bad.Code)
	}
}

func TestMetricsToggle(t *testing.T) {
	// Disabled: /metrics is not registered, so it falls through to the SPA
	// (HTML), never the Prometheus exposition.
	off := do(newTestRouter(t), "GET", "/metrics", "")
	if strings.Contains(off.Body.String(), "go_goroutines") {
		t.Error("metrics must not be exposed when METRICS_ENABLED is false")
	}
	// Enabled: real Prometheus output.
	on := do(newTestRouterCfg(t, true), "GET", "/metrics", "")
	if on.Code != 200 || !strings.Contains(on.Body.String(), "go_goroutines") {
		t.Errorf("metrics should be exposed when enabled: code=%d", on.Code)
	}
}

// openTestDB is an empty database for one test: SQLite in memory, or — with
// ORBIT_TEST_POSTGRES set to a connection string — a schema of its own on
// that PostgreSQL (dropped after), so the API's tests check both engines.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if dsn := os.Getenv("ORBIT_TEST_POSTGRES"); dsn != "" {
		quiet := &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
		admin, err := gorm.Open(postgres.Open(dsn), quiet)
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		schema := "t_" + hex.EncodeToString(b)
		if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatalf("schema: %v", err)
		}
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		db, err := gorm.Open(postgres.Open(dsn+sep+"search_path="+schema), quiet)
		if err != nil {
			t.Fatalf("open schema: %v", err)
		}
		t.Cleanup(func() {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
			admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			if sqlDB, err := admin.DB(); err == nil {
				_ = sqlDB.Close()
			}
		})
		return db
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Each connection to ":memory:" is its own empty database: keep one, so
	// every request sees the same data.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return db
}

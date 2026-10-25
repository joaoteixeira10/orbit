// Package config loads runtime configuration from environment variables.
//
// Values come from the environment (secrets through the secrets provider), so
// the same binary runs unchanged in dev and prod — see .env.example for the
// full list. Runtime policy that admins change lives in the settings table.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/secrets"
)

// minSecretLen is the minimum length enforced for JWT_SECRET and
// ORBIT_MASTER_KEY in release mode (32 bytes ≈ a 256-bit random secret).
const minSecretLen = 32

// Config is the fully-resolved runtime configuration.
type Config struct {
	// HTTP
	HTTPAddr string // listen address, e.g. ":8080"
	GinMode  string // "debug" | "release" | "test"

	// Database
	DBDriver string // "sqlite" (default) | "postgres"
	DBURL    string // file path

	// Auth / crypto
	JWTSecret string // HMAC secret for signing JWTs
	MasterKey string // KEK used to seal secrets at rest (TOTP, OIDC client secret, calendar addresses; AES-256-GCM)

	// Bootstrap admin (first run only)
	AdminUser     string
	AdminPassword string

	// CORS (dev convenience; in prod the SPA is served same-origin)
	CORSOrigins []string

	// TrustedProxies is the CIDR allowlist of reverse proxies whose
	// X-Forwarded-For is trusted for client-IP resolution (rate limiting +
	// audit). Empty (the default) trusts NO proxy — ClientIP() is the socket
	// peer, so a spoofed XFF can't forge the client IP. Behind a proxy, set this
	// to the proxy's address/CIDR(s).
	TrustedProxies []string // TRUSTED_PROXIES (comma CIDR/IP list)

	// MaxBodyBytes caps the request body size (bytes) for all routes except the
	// backup restore (which keeps its own larger limit). Guards against
	// memory-exhaustion via oversized JSON/data-URL payloads.
	MaxBodyBytes int64 // MAX_BODY_BYTES

	// Uploads (images and files in pages, notes and tasks) are stored in the
	// database. UploadMaxBytes caps one file, UploadQuotaBytes all of them
	// (0 = no limit).
	UploadMaxBytes   int64 // UPLOAD_MAX_MB
	UploadQuotaBytes int64 // UPLOAD_QUOTA_MB
	// ImportMaxBytes caps a workspace export being imported (files included).
	ImportMaxBytes int64 // IMPORT_MAX_MB

	// WebAuthn / passkeys. RPID must be a registrable-domain suffix of every
	// origin's host (exactly the public hostname behind the TLS proxy in prod;
	// "localhost" in dev). Empty RPID disables passkeys (handlers return 501).
	WebAuthnRPID          string   // WEBAUTHN_RP_ID
	WebAuthnRPOrigins     []string // WEBAUTHN_RP_ORIGINS (comma list, full scheme+host)
	WebAuthnRPDisplayName string   // WEBAUTHN_RP_DISPLAY_NAME

	// Auth brute-force throttle (per client IP). Burst is the bucket size;
	// PerMinute is the steady refill rate.
	AuthRateBurst     int // AUTH_RATE_BURST
	AuthRatePerMinute int // AUTH_RATE_PER_MINUTE

	// Per-account brute-force lockout: after Threshold consecutive failed
	// password/2FA attempts the account is locked for Window. Threshold <= 0
	// disables the lockout (the per-IP throttle still applies). It's a temporary
	// timed lock (not permanent) to limit lockout-as-DoS.
	LoginLockoutThreshold int           // LOGIN_LOCKOUT_THRESHOLD
	LoginLockoutWindow    time.Duration // LOGIN_LOCKOUT_MINUTES

	// MetricsEnabled exposes a Prometheus /metrics endpoint when true.
	MetricsEnabled bool // METRICS_ENABLED

	// SyslogAddr, when set, also forwards every audit entry as JSON to syslog:
	// "host:514" (UDP; the port defaults to 514) or a socket path like /dev/log.
	SyslogAddr string // SYSLOG_ADDR

	// PublicURL is the address people use to reach Orbit (scheme + host, no
	// trailing slash). Used to build calendar-feed addresses; empty = taken
	// from the request.
	PublicURL string // PUBLIC_URL
	// GitServers are the git servers (Gitea / Forgejo base URLs) users may
	// connect even when they resolve to a private address — your own server
	// on the LAN. Any other server must be public (https, not private IPs).
	GitServers []string // GIT_SERVERS (comma list of base URLs)
	// DrawioURL is where the draw.io editor for Diagrams is loaded from:
	// draw.io's own (the default), or your own (the jgraph/drawio container).
	// An admin can change it in Settings.
	DrawioURL string // DRAWIO_URL

	// Optional app-terminated HTTPS for the MAIN listener. When both are set the
	// app serves HTTPS on HTTPAddr itself (run with nothing in front, or encrypt
	// the proxy→app hop); otherwise it serves plain HTTP and TLS is terminated
	// upstream.
	TLSCertFile string // TLS_CERT_FILE
	TLSKeyFile  string // TLS_KEY_FILE

	// CookieSecure marks auth cookies Secure (TLS-only). Defaults to true in
	// release or when the app terminates TLS; override with COOKIE_SECURE (e.g.
	// set false for a plain-HTTP dev/test box, or true behind a TLS proxy in debug).
	CookieSecure bool // COOKIE_SECURE

	// CookieDomain, when set, scopes the auth cookies to a parent domain (e.g.
	// "example.org") so they are shared across subdomains. Empty = host-only
	// (the default; correct for a single-host deployment).
	CookieDomain string // COOKIE_DOMAIN

	// RepoURL overrides the repository used for the About link + update check
	// (empty = the built-in default). UpdateCheck enables/disables the periodic
	// "newer release available?" check (default on).
	RepoURL     string // REPO_URL
	UpdateCheck bool   // UPDATE_CHECK
}

// Load reads configuration from the environment, applying defaults.
// It returns an error if a required secret is missing in release mode.
func Load() (*Config, error) {
	// Sensitive values resolve through the configured secrets backend (env by
	// default; file/Vault optional). Non-secret config stays in the environment.
	prov, err := secrets.New(os.Getenv("SECRETS_BACKEND"))
	if err != nil {
		return nil, err
	}
	jwtSecret, err := prov.Get("JWT_SECRET")
	if err != nil {
		return nil, err
	}
	masterKey, err := prov.Get("ORBIT_MASTER_KEY")
	if err != nil {
		return nil, err
	}
	dbURL, err := prov.Get("DB_URL")
	if err != nil {
		return nil, err
	}
	adminPassword, err := prov.Get("ADMIN_PASSWORD")
	if err != nil {
		return nil, err
	}
	if dbURL == "" {
		dbURL = "orbit.db"
	}
	c := &Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		GinMode:          env("GIN_MODE", "debug"),
		DBDriver:         env("DB_DRIVER", "sqlite"),
		DBURL:            dbURL,
		JWTSecret:        jwtSecret,
		MasterKey:        masterKey,
		AdminUser:        env("ADMIN_USER", "admin"),
		AdminPassword:    adminPassword,
		CORSOrigins:      splitNonEmpty(env("CORS_ORIGINS", "")),
		TrustedProxies:   splitNonEmpty(env("TRUSTED_PROXIES", "")),
		GitServers:       splitNonEmpty(env("GIT_SERVERS", "")),
		DrawioURL:        env("DRAWIO_URL", "https://embed.diagrams.net"),
		MaxBodyBytes:     int64(envInt("MAX_BODY_BYTES", 4<<20)), // 4 MiB
		UploadMaxBytes:   int64(envInt("UPLOAD_MAX_MB", 25)) << 20,
		UploadQuotaBytes: int64(envInt("UPLOAD_QUOTA_MB", 2048)) << 20,
		ImportMaxBytes:   int64(envInt("IMPORT_MAX_MB", 512)) << 20,

		WebAuthnRPID:          env("WEBAUTHN_RP_ID", ""),
		WebAuthnRPOrigins:     splitNonEmpty(env("WEBAUTHN_RP_ORIGINS", "")),
		WebAuthnRPDisplayName: env("WEBAUTHN_RP_DISPLAY_NAME", "Orbit"),

		AuthRateBurst:     envInt("AUTH_RATE_BURST", 8),
		AuthRatePerMinute: envInt("AUTH_RATE_PER_MINUTE", 12),

		LoginLockoutThreshold: envInt("LOGIN_LOCKOUT_THRESHOLD", 5),
		LoginLockoutWindow:    time.Duration(envInt("LOGIN_LOCKOUT_MINUTES", 15)) * time.Minute,

		MetricsEnabled: envBool("METRICS_ENABLED", false),
		SyslogAddr:     env("SYSLOG_ADDR", ""),
		PublicURL:      strings.TrimRight(env("PUBLIC_URL", ""), "/"),

		TLSCertFile: env("TLS_CERT_FILE", ""),
		TLSKeyFile:  env("TLS_KEY_FILE", ""),

		CookieDomain: env("COOKIE_DOMAIN", ""),
		RepoURL:      env("REPO_URL", ""),
		UpdateCheck:  envBool("UPDATE_CHECK", true),
	}
	// Auth cookies default to Secure in release or when the app serves TLS.
	c.CookieSecure = envBool("COOKIE_SECURE", c.GinMode == "release" || c.TLSCertFile != "")

	// SQLite (one file) or PostgreSQL (DB_URL a connection string); both are
	// tested (the API's tests also run on Postgres, ORBIT_TEST_POSTGRES).
	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (sqlite or postgres)", c.DBDriver)
	}

	// In release mode secrets must be supplied explicitly. In dev we fall back
	// to insecure constants so `make run` works out of the box.
	if c.GinMode == "release" {
		// Require strong secrets: a short JWT_SECRET makes HS256 tokens
		// forgeable, and a weak ORBIT_MASTER_KEY weakens every sealed secret.
		if len(c.JWTSecret) < minSecretLen {
			return nil, fmt.Errorf("JWT_SECRET must be at least %d characters in release mode", minSecretLen)
		}
		if len(c.MasterKey) < minSecretLen {
			return nil, fmt.Errorf("ORBIT_MASTER_KEY must be at least %d characters in release mode", minSecretLen)
		}
	} else {
		if c.JWTSecret == "" {
			c.JWTSecret = "dev-insecure-jwt-secret-change-me"
		}
		if c.MasterKey == "" {
			c.MasterKey = "dev-insecure-master-key-change-me-32b"
		}
		// Enable passkeys out of the box in dev (SPA on :5173, API on :8080).
		if c.WebAuthnRPID == "" {
			c.WebAuthnRPID = "localhost"
		}
		if len(c.WebAuthnRPOrigins) == 0 {
			c.WebAuthnRPOrigins = []string{"http://localhost:5173", "http://localhost:8080"}
		}
	}

	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func splitNonEmpty(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

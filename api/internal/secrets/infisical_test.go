package secrets

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInfisical answers Universal Auth sign-in and a folder's secrets (with
// one imported), and checks the token and what was asked for.
func fakeInfisical(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/universal-auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in["clientId"] != "cid" || in["clientSecret"] != "csecret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"Invalid credentials"}`))
			return
		}
		_, _ = w.Write([]byte(`{"accessToken":"tok-1","expiresIn":7200,"tokenType":"Bearer"}`))
	})
	mux.HandleFunc("GET /api/v3/secrets/raw", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("Authorization") != "Bearer tok-1" || q.Get("workspaceId") != "proj-1" || q.Get("environment") != "prod" || q.Get("secretPath") != "/orbit" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"no access"}`))
			return
		}
		_, _ = w.Write([]byte(`{"secrets":[{"secretKey":"ORBIT_MASTER_KEY","secretValue":"inf-mk"},{"secretKey":"JWT_SECRET","secretValue":"inf-jwt"}],
			"imports":[{"secretPath":"/shared","environment":"prod","secrets":[{"secretKey":"SMTP_PASSWORD","secretValue":"imported"},{"secretKey":"JWT_SECRET","secretValue":"loses"}]}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestInfisical(t *testing.T) {
	srv := fakeInfisical(t)
	t.Setenv("INFISICAL_URL", srv.URL+"/")
	t.Setenv("INFISICAL_PROJECT_ID", "proj-1")
	t.Setenv("INFISICAL_SECRET_PATH", "/orbit")
	t.Setenv("INFISICAL_CLIENT_ID", "cid")
	// The secret from a file, as a Docker secret would be.
	f := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(f, []byte("csecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("INFISICAL_CLIENT_SECRET_FILE", f)
	t.Setenv("DB_URL", "env-dsn")

	p, err := New("infisical")
	if err != nil {
		t.Fatalf("infisical provider: %v", err)
	}
	for k, want := range map[string]string{"ORBIT_MASTER_KEY": "inf-mk", "JWT_SECRET": "inf-jwt", "SMTP_PASSWORD": "imported", "DB_URL": "env-dsn"} {
		if v, _ := p.Get(k); v != want {
			t.Errorf("%s: %q, want %q", k, v, want)
		}
	}

	// A wrong secret: startup is refused, saying Infisical's reason.
	t.Setenv("INFISICAL_CLIENT_SECRET_FILE", "")
	t.Setenv("INFISICAL_CLIENT_SECRET", "nope")
	if _, err := New("infisical"); err == nil || !strings.Contains(err.Error(), "Invalid credentials") {
		t.Errorf("wrong secret: %v", err)
	}
	// No project: refused.
	t.Setenv("INFISICAL_PROJECT_ID", "")
	if _, err := New("infisical"); err == nil {
		t.Error("no project should be refused")
	}
}

// TestInfisicalLive reads a real Infisical (only with INFISICAL_LIVE=1 and
// the INFISICAL_* variables set): JWT_SECRET must be there.
func TestInfisicalLive(t *testing.T) {
	if os.Getenv("INFISICAL_LIVE") != "1" {
		t.Skip("INFISICAL_LIVE not set")
	}
	p, err := New("infisical")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := p.Get("JWT_SECRET"); v == "" {
		t.Error("JWT_SECRET not read")
	} else {
		t.Logf("JWT_SECRET read (%d characters)", len(v))
	}
}

package secrets

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeVault stands up the minimal Vault HTTP surface the provider uses:
// AppRole login and a KV v2 secret read.
func fakeVault(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/approle/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"auth":{"client_token":"s.approle-token"}}`))
	})
	mux.HandleFunc("/v1/secret/data/pki", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"ORBIT_MASTER_KEY":"vault-mk","JWT_SECRET":"vault-jwt"},"metadata":{"version":1}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestVaultTokenAuth(t *testing.T) {
	srv := fakeVault(t)
	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "s.root")
	t.Setenv("VAULT_KV_PATH", "pki")
	t.Setenv("DB_URL", "env-dsn") // not in Vault → env fallback

	p, err := New("vault")
	if err != nil {
		t.Fatalf("vault provider: %v", err)
	}
	if v, _ := p.Get("ORBIT_MASTER_KEY"); v != "vault-mk" {
		t.Errorf("ORBIT_MASTER_KEY: %q, want vault-mk", v)
	}
	if v, _ := p.Get("JWT_SECRET"); v != "vault-jwt" {
		t.Errorf("JWT_SECRET: %q, want vault-jwt", v)
	}
	if v, _ := p.Get("DB_URL"); v != "env-dsn" {
		t.Errorf("DB_URL fallback: %q, want env-dsn", v)
	}
}

func TestVaultAppRoleAuth(t *testing.T) {
	srv := fakeVault(t)
	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_ROLE_ID", "role-123")
	t.Setenv("VAULT_SECRET_ID", "secret-123")
	t.Setenv("VAULT_KV_PATH", "pki")

	p, err := New("vault")
	if err != nil {
		t.Fatalf("vault provider (approle): %v", err)
	}
	if v, _ := p.Get("ORBIT_MASTER_KEY"); v != "vault-mk" {
		t.Errorf("ORBIT_MASTER_KEY: %q, want vault-mk", v)
	}
}

func TestVaultMissingConfig(t *testing.T) {
	t.Setenv("VAULT_ADDR", "")
	if _, err := New("vault"); err == nil {
		t.Error("expected an error without VAULT_ADDR")
	}
}

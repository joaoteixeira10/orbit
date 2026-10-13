package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvProvider(t *testing.T) {
	t.Setenv("JWT_SECRET", "from-env")
	p, err := New("env")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := p.Get("JWT_SECRET"); v != "from-env" {
		t.Errorf("env Get: %q, want from-env", v)
	}
	if v, _ := p.Get("MISSING"); v != "" {
		t.Errorf("missing key: %q, want empty", v)
	}
}

func TestFileProvider(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "mk")
	if err := os.WriteFile(f, []byte("file-master-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBIT_MASTER_KEY_FILE", f)
	t.Setenv("JWT_SECRET", "plain-env")

	p, err := New("file")
	if err != nil {
		t.Fatal(err)
	}
	// _FILE wins and is trimmed.
	if v, _ := p.Get("ORBIT_MASTER_KEY"); v != "file-master-key" {
		t.Errorf("file Get: %q, want file-master-key", v)
	}
	// No _FILE → plain env fallback.
	if v, _ := p.Get("JWT_SECRET"); v != "plain-env" {
		t.Errorf("env fallback: %q, want plain-env", v)
	}
	// Missing file path → error.
	t.Setenv("DB_URL_FILE", filepath.Join(dir, "does-not-exist"))
	if _, err := p.Get("DB_URL"); err == nil {
		t.Error("expected an error for a missing _FILE path")
	}
}

func TestUnknownBackend(t *testing.T) {
	if _, err := New("carrier-pigeon"); err == nil {
		t.Error("expected an error for an unknown backend")
	}
}

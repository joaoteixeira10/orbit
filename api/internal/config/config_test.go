package config

import (
	"strings"
	"testing"
)

func TestLoadDevDefaults(t *testing.T) {
	t.Setenv("GIN_MODE", "debug")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("ORBIT_MASTER_KEY", "")
	t.Setenv("WEBAUTHN_RP_ID", "")
	t.Setenv("WEBAUTHN_RP_ORIGINS", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.JWTSecret == "" || c.MasterKey == "" {
		t.Fatal("dev mode should fall back to insecure default secrets")
	}
	if c.WebAuthnRPID != "localhost" {
		t.Fatalf("dev WebAuthnRPID = %q, want localhost", c.WebAuthnRPID)
	}
	if len(c.WebAuthnRPOrigins) == 0 {
		t.Fatal("dev WebAuthnRPOrigins should default to localhost origins")
	}
}

func TestLoadReleaseRequiresStrongSecrets(t *testing.T) {
	strong := strings.Repeat("a", minSecretLen)
	t.Setenv("GIN_MODE", "release")
	t.Setenv("ORBIT_MASTER_KEY", strong)

	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("release mode must reject a short JWT_SECRET")
	}

	t.Setenv("JWT_SECRET", strong)
	if _, err := Load(); err != nil {
		t.Fatalf("release mode should accept >=%d-char secrets: %v", minSecretLen, err)
	}

	t.Setenv("ORBIT_MASTER_KEY", "short")
	if _, err := Load(); err == nil {
		t.Fatal("release mode must reject a short ORBIT_MASTER_KEY")
	}
}

func TestLoadRejectsUnknownDriver(t *testing.T) {
	t.Setenv("GIN_MODE", "debug")
	t.Setenv("DB_DRIVER", "oracle")
	if _, err := Load(); err == nil {
		t.Fatal("an unsupported DB_DRIVER should error")
	}
}

package auth

import (
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

func TestAPITokenAuth(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := models.User{Username: "svc"}
	db.Create(&u)

	plain, hash, prefix := NewAPITokenSecret()
	if !strings.HasPrefix(plain, APITokenPrefix) || !strings.HasPrefix(prefix, APITokenPrefix) {
		t.Fatalf("unexpected token/prefix: %q / %q", plain, prefix)
	}
	tok := models.APIToken{UserID: u.ID, Name: "ci", Prefix: prefix, TokenHash: hash}
	db.Create(&tok)

	got, _, err := m.authenticateAPIToken(plain)
	if err != nil || got.ID != u.ID {
		t.Fatalf("valid token auth failed: err=%v user=%v", err, got)
	}
	if _, _, err := m.authenticateAPIToken("orb_unknown"); err == nil {
		t.Error("unknown token should be rejected")
	}

	db.Model(&tok).Update("revoked", true)
	if _, _, err := m.authenticateAPIToken(plain); err == nil {
		t.Error("revoked token should be rejected")
	}

	past := time.Now().Add(-time.Hour)
	db.Model(&tok).Updates(map[string]any{"revoked": false, "expires_at": &past})
	if _, _, err := m.authenticateAPIToken(plain); err == nil {
		t.Error("expired token should be rejected")
	}
}

func TestSweepExpiredAPITokens(t *testing.T) {
	db := testDB(t)
	past := time.Now().Add(-time.Hour)
	fut := time.Now().Add(time.Hour)
	db.Create(&models.APIToken{Name: "old", TokenHash: "h1", ExpiresAt: &past})
	db.Create(&models.APIToken{Name: "future", TokenHash: "h2", ExpiresAt: &fut})
	db.Create(&models.APIToken{Name: "never", TokenHash: "h3"})

	n, err := SweepExpiredAPITokens(db)
	if err != nil || n != 1 {
		t.Fatalf("expected to sweep 1 expired token, got %d (err=%v)", n, err)
	}
	var remaining int64
	db.Model(&models.APIToken{}).Count(&remaining)
	if remaining != 2 {
		t.Errorf("expected 2 tokens remaining, got %d", remaining)
	}
}

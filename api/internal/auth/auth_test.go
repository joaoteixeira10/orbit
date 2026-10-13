package auth

import (
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedUser(t *testing.T, db *gorm.DB, username, password string, role models.RoleName) *models.User {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := &models.User{Username: username, PasswordHash: hash}
	if role != "" { // attach the named role (created on the fly; admin = superuser)
		var r models.Role
		if err := db.Where(models.Role{Name: string(role)}).Attrs(models.Role{Superuser: role == models.RoleAdmin}).FirstOrCreate(&r).Error; err != nil {
			t.Fatalf("role: %v", err)
		}
		u.Roles = []models.Role{r}
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestPasswordHashAndCheck(t *testing.T) {
	hash, err := HashPassword("s3cret-password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !CheckPassword(hash, "s3cret-password") {
		t.Error("expected password to verify")
	}
	if CheckPassword(hash, "wrong") {
		t.Error("expected wrong password to fail")
	}
}

func TestAuthenticate(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	seedUser(t, db, "alice", "password123", models.RoleMember)

	if _, err := m.Authenticate("alice", "password123"); err != nil {
		t.Errorf("valid login should succeed, got %v", err)
	}
	if _, err := m.Authenticate("alice", "nope"); err != ErrInvalidCredentials {
		t.Errorf("bad password: want ErrInvalidCredentials, got %v", err)
	}
	if _, err := m.Authenticate("ghost", "x"); err != ErrInvalidCredentials {
		t.Errorf("unknown user: want ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthenticateLockout(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 3, 15*time.Minute) // lock after 3 failures
	seedUser(t, db, "carol", "password123", models.RoleMember)

	// First two wrong attempts: generic invalid creds, not yet locked.
	for i := 1; i <= 2; i++ {
		if _, err := m.Authenticate("carol", "wrong"); err != ErrInvalidCredentials {
			t.Fatalf("attempt %d: want ErrInvalidCredentials, got %v", i, err)
		}
	}
	// Third crosses the threshold → ErrAccountLocked (signals the caller to audit).
	if _, err := m.Authenticate("carol", "wrong"); err != ErrAccountLocked {
		t.Fatalf("3rd attempt: want ErrAccountLocked, got %v", err)
	}
	// While locked, even the CORRECT password is rejected (generically).
	if _, err := m.Authenticate("carol", "password123"); err != ErrInvalidCredentials {
		t.Fatalf("while locked: want ErrInvalidCredentials, got %v", err)
	}

	// Simulate the lock window elapsing.
	db.Model(&models.User{}).Where("username = ?", "carol").Update("locked_until", time.Now().Add(-time.Minute))
	u, err := m.Authenticate("carol", "password123")
	if err != nil {
		t.Fatalf("after window: want success, got %v", err)
	}
	m.RecordLoginSuccess(u)
	var reloaded models.User
	db.Where("username = ?", "carol").First(&reloaded)
	if reloaded.FailedLoginCount != 0 || reloaded.LockedUntil != nil {
		t.Errorf("after success: not cleared (count=%d locked=%v)", reloaded.FailedLoginCount, reloaded.LockedUntil)
	}
}

func TestAuthenticateLockoutDisabled(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0) // threshold 0 = disabled
	seedUser(t, db, "dave", "password123", models.RoleMember)
	for i := 1; i <= 10; i++ {
		if _, err := m.Authenticate("dave", "wrong"); err != ErrInvalidCredentials {
			t.Fatalf("attempt %d: want ErrInvalidCredentials (no lock), got %v", i, err)
		}
	}
	if _, err := m.Authenticate("dave", "password123"); err != nil {
		t.Fatalf("correct password should still work with lockout disabled, got %v", err)
	}
}

func TestAuthenticateDisabled(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "bob", "password123", models.RoleMember)
	db.Model(u).Update("disabled", true)

	if _, err := m.Authenticate("bob", "password123"); err != ErrUserDisabled {
		t.Errorf("disabled user: want ErrUserDisabled, got %v", err)
	}
}

func TestAccessTokenRoundTrip(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "carol", "password123", models.RoleAdmin)

	tok, exp, err := m.IssueAccessToken(u, "", time.Now())
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Error("expiry should be in the future")
	}
	claims, err := m.ParseAccessToken(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Subject != u.ID {
		t.Errorf("subject = %q, want %q", claims.Subject, u.ID)
	}
}

func TestAccessTokenWrongSecret(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "secret-a", 0, 0)
	u := seedUser(t, db, "dave", "password123", models.RoleMember)
	tok, _, _ := m.IssueAccessToken(u, "", time.Now())

	other := NewManager(db, "secret-b", 0, 0)
	if _, err := other.ParseAccessToken(tok); err != ErrInvalidToken {
		t.Errorf("token signed with different secret should fail, got %v", err)
	}
}

func TestExpiredAccessToken(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "erin", "password123", models.RoleMember)
	// Issue a token that expired an hour ago.
	tok, _, err := m.IssueAccessToken(u, "", time.Now().Add(-AccessTokenTTL-time.Hour))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, err := m.ParseAccessToken(tok); err != ErrInvalidToken {
		t.Errorf("expired token should fail, got %v", err)
	}
}

func TestRefreshTokenRotation(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "frank", "password123", models.RoleMember)
	now := time.Now()

	rt, _, err := m.IssueRefreshToken(u.ID, "", "", now)
	if err != nil {
		t.Fatalf("issue refresh: %v", err)
	}

	// First use succeeds and returns a new token.
	gotUser, next, _, err := m.Refresh(rt, "", "", now)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if gotUser.ID != u.ID {
		t.Errorf("refresh returned user %s, want %s", gotUser.ID, u.ID)
	}
	if next == rt {
		t.Error("refresh should rotate to a new token")
	}

	// The old token is now spent: replaying it after the reuse-grace window is
	// rejected as reuse (see refresh_reuse_test.go for the grace behaviour).
	if _, _, _, err := m.Refresh(rt, "", "", now.Add(RefreshReuseGrace+time.Second)); err != ErrRefreshReuse {
		t.Errorf("reused refresh token: want ErrRefreshReuse, got %v", err)
	}
}

func TestRefreshTokenRevoke(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "grace", "password123", models.RoleMember)
	now := time.Now()

	rt, _, _ := m.IssueRefreshToken(u.ID, "", "", now)
	if err := m.RevokeRefreshToken(rt); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, _, err := m.Refresh(rt, "", "", now); err != ErrInvalidRefresh {
		t.Errorf("revoked token: want ErrInvalidRefresh, got %v", err)
	}
}

func TestRefreshTokenExpired(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "heidi", "password123", models.RoleMember)

	past := time.Now().Add(-RefreshTokenTTL - time.Hour)
	rt, _, _ := m.IssueRefreshToken(u.ID, "", "", past)
	if _, _, _, err := m.Refresh(rt, "", "", time.Now()); err != ErrInvalidRefresh {
		t.Errorf("expired refresh: want ErrInvalidRefresh, got %v", err)
	}
}

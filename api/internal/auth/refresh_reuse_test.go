package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// TestRefreshReuseWithinGrace: a just-rotated token presented again inside the
// grace window (two tabs refreshing at once) still works and keeps the session.
func TestRefreshReuseWithinGrace(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "alice", "pw", models.RoleAdmin)
	now := time.Now()

	tok1, sid, _ := m.IssueRefreshToken(u.ID, "ua", "ip", now)
	_, tok2, sid2, err := m.Refresh(tok1, "", "ip", now)
	if err != nil || sid2 != sid {
		t.Fatalf("first refresh: err=%v sid=%s/%s", err, sid, sid2)
	}
	// Tab 2 races with the same (now rotated) token, 5s later.
	_, tok3, sid3, err := m.Refresh(tok1, "", "ip", now.Add(5*time.Second))
	if err != nil || sid3 != sid {
		t.Fatalf("in-grace replay must succeed in the same session: err=%v", err)
	}
	// Both successor chains remain usable.
	for i, tk := range []string{tok2, tok3} {
		if _, _, _, err := m.Refresh(tk, "", "ip", now.Add(6*time.Second)); err != nil {
			t.Errorf("successor %d unusable: %v", i, err)
		}
	}
}

// TestRefreshReuseBeyondGraceRevokesSession: replaying a rotated token after the
// grace window is treated as theft — the whole session (including the
// legitimate successor) is revoked.
func TestRefreshReuseBeyondGraceRevokesSession(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "bob", "pw", models.RoleAdmin)
	now := time.Now()

	tok1, sid, _ := m.IssueRefreshToken(u.ID, "ua", "ip", now)
	_, tok2, _, err := m.Refresh(tok1, "", "ip", now)
	if err != nil {
		t.Fatal(err)
	}
	// A second, unrelated session must be left alone.
	other, otherSid, _ := m.IssueRefreshToken(u.ID, "ua2", "ip2", now)

	gotUser, _, _, err := m.Refresh(tok1, "", "evil", now.Add(RefreshReuseGrace+time.Second))
	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("want ErrRefreshReuse, got %v", err)
	}
	if gotUser == nil || gotUser.ID != u.ID {
		t.Fatal("reuse error must identify the affected user for auditing")
	}
	if _, _, _, err := m.Refresh(tok2, "", "ip", now.Add(RefreshReuseGrace+2*time.Second)); err == nil {
		t.Fatal("the legitimate successor must be revoked with the session")
	}
	s, _ := m.ListSessions(u.ID, now)
	if len(s) != 1 || s[0].ID != otherSid || s[0].ID == sid {
		t.Fatalf("only the unrelated session should survive; got %+v", s)
	}
	if _, _, _, err := m.Refresh(other, "", "ip2", now.Add(time.Minute)); err != nil {
		t.Errorf("unrelated session broken: %v", err)
	}
}

// TestLogoutRevokesWholeSession: logging out ends the device session, so a
// just-rotated predecessor can't resurrect it inside the grace window, and a
// logged-out token is a plain invalid token (not a reuse alarm).
func TestLogoutRevokesWholeSession(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "carol", "pw", models.RoleAdmin)
	now := time.Now()

	tok1, _, _ := m.IssueRefreshToken(u.ID, "ua", "ip", now)
	_, tok2, _, _ := m.Refresh(tok1, "", "ip", now)
	if err := m.RevokeRefreshToken(tok2); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := m.Refresh(tok1, "", "ip", now.Add(time.Second)); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("predecessor after logout: want ErrInvalidRefresh, got %v", err)
	}
	if _, _, _, err := m.Refresh(tok2, "", "ip", now.Add(time.Hour)); !errors.Is(err, ErrInvalidRefresh) {
		t.Fatalf("logged-out token: want ErrInvalidRefresh, got %v", err)
	}
	if s, _ := m.ListSessions(u.ID, now); len(s) != 0 {
		t.Fatalf("session should be gone after logout, got %d", len(s))
	}
}

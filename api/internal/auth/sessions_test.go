package auth

import (
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

func TestSessionsLifecycle(t *testing.T) {
	db := testDB(t)
	m := NewManager(db, "test-secret", 0, 0)
	u := seedUser(t, db, "alice", "pw", models.RoleAdmin)
	now := time.Now()

	// Two distinct sessions.
	_, sidA, err := m.IssueRefreshToken(u.ID, "macOS Chrome", "1.1.1.1", now)
	if err != nil {
		t.Fatal(err)
	}
	tokB, sidB, _ := m.IssueRefreshToken(u.ID, "iOS Safari", "2.2.2.2", now)
	if sidA == sidB {
		t.Fatal("distinct logins must get distinct session ids")
	}

	if s, _ := m.ListSessions(u.ID, now); len(s) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(s))
	}

	// Rotating B keeps its session id and stays a single session.
	_, _, sidB2, err := m.Refresh(tokB, "", "9.9.9.9", now)
	if err != nil {
		t.Fatal(err)
	}
	if sidB2 != sidB {
		t.Errorf("rotation changed session id: %s -> %s", sidB, sidB2)
	}
	if s, _ := m.ListSessions(u.ID, now); len(s) != 2 {
		t.Errorf("rotation should not add a session; got %d", len(s))
	}

	// Revoke session A → only B remains.
	if err := m.RevokeSession(u.ID, sidA); err != nil {
		t.Fatal(err)
	}
	s, _ := m.ListSessions(u.ID, now)
	if len(s) != 1 || s[0].ID != sidB {
		t.Fatalf("after revoking A, want only B; got %+v", s)
	}

	// Revoke-others keeping B is a no-op here; add a third then revoke others.
	_, sidC, _ := m.IssueRefreshToken(u.ID, "Win Firefox", "3.3.3.3", now)
	_ = sidC
	if err := m.RevokeOtherSessions(u.ID, sidB); err != nil {
		t.Fatal(err)
	}
	s, _ = m.ListSessions(u.ID, now)
	if len(s) != 1 || s[0].ID != sidB {
		t.Fatalf("after revoke-others(keep B), want only B; got %+v", s)
	}
}

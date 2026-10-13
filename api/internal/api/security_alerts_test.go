package api

import (
	"encoding/json"
	"testing"
)

// TestSecurityAlerts covers the owner-managed alert toggles: default off,
// owner-only writes, persisted, and unknown keys rejected.
func TestSecurityAlerts(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	adminRole := adminRoleID(t, r, owner)

	type cfg struct {
		Events   map[string]bool `json:"events"`
		Channels []string        `json:"channels"`
		Email    string          `json:"email"`
	}
	get := func() cfg {
		t.Helper()
		w := authdo(r, "GET", "/api/v1/security-alerts", "", owner)
		if w.Code != 200 {
			t.Fatalf("get alerts: %d", w.Code)
		}
		var v cfg
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return v
	}

	// Default: every event off, no channel restriction, no email.
	d := get()
	if len(d.Events) == 0 {
		t.Fatal("expected the alert-event catalog")
	}
	for k, v := range d.Events {
		if v {
			t.Errorf("event %q defaulted on, want off", k)
		}
	}

	// Owner enables an event + picks channels + a recipient; it persists.
	if w := authdo(r, "PUT", "/api/v1/security-alerts",
		`{"events":{"owner_transfer":true},"channels":["email"],"email":"alerts@example.org"}`, owner); w.Code != 200 {
		t.Fatalf("update alerts: %d, body=%s", w.Code, w.Body.String())
	}
	g := get()
	if !g.Events["owner_transfer"] {
		t.Error("owner_transfer should be enabled after update")
	}
	if len(g.Channels) != 1 || g.Channels[0] != "email" || g.Email != "alerts@example.org" {
		t.Errorf("channels/email not persisted: %+v", g)
	}

	// Unknown event / channel are rejected.
	if w := authdo(r, "PUT", "/api/v1/security-alerts", `{"events":{"nope":true}}`, owner); w.Code != 400 {
		t.Errorf("unknown event: got %d, want 400", w.Code)
	}
	if w := authdo(r, "PUT", "/api/v1/security-alerts", `{"channels":["carrier-pigeon"]}`, owner); w.Code != 400 {
		t.Errorf("unknown channel: got %d, want 400", w.Code)
	}

	// Non-owner admin is blocked.
	if w := authdo(r, "POST", "/api/v1/users",
		`{"username":"bob","password":"bobpassword","role_ids":["`+adminRole+`"]}`, owner); w.Code != 201 {
		t.Fatalf("create bob: %d", w.Code)
	}
	bob := loginTok(t, r, "bob", "bobpassword")
	if w := authdo(r, "GET", "/api/v1/security-alerts", "", bob); w.Code != 403 {
		t.Errorf("non-owner get alerts: got %d, want 403", w.Code)
	}
	if w := authdo(r, "PUT", "/api/v1/security-alerts", `{"owner_transfer":false}`, bob); w.Code != 403 {
		t.Errorf("non-owner put alerts: got %d, want 403", w.Code)
	}
}

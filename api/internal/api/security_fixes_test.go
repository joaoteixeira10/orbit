package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// TestSSOAdminNeedsOwnerOptIn: a role claim naming admin (or an admin default
// role) grants nothing admin-level unless the owner allowed it.
func TestSSOAdminNeedsOwnerOptIn(t *testing.T) {
	_, db := newTestRouterDB(t, false)
	hh := &Handlers{db: db}
	names := func(rs []models.Role) string {
		var n []string
		for _, x := range rs {
			n = append(n, x.Name)
		}
		return strings.Join(n, ",")
	}
	if got := names(hh.mapOIDCRoles([]string{"admin", "member"}, "", false)); got != "member" {
		t.Errorf("claim admin without opt-in: %q", got)
	}
	if got := names(hh.mapOIDCRoles([]string{"admin"}, "admin", false)); got != "none" {
		t.Errorf("admin default without opt-in: %q", got)
	}
	if got := names(hh.mapOIDCRoles([]string{"admin"}, "", true)); got != "admin" {
		t.Errorf("with opt-in: %q", got)
	}
}

// TestFailedSignInsAudited: a wrong password leaves an audit entry.
func TestFailedSignInsAudited(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	do(r, "POST", "/api/v1/auth/login", `{"username":"nobody","password":"nope"}`)
	var n int64
	db.Model(&models.AuditLog{}).Where("action = ? AND detail LIKE ?", "auth.login_failed", "%nobody%").Count(&n)
	if n != 1 {
		t.Errorf("failed sign-in audited: %d", n)
	}
}

// TestEndedSessionStopsAtOnce: after sign-out the access token stops working.
func TestEndedSessionStopsAtOnce(t *testing.T) {
	r := newTestRouter(t)
	refresh, csrf, body := loginCookies(t, r)
	var login struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal([]byte(body), &login)
	if w := authdo(r, "GET", "/api/v1/auth/me", "", login.AccessToken); w.Code != http.StatusOK {
		t.Fatalf("before: %d", w.Code)
	}
	if w := reqWithCookies(r, "/api/v1/auth/logout", []*http.Cookie{refresh, csrf}, csrf.Value); w.Code >= 300 {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", login.AccessToken); w.Code != http.StatusUnauthorized {
		t.Errorf("access token after sign-out: %d, want 401", w.Code)
	}
}

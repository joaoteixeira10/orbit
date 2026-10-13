package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// loginCookiesFor logs in and returns the refresh + csrf cookies (the refresh
// token now lives in the HttpOnly cookie, not the response body).
func loginCookiesFor(t *testing.T, r *gin.Engine, user, pass string) (refresh, csrf *http.Cookie) {
	t.Helper()
	w := do(r, "POST", "/api/v1/auth/login", `{"username":"`+user+`","password":"`+pass+`"}`)
	if w.Code != 200 {
		t.Fatalf("login %s: got %d, body=%s", user, w.Code, w.Body.String())
	}
	for _, ck := range w.Result().Cookies() {
		switch ck.Name {
		case refreshCookieName:
			refresh = ck
		case csrfCookieName:
			csrf = ck
		}
	}
	if refresh == nil || csrf == nil {
		t.Fatalf("login %s: missing refresh/csrf cookie", user)
	}
	return refresh, csrf
}

// TestForceLogout covers the admin "sign out everywhere" endpoint: it requires
// users.manage, 404s on an unknown user, and actually kills the target's
// sessions so their refresh token stops working.
func TestForceLogout(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")

	// A plain (no-role) user — has neither users.manage nor any privilege.
	if w := authdo(r, "POST", "/api/v1/users",
		`{"username":"eve","password":"evepassword","role_ids":[]}`, admin); w.Code != 201 {
		t.Fatalf("create eve: got %d, body=%s", w.Code, w.Body.String())
	}
	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	// fetch eve's id via the users list
	lw := authdo(r, "GET", "/api/v1/users", "", admin)
	var users []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &users)
	for _, u := range users {
		if u.Username == "eve" {
			created.User.ID = u.ID
		}
	}
	if created.User.ID == "" {
		t.Fatal("could not find eve's id")
	}

	// eve has a live session.
	eveRefresh, eveCsrf := loginCookiesFor(t, r, "eve", "evepassword")
	eveTok := loginTok(t, r, "eve", "evepassword")

	// eve (no users.manage) cannot force-logout anyone.
	if w := authdo(r, "POST", "/api/v1/users/"+created.User.ID+"/logout", "", eveTok); w.Code != 403 {
		t.Errorf("eve force-logout: got %d, want 403", w.Code)
	}

	// Unknown user → 404.
	if w := authdo(r, "POST", "/api/v1/users/999999/logout", "", admin); w.Code != 404 {
		t.Errorf("force-logout missing user: got %d, want 404", w.Code)
	}

	// eve's access token works right before the force-logout.
	if w := authdo(r, "GET", "/api/v1/auth/me", "", eveTok); w.Code != 200 {
		t.Fatalf("eve /me before force-logout: got %d, want 200", w.Code)
	}

	// Admin signs eve out everywhere.
	if w := authdo(r, "POST", "/api/v1/users/"+created.User.ID+"/logout", "", admin); w.Code != 204 {
		t.Fatalf("admin force-logout eve: got %d, body=%s", w.Code, w.Body.String())
	}

	// eve's existing access token is rejected immediately (TokenVersion bump),
	// not just after the TTL.
	if w := authdo(r, "GET", "/api/v1/auth/me", "", eveTok); w.Code != 401 {
		t.Errorf("eve /me after force-logout: got %d, want 401 (immediate)", w.Code)
	}
	// …and the refresh cookie is dead too: it can't be refreshed (CSRF passes —
	// cookie matches header — but the revoked token is rejected).
	if w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{eveRefresh, eveCsrf}, eveCsrf.Value); w.Code != 401 {
		t.Errorf("eve refresh after force-logout: got %d, want 401", w.Code)
	}
}

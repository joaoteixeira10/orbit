package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/ephemeral"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// createUserVia creates a local user through the admin API and returns its id.
func createUserVia(t *testing.T, r *gin.Engine, admin, body string) string {
	t.Helper()
	w := authdo(r, "POST", "/api/v1/users", body, admin)
	if w.Code != http.StatusCreated {
		t.Fatalf("create user: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.User.ID
}

// TestChangePasswordEndsOtherSessions: changing your own password kills every
// other live access token and session at once, and hands the caller a fresh
// access token so the current session continues.
func TestChangePasswordEndsOtherSessions(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"pat","password":"oldpassword","role_ids":[]}`)

	other := loginTok(t, r, "pat", "oldpassword") // another device
	otherRefresh, otherCsrf := loginCookiesFor(t, r, "pat", "oldpassword")
	me := loginTok(t, r, "pat", "oldpassword") // this device

	w := authdo(r, "PUT", "/api/v1/me/password", `{"old_password":"oldpassword","new_password":"newpassword"}`, me)
	if w.Code != http.StatusOK {
		t.Fatalf("change password: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Fatal("change password must return a fresh access token")
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", resp.AccessToken); w.Code != http.StatusOK {
		t.Errorf("fresh token rejected: %d", w.Code)
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", other); w.Code != http.StatusUnauthorized {
		t.Errorf("another device's access token must die at once: got %d", w.Code)
	}
	if w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{otherRefresh, otherCsrf}, otherCsrf.Value); w.Code != http.StatusUnauthorized {
		t.Errorf("another device's refresh must be revoked: got %d", w.Code)
	}
}

// TestAdminResetPasswordForcesChangeAndSignsOut: an admin reset makes the
// password temporary (must change) by default and ends live sessions at once.
func TestAdminResetPasswordForcesChangeAndSignsOut(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	id := createUserVia(t, r, admin, `{"username":"quinn","password":"quinnpassword","role_ids":[]}`)
	live := loginTok(t, r, "quinn", "quinnpassword")

	if w := authdo(r, "PUT", "/api/v1/users/"+id+"/password", `{"new_password":"temporary123"}`, admin); w.Code != http.StatusNoContent {
		t.Fatalf("reset: got %d, body=%s", w.Code, w.Body.String())
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", live); w.Code != http.StatusUnauthorized {
		t.Errorf("live access token must be rejected after an admin reset: got %d", w.Code)
	}
	var u models.User
	db.First(&u, "id = ?", id)
	if !u.MustChangePassword {
		t.Error("an admin reset must force a password change by default")
	}

	// Explicit opt-out is honoured.
	if w := authdo(r, "PUT", "/api/v1/users/"+id+"/password", `{"new_password":"permanent123","must_change_password":false}`, admin); w.Code != http.StatusNoContent {
		t.Fatalf("reset (no force): got %d", w.Code)
	}
	db.First(&u, "id = ?", id)
	if u.MustChangePassword {
		t.Error("must_change_password=false must be honoured")
	}
}

// TestForcedChangeNeedsANewPassword: a forced change refuses the current
// password again; an ordinary change may keep or reuse one (no history).
func TestForcedChangeNeedsANewPassword(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	id := createUserVia(t, r, admin, `{"username":"rowan","password":"rowanpassword","role_ids":[]}`)
	if w := authdo(r, "PUT", "/api/v1/users/"+id+"/password", `{"new_password":"temporary123"}`, admin); w.Code != http.StatusNoContent {
		t.Fatalf("reset: got %d", w.Code)
	}
	me := loginTok(t, r, "rowan", "temporary123")
	if w := authdo(r, "PUT", "/api/v1/me/password", `{"old_password":"temporary123","new_password":"temporary123"}`, me); w.Code != http.StatusBadRequest {
		t.Fatalf("forced change to the same password: got %d, want 400", w.Code)
	}
	w := authdo(r, "PUT", "/api/v1/me/password", `{"old_password":"temporary123","new_password":"rowanpassword"}`, me)
	if w.Code != http.StatusOK {
		t.Fatalf("forced change to an older password: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w := authdo(r, "PUT", "/api/v1/me/password", `{"old_password":"rowanpassword","new_password":"rowanpassword"}`, resp.AccessToken); w.Code != http.StatusOK {
		t.Errorf("an ordinary change may keep the password: got %d", w.Code)
	}
}

// TestEmailUniqueness: no two accounts may share an email (case-insensitively),
// on any write path — emails are where notifications and SSO matching go.
func TestEmailUniqueness(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"una","email":"una@example.org","password":"unapassword","role_ids":[]}`)
	vicID := createUserVia(t, r, admin, `{"username":"vic","email":"vic@example.org","password":"vicpassword","role_ids":[]}`)

	if w := authdo(r, "POST", "/api/v1/users", `{"username":"una2","email":"UNA@example.org","password":"una2password","role_ids":[]}`, admin); w.Code != http.StatusConflict {
		t.Errorf("create with a duplicate email: got %d, want 409", w.Code)
	}
	if w := authdo(r, "PUT", "/api/v1/users/"+vicID, `{"email":"Una@Example.org"}`, admin); w.Code != http.StatusConflict {
		t.Errorf("admin update to a duplicate email: got %d, want 409", w.Code)
	}
	vic := loginTok(t, r, "vic", "vicpassword")
	if w := authdo(r, "PUT", "/api/v1/me/profile", `{"email":"una@example.org"}`, vic); w.Code != http.StatusConflict {
		t.Errorf("self update to a duplicate email: got %d, want 409", w.Code)
	}
	// Re-saving your own email (different case) is not a conflict.
	if w := authdo(r, "PUT", "/api/v1/me/profile", `{"email":"VIC@example.org"}`, vic); w.Code != http.StatusOK {
		t.Errorf("re-saving own email: got %d, body=%s", w.Code, w.Body.String())
	}
}

// TestSSOExchangeSingleUse: a one-time SSO code redeems once for an access token
// on the session opened by the callback, and never again.
func TestSSOExchangeSingleUse(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	var admin models.User
	db.Where(&models.User{Username: "admin"}).First(&admin)
	code := seedSSOCode(t, db, admin.ID, "sid-1")

	w := do(r, "POST", "/api/v1/auth/oidc/exchange", `{"code":"`+code+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("exchange: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w := authdo(r, "GET", "/api/v1/auth/me", "", resp.AccessToken); w.Code != http.StatusOK {
		t.Errorf("exchanged token rejected: %d", w.Code)
	}
	if w := do(r, "POST", "/api/v1/auth/oidc/exchange", `{"code":"`+code+`"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("second exchange: got %d, want 401", w.Code)
	}
}

func seedSSOCode(t *testing.T, db *gorm.DB, userID, sid string) string {
	t.Helper()
	// The callback opens a session (a refresh token) before handing out the
	// code; the access token minted at exchange belongs to it.
	db.Create(&models.RefreshToken{UserID: userID, SessionID: sid, TokenHash: "seed-" + sid, ExpiresAt: time.Now().Add(time.Hour)})
	code, err := ephemeral.New(db).Put(ephemeral.KindSSOCode, ssoHandoff{UserID: userID, SessionID: sid}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// TestRefreshReuseRevokesSessionEndToEnd: replaying a rotated refresh cookie
// after the grace window fails, clears the cookies, audits the event, and
// kills the session for the legitimate holder too.
func TestRefreshReuseRevokesSessionEndToEnd(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	refresh, csrf, _ := loginCookies(t, r)
	w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{refresh, csrf}, csrf.Value)
	if w.Code != http.StatusOK {
		t.Fatalf("first refresh: got %d", w.Code)
	}
	var next *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == refreshCookieName {
			next = ck
		}
	}
	// Age the rotation past the grace window.
	db.Model(&models.RefreshToken{}).Where("rotated_at IS NOT NULL").
		Update("rotated_at", time.Now().Add(-time.Hour))

	w = reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{refresh, csrf}, csrf.Value)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("replayed rotated token: got %d, want 401", w.Code)
	}
	cleared := false
	for _, ck := range w.Result().Cookies() {
		if ck.Name == refreshCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("a failed refresh should clear the refresh cookie")
	}
	var n int64
	db.Model(&models.AuditLog{}).Where("action = ?", "auth.refresh_reuse").Count(&n)
	if n != 1 {
		t.Errorf("want 1 auth.refresh_reuse audit row, got %d", n)
	}
	if w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{next, csrf}, csrf.Value); w.Code != http.StatusUnauthorized {
		t.Errorf("the legitimate successor must be revoked too: got %d", w.Code)
	}
}

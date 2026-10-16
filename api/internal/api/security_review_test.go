package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// Fixes from the 2026-10-15 security review (SECURITY-REVIEW-2026-10-15.md).

// TestImpersonationCantMintAccess: acting as someone can't leave anything
// that outlasts the impersonation token.
func TestImpersonationCantMintAccess(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	if w := authdo(r, "PUT", "/api/v1/features", `{"impersonation":true}`, owner); w.Code != 200 {
		t.Fatalf("enable flag: %d", w.Code)
	}
	bob := createUserAs(t, r, owner, `{"username":"bob","password":"bobpassword","role_ids":[]}`)
	w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate", "", owner)
	if w.Code != 200 {
		t.Fatalf("impersonate: %d %s", w.Code, w.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/v1/me/api-tokens", `{"name":"keep"}`},
		{"POST", "/api/v1/me/rotate-token", ""},
		{"PUT", "/api/v1/me/password", `{"old_password":"x","new_password":"newpassword123"}`},
		{"POST", "/api/v1/me/otp/setup", ""},
		{"POST", "/api/v1/me/webauthn/register/begin", ""},
		{"POST", "/api/v1/me/feeds", `{"name":"mine"}`},
		{"POST", "/api/v1/me/git", `{"base_url":"https://git.example.org","token":"x"}`},
	} {
		if w := authdo(r, c.method, c.path, c.body, tok.AccessToken); w.Code != http.StatusForbidden {
			t.Errorf("%s %s while impersonating: %d", c.method, c.path, w.Code)
		}
	}
	// Bob himself still can.
	bobTok := loginTok(t, r, "bob", "bobpassword")
	if w := authdo(r, "POST", "/api/v1/me/api-tokens", `{"name":"mine"}`, bobTok); w.Code != http.StatusCreated {
		t.Errorf("bob's own token: %d %s", w.Code, w.Body.String())
	}
}

// TestOTPSetupRefusedWhileOn: setting up again would silently turn two-factor off.
func TestOTPSetupRefusedWhileOn(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	db.Model(&models.User{}).Where("username = ?", "admin").Update("otp_enabled", true)
	if w := authdo(r, "POST", "/api/v1/me/otp/setup", "", owner); w.Code != http.StatusConflict {
		t.Fatalf("setup while on: %d %s", w.Code, w.Body.String())
	}
	var u models.User
	db.First(&u, "username = ?", "admin")
	if !u.OTPEnabled {
		t.Fatal("two-factor was turned off")
	}
}

// TestPasswordRecheckLocksOut: wrong passwords in "confirm it's you" count
// toward the lockout, which then refuses even the right one.
func TestPasswordRecheckLocksOut(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	createUserAs(t, r, owner, `{"username":"bob","password":"bobpassword","role_ids":[]}`)
	bob := loginTok(t, r, "bob", "bobpassword")
	for i := 0; i < 5; i++ {
		if w := authdo(r, "POST", "/api/v1/auth/step-up", `{"password":"wrong-guess"}`, bob); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: %d", i, w.Code)
		}
	}
	if w := authdo(r, "POST", "/api/v1/auth/step-up", `{"password":"bobpassword"}`, bob); w.Code != http.StatusTooManyRequests {
		t.Fatalf("right password while locked: %d", w.Code)
	}
	if w := authdo(r, "PUT", "/api/v1/me/password", `{"old_password":"bobpassword","new_password":"newpassword123"}`, bob); w.Code != http.StatusTooManyRequests {
		t.Fatalf("password change while locked: %d", w.Code)
	}
}

// TestForceLogoutRevokesAPITokens: "sign out everywhere" includes scripts.
func TestForceLogoutRevokesAPITokens(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	bobID := createUserAs(t, r, owner, `{"username":"bob","password":"bobpassword","role_ids":[]}`)
	bob := loginTok(t, r, "bob", "bobpassword")
	w := authdo(r, "POST", "/api/v1/me/api-tokens", `{"name":"script"}`, bob)
	var tok struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok.Token == "" {
		t.Fatalf("no token: %s", w.Body.String())
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", tok.Token); w.Code != 200 {
		t.Fatalf("token before: %d", w.Code)
	}
	if w := authdo(r, "POST", "/api/v1/users/"+bobID+"/logout", "", owner); w.Code != http.StatusNoContent {
		t.Fatalf("force logout: %d", w.Code)
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", tok.Token); w.Code != http.StatusUnauthorized {
		t.Fatalf("token after sign-out everywhere: %d", w.Code)
	}
}

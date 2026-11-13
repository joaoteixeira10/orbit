package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// TestRequireOTPForOneUser: "require two-factor" on one user (global policy
// off) restricts only that user to the enrollment endpoints, /auth/me says so,
// and lifting the requirement frees them again.
func TestRequireOTPForOneUser(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")
	id := createUserVia(t, r, admin, `{"username":"otto","password":"ottopassword","role_ids":[]}`)
	createUserVia(t, r, admin, `{"username":"free","password":"freepassword","role_ids":[]}`)

	if w := authdo(r, "PUT", "/api/v1/users/"+id, `{"otp_required":true}`, admin); w.Code != http.StatusOK {
		t.Fatalf("require otp: %d %s", w.Code, w.Body.String())
	}
	var list []models.User
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/users", "", admin).Body.Bytes(), &list)
	for _, u := range list {
		if want := u.ID == id; u.NeedsOTPSetup != want {
			t.Errorf("user list: %s needs_otp_setup=%v, want %v", u.Username, u.NeedsOTPSetup, want)
		}
	}
	otto := loginTok(t, r, "otto", "ottopassword")
	w := authdo(r, "GET", "/api/v1/auth/me", "", otto)
	var me struct {
		MustSetupOTP bool `json:"must_setup_otp"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &me)
	if !me.MustSetupOTP {
		t.Error("/auth/me must_setup_otp should be true for otto")
	}
	if w := authdo(r, "GET", "/api/v1/me/prefs", "", otto); w.Code != http.StatusForbidden {
		t.Errorf("otto without two-factor: got %d, want 403 otp_setup_required", w.Code)
	}
	if w := authdo(r, "POST", "/api/v1/me/otp/setup", "", otto); w.Code != http.StatusOK {
		t.Errorf("otto may still start enrollment: got %d", w.Code)
	}
	free := loginTok(t, r, "free", "freepassword")
	if w := authdo(r, "GET", "/api/v1/me/prefs", "", free); w.Code != http.StatusOK {
		t.Errorf("another user is not affected: got %d", w.Code)
	}
	authdo(r, "PUT", "/api/v1/users/"+id, `{"otp_required":false}`, admin)
	if w := authdo(r, "GET", "/api/v1/me/prefs", "", otto); w.Code != http.StatusOK {
		t.Errorf("after lifting the requirement: got %d", w.Code)
	}
}

// TestUnlockUser: a locked account can't sign in; an admin unlock lets it.
func TestUnlockUser(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	id := createUserVia(t, r, admin, `{"username":"lou","password":"loupassword","role_ids":[]}`)
	until := time.Now().Add(time.Hour)
	db.Model(&models.User{}).Where("id = ?", id).Update("locked_until", until)

	var list []models.User
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/users", "", admin).Body.Bytes(), &list)
	for _, u := range list {
		if u.ID == id && u.LockedUntilView == nil {
			t.Error("the user list should show locked_until")
		}
	}
	if w := do(r, "POST", "/api/v1/auth/login", `{"username":"lou","password":"loupassword"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("locked login: got %d", w.Code)
	}
	if w := authdo(r, "POST", "/api/v1/users/"+id+"/unlock", "", admin); w.Code != http.StatusNoContent {
		t.Fatalf("unlock: %d %s", w.Code, w.Body.String())
	}
	loginTok(t, r, "lou", "loupassword") // fatal if it still fails
	var u models.User
	db.First(&u, "id = ?", id)
	if u.LastLoginAt == nil {
		t.Error("last_login_at should be stamped on sign-in")
	}
}

// TestGeneratedTemporaryPassword: create and reset can generate a one-time
// password (returned once, must be changed); you can't disable yourself.
func TestGeneratedTemporaryPassword(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")

	w := authdo(r, "POST", "/api/v1/users", `{"username":"tim","generate_password":true,"role_ids":[]}`, admin)
	var created struct {
		User struct{ ID string } `json:"user"`
		Pw   string              `json:"generated_password"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if w.Code != http.StatusCreated || len(created.Pw) != 19 {
		t.Fatalf("create: %d %q", w.Code, created.Pw)
	}
	var u models.User
	db.First(&u, "id = ?", created.User.ID)
	if !u.MustChangePassword {
		t.Error("a generated password must be temporary")
	}

	w = authdo(r, "PUT", "/api/v1/users/"+created.User.ID+"/password", `{"generate":true}`, admin)
	var reset struct {
		Pw string `json:"generated_password"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &reset)
	if w.Code != http.StatusOK || len(reset.Pw) != 19 || reset.Pw == created.Pw {
		t.Fatalf("reset: %d %q", w.Code, reset.Pw)
	}
	loginTok(t, r, "tim", reset.Pw)
	if w := authdo(r, "PUT", "/api/v1/users/"+created.User.ID+"/password", `{"new_password":"short"}`, admin); w.Code != http.StatusBadRequest {
		t.Errorf("short admin-set password: got %d", w.Code)
	}

	var self models.User
	db.First(&self, "username = ?", "admin")
	if w := authdo(r, "PUT", "/api/v1/users/"+self.ID, `{"disabled":true}`, admin); w.Code != http.StatusBadRequest {
		t.Errorf("disabling yourself: got %d, want 400", w.Code)
	}
}

// TestDisableSignsOut: disabling a user kills their live access token and
// sessions at once; enabling again doesn't bring the old token back.
func TestDisableSignsOut(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")
	id := createUserVia(t, r, admin, `{"username":"dana","password":"danapassword","role_ids":[]}`)
	dana := loginTok(t, r, "dana", "danapassword")
	if w := authdo(r, "GET", "/api/v1/auth/me", "", dana); w.Code != http.StatusOK {
		t.Fatalf("before: %d", w.Code)
	}
	if w := authdo(r, "PUT", "/api/v1/users/"+id, `{"disabled":true}`, admin); w.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}
	if w := authdo(r, "GET", "/api/v1/auth/me", "", dana); w.Code != http.StatusUnauthorized {
		t.Errorf("disabled user's token: got %d, want 401", w.Code)
	}
	authdo(r, "PUT", "/api/v1/users/"+id, `{"disabled":false}`, admin)
	if w := authdo(r, "GET", "/api/v1/auth/me", "", dana); w.Code != http.StatusUnauthorized {
		t.Errorf("old token after re-enable: got %d, want 401", w.Code)
	}
	if loginTok(t, r, "dana", "danapassword") == "" {
		t.Error("re-enabled user can sign in again")
	}
}

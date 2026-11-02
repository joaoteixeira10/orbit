package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// meAs calls /auth/me with the given token and returns the username and the
// impersonator username ("" when not impersonating).
func meAs(t *testing.T, r *gin.Engine, token string) (username, impersonator string) {
	t.Helper()
	w := authdo(r, "GET", "/api/v1/auth/me", "", token)
	if w.Code != 200 {
		t.Fatalf("me: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Impersonator *struct {
			Username string `json:"username"`
		} `json:"impersonator"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Impersonator != nil {
		impersonator = resp.Impersonator.Username
	}
	return resp.User.Username, impersonator
}

// meID calls /auth/me and returns the authenticated user's id.
func meID(t *testing.T, r *gin.Engine, token string) string {
	t.Helper()
	w := authdo(r, "GET", "/api/v1/auth/me", "", token)
	if w.Code != 200 {
		t.Fatalf("me: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.User.ID == "" {
		t.Fatalf("me: no id, body=%s", w.Body.String())
	}
	return resp.User.ID
}

// createUserAs creates a user and returns its id.
func createUserAs(t *testing.T, r *gin.Engine, token, body string) string {
	t.Helper()
	w := authdo(r, "POST", "/api/v1/users", body, token)
	if w.Code != 201 {
		t.Fatalf("create user: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.User.ID == "" {
		t.Fatalf("create user: no id, body=%s", w.Body.String())
	}
	return resp.User.ID
}

func TestImpersonation(t *testing.T) {
	r := newTestRouter(t) // bootstrap admin is owner + superuser
	owner := loginTok(t, r, "admin", "adminpass")
	ownerID := meID(t, r, owner)
	adminRole := adminRoleID(t, r, owner)

	// Flag off → the route 404s even for the owner.
	if w := authdo(r, "POST", "/api/v1/users/1/impersonate", "", owner); w.Code != 404 {
		t.Fatalf("impersonate with flag off: got %d, want 404", w.Code)
	}
	// Owner enables the impersonation flag.
	if w := authdo(r, "PUT", "/api/v1/features", `{"impersonation":true}`, owner); w.Code != 200 {
		t.Fatalf("enable flag: got %d, body=%s", w.Code, w.Body.String())
	}

	// A custom role that grants only users.impersonate (not superuser).
	cr := authdo(r, "POST", "/api/v1/roles",
		`{"name":"helpdesk","global_privileges":["users.impersonate"]}`, owner)
	if cr.Code != 201 {
		t.Fatalf("create role: got %d, body=%s", cr.Code, cr.Body.String())
	}
	var role struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(cr.Body.Bytes(), &role)

	// Users: eve (plain), bob (superuser admin), carol (helpdesk: can impersonate).
	eve := createUserAs(t, r, owner, `{"username":"eve","password":"evepassword","role_ids":[]}`)
	bob := createUserAs(t, r, owner, `{"username":"bob","password":"bobpassword","role_ids":["`+adminRole+`"]}`)
	_ = createUserAs(t, r, owner, `{"username":"carol","password":"carolpassword","role_ids":["`+role.ID+`"]}`)
	carol := loginTok(t, r, "carol", "carolpassword")
	eveTok := loginTok(t, r, "eve", "evepassword")

	// eve has no users.impersonate → 403 at the route gate.
	if w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate", "", eveTok); w.Code != 403 {
		t.Errorf("no-priv impersonate: got %d, want 403", w.Code)
	}
	// Cannot impersonate yourself.
	if w := authdo(r, "POST", "/api/v1/users/"+ownerID+"/impersonate", "", owner); w.Code != 400 {
		t.Errorf("self impersonate: got %d, want 400", w.Code)
	}

	// Owner impersonates eve → 200; the returned token authenticates as eve and
	// reports the owner as impersonator.
	w := authdo(r, "POST", "/api/v1/users/"+eve+"/impersonate", "", owner)
	if w.Code != 200 {
		t.Fatalf("owner impersonate eve: got %d, body=%s", w.Code, w.Body.String())
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	if tok.RefreshToken != "" {
		t.Error("impersonation must not return a refresh token")
	}
	if u, imp := meAs(t, r, tok.AccessToken); u != "eve" || imp != "admin" {
		t.Errorf("me as impersonated: user=%q impersonator=%q, want eve/admin", u, imp)
	}

	// Owner may impersonate a superuser admin (bob).
	if w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate", "", owner); w.Code != 200 {
		t.Errorf("owner impersonate superuser: got %d, want 200", w.Code)
	}

	// carol (non-owner, has the privilege) may impersonate a plain user…
	if w := authdo(r, "POST", "/api/v1/users/"+eve+"/impersonate", "", carol); w.Code != 200 {
		t.Errorf("carol impersonate eve: got %d, want 200, body=%s", w.Code, w.Body.String())
	}
	// …but NOT a superuser admin (bob)…
	if w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate", "", carol); w.Code != 403 {
		t.Errorf("carol impersonate superuser: got %d, want 403", w.Code)
	}
	// …nor the instance owner.
	if w := authdo(r, "POST", "/api/v1/users/"+ownerID+"/impersonate", "", carol); w.Code != 403 {
		t.Errorf("carol impersonate owner: got %d, want 403", w.Code)
	}
}

// TestImpersonationAudit verifies that a mutation performed while impersonating
// is attributed to the impersonated user but records the real admin behind it.
func TestImpersonationAudit(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	adminRole := adminRoleID(t, r, owner)
	if w := authdo(r, "PUT", "/api/v1/features", `{"impersonation":true}`, owner); w.Code != 200 {
		t.Fatalf("enable flag: %d", w.Code)
	}
	bob := createUserAs(t, r, owner, `{"username":"bob","password":"bobpassword","role_ids":["`+adminRole+`"]}`)

	// Owner impersonates bob (a superuser, allowed for the owner).
	w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate", "", owner)
	if w.Code != 200 {
		t.Fatalf("impersonate bob: %d, body=%s", w.Code, w.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)

	// As impersonated-bob, create a user — a mutation that funnels through recordCtx.
	createUserAs(t, r, tok.AccessToken, `{"username":"frank","password":"frankpassword","role_ids":[]}`)

	// The user.create entry should name bob as actor and admin as impersonator.
	aw := authdo(r, "GET", "/api/v1/audit?per_page=500", "", owner)
	if aw.Code != 200 {
		t.Fatalf("list audit: %d", aw.Code)
	}
	var page struct {
		Items []struct {
			Action           string `json:"action"`
			UserName         string `json:"user_name"`
			ImpersonatorName string `json:"impersonator_name"`
			Detail           string `json:"detail"`
		} `json:"items"`
	}
	_ = json.Unmarshal(aw.Body.Bytes(), &page)
	var found bool
	for _, it := range page.Items {
		if it.Action == "user.create" && strings.Contains(it.Detail, "frank") {
			found = true
			if it.UserName != "bob" {
				t.Errorf("actor: got %q, want bob", it.UserName)
			}
			if it.ImpersonatorName != "admin" {
				t.Errorf("impersonator: got %q, want admin", it.ImpersonatorName)
			}
		}
	}
	if !found {
		t.Error("did not find the user.create audit entry for frank")
	}

	// Back as the owner: the end is recorded, and the impersonation filter
	// shows the start, the end and what was done meanwhile — nothing else.
	if w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate/end", `{"reason":"returned"}`, owner); w.Code != 204 {
		t.Fatalf("end impersonation: %d, body=%s", w.Code, w.Body.String())
	}
	fw := authdo(r, "GET", "/api/v1/audit?per_page=500&impersonation=1", "", owner)
	page.Items = nil
	_ = json.Unmarshal(fw.Body.Bytes(), &page)
	var got []string
	for _, it := range page.Items {
		got = append(got, it.Action)
	}
	if strings.Join(got, ",") != "user.impersonate_end,user.create,user.impersonate" {
		t.Errorf("impersonation filter: got %v", got)
	}
	if w := authdo(r, "POST", "/api/v1/users/"+bob+"/impersonate/end", "", tok.AccessToken); w.Code != 403 {
		t.Errorf("bob (impersonated) may not record ends: got %d, want 403", w.Code)
	}
}

// TestForceLogoutEndsImpersonation verifies that force-logging-out a user also
// instantly kills any active impersonation session of that user: the
// impersonation token carries the target's TokenVersion, which force-logout
// bumps.
func TestForceLogoutEndsImpersonation(t *testing.T) {
	r := newTestRouter(t)
	owner := loginTok(t, r, "admin", "adminpass")
	if w := authdo(r, "PUT", "/api/v1/features", `{"impersonation":true}`, owner); w.Code != 200 {
		t.Fatalf("enable flag: %d", w.Code)
	}
	dave := createUserAs(t, r, owner, `{"username":"dave","password":"davepassword","role_ids":[]}`)

	// Owner starts impersonating dave.
	w := authdo(r, "POST", "/api/v1/users/"+dave+"/impersonate", "", owner)
	if w.Code != 200 {
		t.Fatalf("impersonate dave: %d, body=%s", w.Code, w.Body.String())
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &tok)

	// The impersonation session is live.
	if u, imp := meAs(t, r, tok.AccessToken); u != "dave" || imp != "admin" {
		t.Fatalf("me as impersonated: user=%q impersonator=%q, want dave/admin", u, imp)
	}

	// Admin force-logs-out dave.
	if w := authdo(r, "POST", "/api/v1/users/"+dave+"/logout", "", owner); w.Code != 204 {
		t.Fatalf("force-logout dave: %d", w.Code)
	}

	// The impersonation token is now rejected immediately.
	if w := authdo(r, "GET", "/api/v1/auth/me", "", tok.AccessToken); w.Code != 401 {
		t.Errorf("impersonation token after target force-logout: got %d, want 401", w.Code)
	}
}

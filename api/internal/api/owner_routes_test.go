package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// authdo issues a request with a bearer token.
func authdo(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func loginTok(t *testing.T, r *gin.Engine, user, pass string) string {
	t.Helper()
	w := do(r, "POST", "/api/v1/auth/login", `{"username":"`+user+`","password":"`+pass+`"}`)
	if w.Code != 200 {
		t.Fatalf("login %s: got %d, body=%s", user, w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Fatalf("login %s: no access token", user)
	}
	return resp.AccessToken
}

func adminRoleID(t *testing.T, r *gin.Engine, token string) string {
	t.Helper()
	w := authdo(r, "GET", "/api/v1/roles", "", token)
	if w.Code != 200 {
		t.Fatalf("list roles: %d", w.Code)
	}
	var roles []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Superuser bool   `json:"superuser"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &roles)
	for _, role := range roles {
		if role.Superuser {
			return role.ID
		}
	}
	t.Fatal("no superuser role found")
	return ""
}

func TestOwnerOnlyGatesAndSuperuserGuard(t *testing.T) {
	r := newTestRouter(t) // bootstrap admin is the owner
	owner := loginTok(t, r, "admin", "adminpass")
	adminRole := adminRoleID(t, r, owner)

	// Owner may read an owner-only route.
	if w := authdo(r, "GET", "/api/v1/security/config", "", owner); w.Code != 200 {
		t.Fatalf("owner GET /security/config: got %d", w.Code)
	}

	// Owner creates a second admin (allowed for the owner).
	if w := authdo(r, "POST", "/api/v1/users",
		`{"username":"bob","password":"bobpassword","role_ids":["`+adminRole+`"]}`, owner); w.Code != 201 {
		t.Fatalf("owner create admin: got %d, body=%s", w.Code, w.Body.String())
	}
	bob := loginTok(t, r, "bob", "bobpassword") // admin, but NOT the owner

	// Non-owner admin is blocked from the owner-only route…
	if w := authdo(r, "GET", "/api/v1/security/config", "", bob); w.Code != 403 {
		t.Errorf("non-owner GET /security/config: got %d, want 403", w.Code)
	}
	// …and from minting another admin…
	if w := authdo(r, "POST", "/api/v1/users",
		`{"username":"carol","password":"carolpassword","role_ids":["`+adminRole+`"]}`, bob); w.Code != 403 {
		t.Errorf("non-owner create admin: got %d, want 403", w.Code)
	}
	// …but may still create a plain (non-admin) user.
	if w := authdo(r, "POST", "/api/v1/users",
		`{"username":"dave","password":"davepassword","role_ids":[]}`, bob); w.Code != 201 {
		t.Errorf("non-owner create plain user: got %d, want 201, body=%s", w.Code, w.Body.String())
	}
}

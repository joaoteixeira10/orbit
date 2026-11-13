package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"

	"github.com/gin-gonic/gin"
)

func scimDo(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/scim+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestSCIM: the identity provider makes, changes and deactivates accounts
// and groups through SCIM — only its own — and its people's first SSO
// sign-in finds the account it made.
func TestSCIM(t *testing.T) {
	old := auth.OIDCClient
	auth.OIDCClient = func() *http.Client { return &http.Client{Timeout: 10 * time.Second} }
	defer func() { auth.OIDCClient = old }()
	r, db, owner, alice, _ := plannerSetupDB(t)

	expect(t, scimDo(r, "GET", "/scim/v2/Users", "", "nope"), http.StatusUnauthorized, "SCIM off")
	var cfg struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	decode(t, authdo(r, "PUT", "/api/v1/scim-config", `{"enabled":true,"new_token":true,"default_role":"member"}`, owner), &cfg)
	if !strings.HasPrefix(cfg.Token, "scim_") || !strings.HasSuffix(cfg.URL, "/scim/v2") {
		t.Fatalf("config: %+v", cfg)
	}
	tok := cfg.Token
	expect(t, scimDo(r, "GET", "/scim/v2/Users", "", "scim_wrong"), http.StatusUnauthorized, "wrong token")
	expect(t, scimDo(r, "GET", "/scim/v2/ServiceProviderConfig", "", tok), http.StatusOK, "config")

	// An account made; its userName unique; a local account never taken over.
	w := scimDo(r, "POST", "/scim/v2/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"erin","externalId":"e-1","name":{"givenName":"Erin","familyName":"Cole"},"emails":[{"value":"erin@corp.test","primary":true}],"active":true}`, tok)
	expect(t, w, http.StatusCreated, "create")
	var u struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Active      bool   `json:"active"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &u)
	if u.ID == "" || u.DisplayName != "Erin Cole" || !u.Active {
		t.Fatalf("user: %s", w.Body.String())
	}
	expect(t, scimDo(r, "POST", "/scim/v2/Users", `{"userName":"erin"}`, tok), http.StatusConflict, "same userName")
	expect(t, scimDo(r, "POST", "/scim/v2/Users", `{"userName":"alice"}`, tok), http.StatusConflict, "a local account")
	expect(t, scimDo(r, "GET", "/scim/v2/Users/"+meID(t, r, alice), "", tok), http.StatusNotFound, "not SCIM's")
	if b := scimDo(r, "GET", `/scim/v2/Users?filter=userName%20eq%20%22Erin%22`, "", tok).Body.String(); !strings.Contains(b, `"totalResults":1`) {
		t.Fatalf("filter: %s", b)
	}

	// A group with Erin; in alice's workspace, so Erin is.
	w = scimDo(r, "POST", "/scim/v2/Groups", `{"displayName":"Sales","externalId":"g-1","members":[{"value":"`+u.ID+`"}]}`, tok)
	expect(t, w, http.StatusCreated, "group")
	var g struct{ ID string }
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	ws := createWS(t, r, alice, `{"name":"Deals","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/groups", `{"group_id":"`+g.ID+`","role":"viewer"}`, alice), http.StatusOK, "sales in")
	if m := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", alice).Body.String(); !strings.Contains(m, `"username":"erin"`) {
		t.Fatalf("erin not in: %s", m)
	}
	expect(t, authdo(r, "POST", "/api/v1/groups/"+g.ID+"/members", `{"user":"alice"}`, owner), http.StatusConflict, "kept by the IdP")

	// Erin's first SSO sign-in is her SCIM account.
	idp := newFakeIdP(t)
	setProviders(t, db, []auth.OIDCSettings{{ID: "default", Name: "Corp", Kind: "oidc", Enabled: true, Issuer: idp.srv.URL, ClientID: "orbit", RedirectURL: "http://orbit.test/api/v1/auth/oidc/callback"}})
	erin, fail := idp.signIn(t, r, "default", map[string]any{"sub": "idp-erin", "email": "erin@corp.test", "email_verified": true, "preferred_username": "erin.cole"})
	if fail != "" || meID(t, r, erin) != u.ID {
		t.Fatalf("sso: %q, %s vs %s", fail, meID(t, r, erin), u.ID)
	}

	// Taken out of the group (Entra's path form), then deactivated: out, signed out.
	expect(t, scimDo(r, "PATCH", "/scim/v2/Groups/"+g.ID, `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"remove","path":"members[value eq \"`+u.ID+`\"]"}]}`, tok), http.StatusOK, "removed")
	if m := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", alice).Body.String(); strings.Contains(m, `"username":"erin"`) {
		t.Fatalf("erin still in: %s", m)
	}
	expect(t, scimDo(r, "PATCH", "/scim/v2/Users/"+u.ID, `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"Replace","path":"active","value":"False"}]}`, tok), http.StatusOK, "deactivated")
	expect(t, authdo(r, "GET", "/api/v1/auth/me", "", erin), http.StatusUnauthorized, "signed out")
	expect(t, scimDo(r, "DELETE", "/scim/v2/Users/"+u.ID, "", tok), http.StatusNoContent, "deleted (disabled)")
	expect(t, scimDo(r, "DELETE", "/scim/v2/Groups/"+g.ID, "", tok), http.StatusNoContent, "group deleted")
	if gs := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/groups", "", alice).Body.String(); gs != "[]" {
		t.Fatalf("group left: %s", gs)
	}
}

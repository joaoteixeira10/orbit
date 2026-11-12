package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestGroups: a group's people get its instance roles and its workspaces
// (with its role there, or their own if higher); joining or leaving does it
// all at once; a group that makes admins is the owner's; no change leaves a
// workspace without an owner.
func TestGroups(t *testing.T) {
	r, _, owner, alice, bob := plannerSetupDB(t)
	newID := func(code int, body string, what string) string {
		t.Helper()
		var x struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal([]byte(body), &x)
		if code >= 300 || x.ID == "" {
			t.Fatalf("%s: %d %s", what, code, body)
		}
		return x.ID
	}

	// A role that reads the audit log, carried by a group.
	w := authdo(r, "POST", "/api/v1/roles", `{"name":"auditors","global_privileges":["audit.read"]}`, owner)
	role := newID(w.Code, w.Body.String(), "role")
	w = authdo(r, "POST", "/api/v1/groups", `{"name":"Team","role_ids":["`+role+`"]}`, owner)
	team := newID(w.Code, w.Body.String(), "group")
	expect(t, authdo(r, "GET", "/api/v1/audit", "", bob), http.StatusForbidden, "bob before")
	expect(t, authdo(r, "POST", "/api/v1/groups/"+team+"/members", `{"user":"bob"}`, owner), http.StatusOK, "bob in")
	expect(t, authdo(r, "GET", "/api/v1/audit", "", bob), http.StatusOK, "bob through the group")

	// Making admins is the owner's: an admin can't give a group an admin role,
	// nor change a group that has one.
	createUserVia(t, r, owner, `{"username":"adam","password":"adampassword1","role_ids":["`+adminRoleID(t, r, owner)+`"]}`)
	adam := loginTok(t, r, "adam", "adampassword1")
	expect(t, authdo(r, "POST", "/api/v1/groups", `{"name":"Admins","role_ids":["`+adminRoleID(t, r, owner)+`"]}`, adam), http.StatusForbidden, "admin role by an admin")
	w = authdo(r, "POST", "/api/v1/groups", `{"name":"Admins","role_ids":["`+adminRoleID(t, r, owner)+`"]}`, owner)
	admins := newID(w.Code, w.Body.String(), "admins group")
	expect(t, authdo(r, "POST", "/api/v1/groups/"+admins+"/members", `{"user":"alice"}`, adam), http.StatusForbidden, "admin adds to an admin group")
	expect(t, authdo(r, "POST", "/api/v1/groups/"+admins+"/members", `{"user":"alice"}`, owner), http.StatusOK, "owner makes alice an admin")
	// Alice, an admin through a group, counts as one: another admin can't manage her.
	var aliceID string
	for _, u := range strings.Split(authdo(r, "GET", "/api/v1/users", "", owner).Body.String(), "},{") {
		if strings.Contains(u, `"username":"alice"`) {
			_, rest, _ := strings.Cut(u, `"id":"`)
			aliceID, _, _ = strings.Cut(rest, `"`)
		}
	}
	expect(t, authdo(r, "PUT", "/api/v1/users/"+aliceID, `{"display_name":"Al"}`, adam), http.StatusForbidden, "admin edits an admin-by-group")
	expect(t, authdo(r, "DELETE", "/api/v1/groups/"+admins+"/members/"+aliceID, "", owner), http.StatusOK, "alice out")

	// A workspace of alice's; the group in it as editors: bob is in.
	ws := createWS(t, r, alice, `{"name":"Shop","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", bob), http.StatusNotFound, "bob not in yet")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/groups", `{"group_id":"`+team+`","role":"editor"}`, bob), http.StatusNotFound, "bob can't add groups")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/groups", `{"group_id":"`+team+`","role":"editor"}`, alice), http.StatusOK, "team in")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"By bob"}`, bob), http.StatusCreated, "bob edits")
	members := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", alice).Body.String()
	if !strings.Contains(members, `"username":"bob"`) || !strings.Contains(members, `"groups":["Team"]`) || !strings.Contains(members, `"direct_role":""`) {
		t.Fatalf("members: %s", members)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/members/"+meID(t, r, bob), "", alice), http.StatusConflict, "only through the group")

	// A role of his own too (viewer): the higher one counts; the group out, his own stays.
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob's own")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Still"}`, bob), http.StatusCreated, "editor through the group")
	expect(t, authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/groups/"+team, "", alice), http.StatusOK, "team out")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"No"}`, bob), http.StatusForbidden, "a viewer now")
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", bob), http.StatusOK, "still in (his own)")

	// The group as owners, alice's own role down to editor: taking the group
	// out would leave no owner — refused, nothing changed.
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/groups", `{"group_id":"`+team+`","role":"owner"}`, alice), http.StatusOK, "team as owners")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+meID(t, r, alice), `{"role":"editor"}`, alice), http.StatusOK, "alice an editor")
	expect(t, authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/groups/"+team, "", bob), http.StatusConflict, "no owner left")
	expect(t, authdo(r, "DELETE", "/api/v1/groups/"+team+"/members/"+meID(t, r, bob), "", owner), http.StatusConflict, "no owner left (leaving the group)")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+meID(t, r, alice), `{"role":"owner"}`, bob), http.StatusOK, "alice an owner again")

	// Leaving the group: its roles and workspaces go; the group deleted, all of it.
	expect(t, authdo(r, "DELETE", "/api/v1/groups/"+team+"/members/"+meID(t, r, bob), "", owner), http.StatusOK, "bob leaves the group")
	expect(t, authdo(r, "GET", "/api/v1/audit", "", bob), http.StatusForbidden, "no audit without the group")
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", bob), http.StatusOK, "his own role stays")
	expect(t, authdo(r, "DELETE", "/api/v1/groups/"+team, "", owner), http.StatusNoContent, "group deleted")
	if g := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/groups", "", alice).Body.String(); g != "[]" {
		t.Fatalf("groups left: %s", g)
	}
}

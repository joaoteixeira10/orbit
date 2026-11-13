package api

import (
	"net/http"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

type inviteMade struct {
	Invite struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	} `json:"invite"`
	Token string `json:"token"`
	Path  string `json:"path"`
}

func makeInvite(t *testing.T, r *gin.Engine, tok, wsID, body string) inviteMade {
	t.Helper()
	w := authdo(r, "POST", "/api/v1/workspaces/"+wsID+"/invites", body, tok)
	expect(t, w, http.StatusCreated, "create invite")
	var inv inviteMade
	decode(t, w, &inv)
	if len(inv.Token) < 30 || inv.Path != "/join/"+inv.Token {
		t.Fatalf("invite: %+v", inv)
	}
	return inv
}

// TestInvites: an owner makes a link; anyone can see what it's for; a
// signed-in person joins with its role; it's used up after its uses; only
// owners make, list and revoke links; a revoked link stops working.
func TestInvites(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Team"}`)

	// Bob isn't an owner (not even a member): no links for him.
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/invites", `{"role":"editor"}`, bob), http.StatusNotFound, "non-member creates")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/invites", `{"role":"owner"}`, alice), http.StatusBadRequest, "owner role")

	inv := makeInvite(t, r, alice, ws.ID, `{"role":"viewer"}`)

	// The preview is public.
	w := do(r, "GET", "/api/v1/invites/"+inv.Token, "")
	expect(t, w, http.StatusOK, "preview")
	var pv struct {
		Workspace string `json:"workspace"`
		Role      string `json:"role"`
		InvitedBy string `json:"invited_by"`
		CanSignUp bool   `json:"can_sign_up"`
	}
	decode(t, w, &pv)
	if pv.Workspace != "Team" || pv.Role != "viewer" || pv.InvitedBy != "alice" || !pv.CanSignUp {
		t.Fatalf("preview: %+v", pv)
	}
	expect(t, do(r, "GET", "/api/v1/invites/nope", ""), http.StatusNotFound, "unknown token")

	// Bob joins as a viewer; the link (one use) is then spent.
	expect(t, authdo(r, "POST", "/api/v1/invites/"+inv.Token+"/accept", "", bob), http.StatusOK, "accept")
	var members []struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", alice), &members)
	found := false
	for _, m := range members {
		if m.Username == "bob" {
			found = m.Role == "viewer"
		}
	}
	if !found {
		t.Fatalf("bob not a viewer: %+v", members)
	}
	expect(t, do(r, "GET", "/api/v1/invites/"+inv.Token, ""), http.StatusNotFound, "spent link")
	expect(t, authdo(r, "POST", "/api/v1/invites/"+inv.Token+"/accept", "", bob), http.StatusNotFound, "accept spent link")

	// A viewer can't make or list links.
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/invites", `{"role":"editor"}`, bob), http.StatusForbidden, "viewer creates")
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/invites", "", bob), http.StatusForbidden, "viewer lists")

	// An editor link raises Bob from viewer to editor (it never lowers).
	ed := makeInvite(t, r, alice, ws.ID, `{"role":"editor","max_uses":5,"days":3}`)
	w = authdo(r, "POST", "/api/v1/invites/"+ed.Token+"/accept", "", bob)
	expect(t, w, http.StatusOK, "accept editor")
	var acc struct {
		Already bool `json:"already_member"`
	}
	decode(t, w, &acc)
	if !acc.Already {
		t.Fatal("bob should already be a member")
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", alice), &members)
	for _, m := range members {
		if m.Username == "bob" && m.Role != "editor" {
			t.Fatalf("bob's role: %s", m.Role)
		}
	}

	// Bob, already an editor, opens it again: he's in, and it isn't used.
	expect(t, authdo(r, "POST", "/api/v1/invites/"+ed.Token+"/accept", "", bob), http.StatusOK, "accept again")

	// Owners list live links; revoking one ends it.
	var live []struct {
		ID   string `json:"id"`
		Uses int    `json:"uses"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/invites", "", alice), &live)
	if len(live) != 1 || live[0].ID != ed.Invite.ID || live[0].Uses != 1 {
		t.Fatalf("live links: %+v", live)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/invites/"+ed.Invite.ID, "", alice), http.StatusNoContent, "revoke")
	expect(t, do(r, "GET", "/api/v1/invites/"+ed.Token, ""), http.StatusNotFound, "revoked link")
	expect(t, authdo(r, "DELETE", "/api/v1/workspaces/"+ws.ID+"/invites/"+ed.Invite.ID, "", alice), http.StatusNotFound, "revoke twice")
}

// TestInviteSignup: someone without an account makes one through a link and
// is signed in, a member; usernames are checked; the instance's switches
// (the flag, a hidden password form) close it; a link of an owner who no
// longer is one stops working.
func TestInviteSignup(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	ws := createWS(t, r, admin, `{"name":"Class 10B"}`)
	inv := makeInvite(t, r, admin, ws.ID, `{"role":"editor","max_uses":3}`)

	expect(t, do(r, "POST", "/api/v1/invites/"+inv.Token+"/signup", `{"username":"Ab","password":"longenough"}`), http.StatusBadRequest, "short username")
	expect(t, do(r, "POST", "/api/v1/invites/"+inv.Token+"/signup", `{"username":"carol","password":"short"}`), http.StatusBadRequest, "short password")
	expect(t, do(r, "POST", "/api/v1/invites/"+inv.Token+"/signup", `{"username":"admin","password":"longenough"}`), http.StatusConflict, "taken username")

	w := do(r, "POST", "/api/v1/invites/"+inv.Token+"/signup", `{"username":"Carol","display_name":"Carol M","password":"carolpassword"}`)
	expect(t, w, http.StatusCreated, "signup")
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, w, &tok)
	if tok.AccessToken == "" {
		t.Fatalf("no token: %s", w.Body.String())
	}
	var mine []wsResp
	decode(t, authdo(r, "GET", "/api/v1/workspaces", "", tok.AccessToken), &mine)
	ok := false
	for _, x := range mine {
		ok = ok || (x.ID == ws.ID && x.Role == "editor")
	}
	if !ok {
		t.Fatalf("carol's workspaces: %+v", mine)
	}
	// She signs in with the lowercased name, and has the member role (not admin).
	carol := loginTok(t, r, "carol", "carolpassword")
	expect(t, authdo(r, "GET", "/api/v1/users", "", carol), http.StatusForbidden, "carol is no admin")

	// A hidden password form: no accounts through links.
	db.Create(&models.Setting{Key: settingHideLocalLogin, Value: "true"})
	expect(t, do(r, "POST", "/api/v1/invites/"+inv.Token+"/signup", `{"username":"dave","password":"davepassword"}`), http.StatusForbidden, "hidden login")
	db.Where("key = ?", settingHideLocalLogin).Delete(&models.Setting{})

	// The flag off: links are gone.
	expect(t, authdo(r, "PUT", "/api/v1/features", `{"workspace_invites":false}`, admin), http.StatusOK, "flag off")
	expect(t, do(r, "GET", "/api/v1/invites/"+inv.Token, ""), http.StatusNotFound, "flag off preview")
	expect(t, authdo(r, "PUT", "/api/v1/features", `{"workspace_invites":true}`, admin), http.StatusOK, "flag on")
	expect(t, do(r, "GET", "/api/v1/invites/"+inv.Token, ""), http.StatusOK, "flag on preview")

	// The admin makes Carol an owner and steps down: the admin's link stops.
	var members []struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", admin), &members)
	var carolID, adminID string
	for _, m := range members {
		switch m.Username {
		case "carol":
			carolID = m.UserID
		case "admin":
			adminID = m.UserID
		}
	}
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+carolID, `{"role":"owner"}`, admin), http.StatusOK, "carol owner")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+adminID, `{"role":"viewer"}`, carol), http.StatusOK, "admin viewer")
	expect(t, do(r, "GET", "/api/v1/invites/"+inv.Token, ""), http.StatusNotFound, "former owner's link")
}

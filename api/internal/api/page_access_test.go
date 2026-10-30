package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

// TestPageRestrictions: a page open to only some people (and what's under
// it) is not there for the others — not in the tree, not by its address,
// not in search, Home, backlinks, the graph or an export; narrower below,
// never wider; whoever restricts it stays in; with nobody it allows left,
// the owners see it.
func TestPageRestrictions(t *testing.T) {
	r, db, admin, alice, bob := plannerSetupDB(t)
	createUserVia(t, r, admin, `{"username":"carol","password":"carolpassword"}`)
	carol := loginTok(t, r, "carol", "carolpassword")
	ws := createWS(t, r, alice, `{"name":"Team","kind":"work","starter":false,"modules":["home","pages"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	for _, u := range []string{"bob", "carol"} {
		expect(t, authdo(r, "POST", base+"/members", `{"user":"`+u+`","role":"editor"}`, alice), http.StatusCreated, u+" in")
	}
	var me struct {
		User struct{ ID string } `json:"user"`
		ID   string              `json:"id"`
	}
	idOf := func(tok string) string {
		decode(t, authdo(r, "GET", "/api/v1/auth/me", "", tok), &me)
		if me.User.ID != "" {
			return me.User.ID
		}
		return me.ID
	}
	bobID := idOf(bob)
	var hr, salaries, open struct{ ID string }
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"HR","content":"people stuff"}`, alice), &hr)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Salaries","parent_id":"`+hr.ID+`","content":"zebrafish numbers"}`, alice), &salaries)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Open","content":"see [Salaries](/w/`+ws.ID+`/p/`+salaries.ID+`)"}`, alice), &open)

	expect(t, authdo(r, "PUT", "/api/v1/pages/"+hr.ID+"/access", `{"restricted":true,"user_ids":["nobody"]}`, alice), http.StatusBadRequest, "not a member")
	var got struct {
		Restricted   bool     `json:"restricted"`
		AllowedUsers []string `json:"allowed_users"`
	}
	decode(t, authdo(r, "PUT", "/api/v1/pages/"+hr.ID+"/access", `{"restricted":true,"user_ids":["`+bobID+`"]}`, alice), &got)
	if !got.Restricted || len(got.AllowedUsers) != 2 {
		t.Fatalf("restricted: %+v", got)
	}

	titles := func(tok string) string {
		var ps []struct{ Title string }
		decode(t, authdo(r, "GET", base+"/pages", "", tok), &ps)
		var out []string
		for _, p := range ps {
			out = append(out, p.Title)
		}
		sort.Strings(out) // as a set: the engines order equal ranks differently
		return strings.Join(out, ",")
	}
	if titles(bob) != "HR,Open,Salaries" || titles(carol) != "Open" {
		t.Fatalf("trees: bob %q carol %q", titles(bob), titles(carol))
	}
	expect(t, authdo(r, "GET", "/api/v1/pages/"+salaries.ID, "", carol), http.StatusNotFound, "by address")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+salaries.ID, `{"title":"x"}`, carol), http.StatusNotFound, "can't edit")
	expect(t, authdo(r, "POST", base+"/pages", `{"title":"Sneak","parent_id":"`+hr.ID+`"}`, carol), http.StatusBadRequest, "can't put a page under it")
	expect(t, authdo(r, "GET", "/api/v1/pages/"+salaries.ID+"/versions", "", carol), http.StatusNotFound, "history")
	var hits []struct{ Title string }
	decode(t, authdo(r, "GET", "/api/v1/search?q=zebrafish", "", carol), &hits)
	if len(hits) != 0 {
		t.Fatalf("search: %+v", hits)
	}
	expect(t, authdo(r, "GET", "/api/v1/pages/"+salaries.ID+"/backlinks", "", carol), http.StatusNotFound, "backlinks")
	var bl struct{ Pages []struct{ Title string } }
	decode(t, authdo(r, "GET", "/api/v1/pages/"+open.ID+"/backlinks", "", bob), &bl)
	var g struct{ Nodes []struct{ Title string } }
	decode(t, authdo(r, "GET", base+"/graph", "", carol), &g)
	for _, n := range g.Nodes {
		if n.Title == "Salaries" || n.Title == "HR" {
			t.Fatalf("graph shows %s", n.Title)
		}
	}

	// Narrower below: Salaries for Alice only — Bob loses it, keeps HR.
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+salaries.ID+"/access", `{"restricted":true}`, alice), http.StatusOK, "narrower")
	if titles(bob) != "HR,Open" {
		t.Fatalf("narrower: %q", titles(bob))
	}
	expect(t, authdo(r, "GET", base+"/transfer", "", alice), http.StatusOK, "export")

	// Nobody it allows is left: the owners see it.
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+hr.ID+"/access", `{"restricted":false}`, alice), http.StatusOK, "open again")
	if titles(carol) != "HR,Open" {
		t.Fatalf("open again: %q", titles(carol))
	}
	ids, _ := json.Marshal([]string{"gone-user"})
	db.Exec("UPDATE pages SET allowed_users = ? WHERE id = ?", string(ids), salaries.ID)
	if titles(alice) != "HR,Open,Salaries" || titles(bob) != "HR,Open" {
		t.Fatalf("orphaned: alice %q bob %q", titles(alice), titles(bob))
	}
}

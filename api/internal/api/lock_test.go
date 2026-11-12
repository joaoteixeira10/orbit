package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// TestLockNotesAndPages: a PIN's key kept wrapped; a note and a page locked —
// their text gone from Orbit (search, history, editing together), kept only
// sealed; only who locked them changes them; forgetting the PIN deletes them.
func TestLockNotesAndPages(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Home","kind":"personal","starter":false,"modules":["home","notes","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	b64 := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	sealedText := `{"v":1,"iv":"AAAAAAAAAAAAAAAA","ct":"c2VhbGVk"}`

	// No PIN yet: nothing can be locked.
	var n struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/notes", `{"body":"bank pin 4321 marmalade"}`, alice), &n)
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`}`, alice), http.StatusBadRequest, "no PIN")
	expect(t, authdo(r, "PUT", "/api/v1/me/lock", `{"salt":"`+b64(16)+`","iterations":1000,"iv":"`+b64(12)+`","wrapped":"`+b64(48)+`"}`, alice), http.StatusBadRequest, "too few rounds")
	expect(t, authdo(r, "PUT", "/api/v1/me/lock", `{"salt":"`+b64(16)+`","iterations":600000,"iv":"`+b64(12)+`","wrapped":"`+b64(48)+`"}`, alice), http.StatusOK, "PIN set")
	if got := authdo(r, "GET", "/api/v1/me/lock", "", alice).Body.String(); !strings.Contains(got, `"set":true`) || !strings.Contains(got, `"iterations":600000`) {
		t.Fatalf("lock key: %s", got)
	}

	// The note: locked, its text only sealed, out of search.
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`,"body":"x"}`, alice), http.StatusBadRequest, "text with the lock")
	w := authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`}`, alice)
	expect(t, w, http.StatusOK, "locked")
	if b := w.Body.String(); strings.Contains(b, "marmalade") || !strings.Contains(b, `"sealed":true`) {
		t.Fatalf("locked note: %s", b)
	}
	if got := authdo(r, "GET", "/api/v1/search?q=marmalade", "", alice).Body.String(); strings.Contains(got, n.ID) {
		t.Fatalf("found by its old text: %s", got)
	}
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"body":"plain again"}`, alice), http.StatusBadRequest, "text not sealed")
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"cipher":`+jsonString(sealedText)+`,"pinned":true}`, alice), http.StatusOK, "sealed text changed")
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"pinned":false}`, bob), http.StatusForbidden, "someone else's")
	expect(t, authdo(r, "DELETE", "/api/v1/notes/"+n.ID, "", bob), http.StatusForbidden, "someone else deletes")

	// The page: locked — no history, not edited together; only sealed text.
	var p struct {
		ID      string
		Version int
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Diary","content":"walnut secret"}`, alice), &p)
	decode(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"content":"walnut secret, more","version":1}`, alice), &p)
	w = authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`,"version":2}`, alice)
	expect(t, w, http.StatusOK, "page locked")
	decode(t, w, &p)
	if b := authdo(r, "GET", "/api/v1/pages/"+p.ID, "", alice).Body.String(); strings.Contains(b, "walnut") || !strings.Contains(b, `"sealed":true`) {
		t.Fatalf("locked page: %s", b)
	}
	if b := authdo(r, "GET", "/api/v1/pages/"+p.ID+"/versions", "", alice).Body.String(); strings.Contains(b, "walnut") {
		t.Fatalf("history kept the text: %s", b)
	}
	if got := authdo(r, "GET", "/api/v1/search?q=walnut", "", alice).Body.String(); strings.Contains(got, p.ID) {
		t.Fatalf("page found by its old text: %s", got)
	}
	expect(t, authdo(r, "GET", "/api/v1/collab/page/"+p.ID, "", alice), http.StatusConflict, "not together")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"content":"plain","version":3}`, alice), http.StatusBadRequest, "page text not sealed")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"title":"Mine now"}`, bob), http.StatusForbidden, "someone else's page")
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+p.ID, "", bob), http.StatusForbidden, "someone else trashes it")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"cipher":`+jsonString(sealedText)+`,"version":3}`, alice), http.StatusOK, "sealed page text changed")
	var tpl struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"T","template":true}`, alice), &tpl)
	authdo(r, "PUT", "/api/v1/pages/"+tpl.ID, `{"template":true}`, alice)
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+tpl.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`,"version":1}`, alice), http.StatusBadRequest, "a template")

	// Opened for good: readable again.
	if b := authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lock":false,"body":"open text"}`, alice).Body.String(); !strings.Contains(b, "open text") || strings.Contains(b, `"sealed":true`) {
		t.Fatalf("opened note: %s", b)
	}

	// The PIN forgotten: what it locked goes (the page to the trash).
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lock":true,"cipher":`+jsonString(sealedText)+`}`, alice), http.StatusOK, "locked again")
	expect(t, authdo(r, "DELETE", "/api/v1/me/lock", "", alice), http.StatusNoContent, "forgotten")
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"pinned":true}`, alice), http.StatusNotFound, "the note is gone")
	if b := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/trash", "", alice).Body.String(); !strings.Contains(b, p.ID) {
		t.Fatalf("page not in the trash: %s", b)
	}
	if got := authdo(r, "GET", "/api/v1/me/lock", "", alice).Body.String(); !strings.Contains(got, `"set":false`) {
		t.Fatalf("still a key: %s", got)
	}
}

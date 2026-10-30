package api

import (
	"net/http"
	"strconv"
	"testing"
)

// TestUndoPageChanges: a person's own changes to pages are listed and undone
// — made (to the trash), moved (back), renamed (back), copied (the copy to
// the trash), trashed (out again); one changed since asks first; one undone
// isn't again; others' changes aren't theirs to undo.
func TestUndoPageChanges(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Notes","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	base := "/api/v1/workspaces/" + ws.ID
	var a, b struct {
		ID      string
		Version int
	}
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Alpha"}`, alice), &a)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Beta"}`, alice), &b)
	expect(t, authdo(r, "POST", "/api/v1/pages/"+b.ID+"/move", `{"parent_id":"`+a.ID+`","after_id":""}`, alice), http.StatusOK, "move")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+a.ID, `{"title":"Alpha 2"}`, alice), http.StatusOK, "rename")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+a.ID, `{"title":"Alpha 3"}`, alice), http.StatusOK, "rename again (the same change)")

	type change struct {
		ID, Kind, Title, FromTitle string
		Undone, Changed            bool
	}
	list := func(tok string) []change {
		var cs []struct {
			ID        string `json:"id"`
			Kind      string `json:"kind"`
			Title     string `json:"title"`
			FromTitle string `json:"from_title"`
			Undone    bool   `json:"undone"`
			Changed   bool   `json:"changed"`
		}
		decode(t, authdo(r, "GET", "/api/v1/me/page-changes?ws="+ws.ID, "", tok), &cs)
		out := []change{}
		for _, c := range cs {
			out = append(out, change{ID: c.ID, Kind: c.Kind, Title: c.Title, FromTitle: c.FromTitle, Undone: c.Undone, Changed: c.Changed})
		}
		return out
	}
	cs := list(alice)
	kinds := ""
	for _, c := range cs {
		kinds += c.Kind + ","
	}
	if kinds != "rename,move,create,create," || cs[0].FromTitle != "Alpha" || cs[0].Title != "Alpha 3" {
		t.Fatalf("changes: %+v", cs)
	}
	if len(list(bob)) != 0 {
		t.Fatal("bob has none")
	}
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[0].ID+"/undo", "", bob), http.StatusNotFound, "not bob's")

	// Rename back; the move back.
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[0].ID+"/undo", "", alice), http.StatusOK, "undo rename")
	var pa struct {
		Title    string
		ParentID *string `json:"parent_id"`
		Version  int
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+a.ID, "", alice), &pa)
	if pa.Title != "Alpha" {
		t.Fatalf("rename undone: %q", pa.Title)
	}
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[0].ID+"/undo", "", alice), http.StatusConflict, "twice")
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[1].ID+"/undo", "", alice), http.StatusOK, "undo move")
	decode(t, authdo(r, "GET", "/api/v1/pages/"+b.ID, "", alice), &pa)
	if pa.ParentID != nil {
		t.Fatalf("move undone: %v", *pa.ParentID)
	}

	// A page made, then changed by someone else: undoing asks first.
	decode(t, authdo(r, "GET", "/api/v1/pages/"+b.ID, "", bob), &pa)
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+b.ID, `{"content":"bob wrote","version":`+strconv.Itoa(pa.Version)+`}`, bob), http.StatusOK, "bob writes")
	var createB string
	for _, c := range list(alice) {
		if c.Kind == "create" && c.Title == "Beta" {
			createB = c.ID
			if !c.Changed {
				t.Fatalf("changed since: %+v", c)
			}
		}
	}
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+createB+"/undo", "", alice), http.StatusConflict, "asks first")
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+createB+"/undo?force=1", "", alice), http.StatusOK, "anyway")
	expect(t, authdo(r, "GET", "/api/v1/pages/"+b.ID, "", alice), http.StatusOK, "in the trash, still there")
	var trash []struct{ ID string }
	decode(t, authdo(r, "GET", base+"/trash", "", alice), &trash)
	if len(trash) != 1 || trash[0].ID != b.ID {
		t.Fatalf("trash: %+v", trash)
	}

	// Copy, and trash: undone.
	var cp struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/pages/"+a.ID+"/duplicate", "", alice), &cp)
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+a.ID, "", alice), http.StatusNoContent, "trash")
	cs = list(alice)
	if cs[0].Kind != "trash" || cs[1].Kind != "copy" {
		t.Fatalf("latest: %+v", cs[:2])
	}
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[0].ID+"/undo", "", alice), http.StatusOK, "undo trash")
	expect(t, authdo(r, "POST", "/api/v1/page-changes/"+cs[1].ID+"/undo", "", alice), http.StatusOK, "undo copy")
	decode(t, authdo(r, "GET", base+"/trash", "", alice), &trash)
	ids := map[string]bool{}
	for _, x := range trash {
		ids[x.ID] = true
	}
	if !ids[cp.ID] || ids[a.ID] {
		t.Fatalf("after: %+v", trash)
	}
}

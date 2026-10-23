package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"
)

type drawingResp struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Scene   string `json:"scene"`
	Preview string `json:"preview"`
	Version int    `json:"version"`
	NoteID  string `json:"note_id"`
}

// TestDrawings: a whiteboard is made, saved (a stale save refused), found by
// its words, listed with its preview; a note's drawing is its note's (and
// goes with it); a workspace's export carries them.
func TestDrawings(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Skip("no FTS5 here")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Studio","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID

	var d drawingResp
	w := authdo(r, "POST", base+"/drawings", `{"title":"Floor plan"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &d)
	if d.Version != 1 {
		t.Fatalf("new: %+v", d)
	}
	body := func(v int, scene, text string) string {
		b, _ := json.Marshal(map[string]any{"version": v, "scene": scene, "preview": `<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`, "text": text})
		return string(b)
	}
	scene := `{"type":"excalidraw","elements":[{"type":"rectangle","id":"r1"}]}`
	w = authdo(r, "PUT", "/api/v1/drawings/"+d.ID, body(1, scene, "kitchen window"), alice)
	expect(t, w, http.StatusOK, "save")
	decode(t, w, &d)
	if d.Version != 2 || d.Scene != scene {
		t.Fatalf("saved: %+v", d)
	}
	if w := authdo(r, "PUT", "/api/v1/drawings/"+d.ID, body(1, "{}", ""), alice); w.Code != http.StatusConflict {
		t.Errorf("a stale save: %d", w.Code)
	}
	// A preview that isn't an SVG isn't kept.
	w = authdo(r, "PUT", "/api/v1/drawings/"+d.ID, `{"version":2,"preview":"<script>alert(1)</script>"}`, alice)
	var bad drawingResp
	decode(t, w, &bad)
	if bad.Preview != "" || bad.Version != 3 {
		t.Errorf("a non-SVG preview kept: %+v", bad)
	}
	var list []drawingResp
	decode(t, authdo(r, "GET", base+"/drawings", "", alice), &list)
	if len(list) != 1 || list[0].Scene != "" || list[0].Title != "Floor plan" {
		t.Fatalf("list: %+v", list)
	}
	if w := authdo(r, "GET", "/api/v1/drawings/"+d.ID, "", bob); w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("an outsider reads it: %d", w.Code)
	}
	if found := authdo(r, "GET", "/api/v1/search?q=kitchen", "", alice).Body.String(); !strings.Contains(found, d.ID) || !strings.Contains(found, `"drawing"`) {
		t.Errorf("not found by its words: %s", found)
	}

	// A drawing on a personal note: only its owner's; deleted with it.
	var note struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/me/notes", `{"title":"Sketch"}`, alice), &note)
	var nd drawingResp
	w = authdo(r, "POST", "/api/v1/notes/"+note.ID+"/drawing", "", alice)
	expect(t, w, http.StatusOK, "note drawing")
	decode(t, w, &nd)
	var again drawingResp
	decode(t, authdo(r, "POST", "/api/v1/notes/"+note.ID+"/drawing", "", alice), &again)
	if nd.NoteID != note.ID || again.ID != nd.ID {
		t.Fatalf("note drawing: %+v / %+v", nd, again)
	}
	expect(t, authdo(r, "PUT", "/api/v1/drawings/"+nd.ID, body(1, scene, "a cat"), alice), http.StatusOK, "save note drawing")
	if w := authdo(r, "GET", "/api/v1/drawings/"+nd.ID, "", bob); w.Code == http.StatusOK {
		t.Error("someone else reads a personal note's drawing")
	}
	if !strings.Contains(authdo(r, "GET", "/api/v1/me/notes", "", alice).Body.String(), `"drawing":{"id":"`+nd.ID) {
		t.Error("the note doesn't list its drawing")
	}
	expect(t, authdo(r, "DELETE", "/api/v1/notes/"+note.ID, "", alice), http.StatusNoContent, "delete note")
	if w := authdo(r, "GET", "/api/v1/drawings/"+nd.ID, "", alice); w.Code != http.StatusNotFound {
		t.Errorf("the drawing outlived its note: %d", w.Code)
	}

	// The export carries the whiteboard.
	archive := authdo(r, "GET", base+"/transfer", "", alice).Body.Bytes()
	w = importWS(r, alice, archive, false)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/drawings", "", alice), &list)
	if len(list) != 1 || list[0].ID == d.ID || list[0].Title != "Floor plan" {
		t.Fatalf("imported: %+v", list)
	}
}

// TestQuickDraw: a sketch with no workspace is its owner's alone.
func TestQuickDraw(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	var d drawingResp
	w := authdo(r, "POST", "/api/v1/me/drawings", `{"title":"Napkin"}`, alice)
	expect(t, w, http.StatusCreated, "sketch")
	decode(t, w, &d)
	expect(t, authdo(r, "PUT", "/api/v1/drawings/"+d.ID, `{"version":1,"scene":"{}","text":"idea"}`, alice), http.StatusOK, "save")
	var mine []drawingResp
	decode(t, authdo(r, "GET", "/api/v1/me/drawings", "", alice), &mine)
	if len(mine) != 1 || mine[0].Title != "Napkin" {
		t.Fatalf("alice's sketches: %+v", mine)
	}
	decode(t, authdo(r, "GET", "/api/v1/me/drawings", "", bob), &mine)
	if len(mine) != 0 {
		t.Fatalf("bob sees: %+v", mine)
	}
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if w := authdo(r, m, "/api/v1/drawings/"+d.ID, `{"version":2}`, bob); w.Code != http.StatusNotFound {
			t.Errorf("bob %s alice's sketch: %d", m, w.Code)
		}
	}
	expect(t, authdo(r, "DELETE", "/api/v1/drawings/"+d.ID, "", alice), http.StatusNoContent, "delete")
}

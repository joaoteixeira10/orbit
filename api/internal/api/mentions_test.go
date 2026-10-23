package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestPageMentions: naming @bob in a page tells him once, linking to it; a
// page or a task that links to another is among its backlinks.
func TestPageMentions(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Team","kind":"work","starter":false}`)
	base := "/api/v1/workspaces/" + ws.ID
	expect(t, authdo(r, "POST", base+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")

	var target, notes pageResponse
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Target"}`, alice), &target)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Notes"}`, alice), &notes)
	save := func(content string, v int) {
		b, _ := json.Marshal(map[string]any{"content": content, "version": v})
		expect(t, authdo(r, "PUT", "/api/v1/pages/"+notes.ID, string(b), alice), http.StatusOK, "save")
	}
	link := "[Target](/w/" + ws.ID + "/p/" + target.ID + ")"
	save("- ask @bob about "+link+" on @2026-11-05", 1)
	mentioned := func() []string {
		var out []string
		for _, it := range inboxOf(t, r, bob).Items {
			if it.Verb == "mentioned" {
				out = append(out, it.Link)
			}
		}
		return out
	}
	got := mentioned()
	if len(got) != 1 || got[0] != "/w/"+ws.ID+"/p/"+notes.ID {
		t.Fatalf("bob's mentions: %+v", got)
	}
	// Saving again with the mention still there: not again. Alice never
	// hears of her own.
	save("- ask @bob about "+link+" on @2026-11-05\n- and @alice", 2)
	if len(mentioned()) != 1 {
		t.Errorf("told twice: %+v", mentioned())
	}
	for _, it := range inboxOf(t, r, alice).Items {
		if it.Verb == "mentioned" {
			t.Errorf("alice told of her own mention")
		}
	}

	// A task's notes link to the target too.
	expect(t, authdo(r, "POST", base+"/lists", `{"name":"Reading"}`, alice), http.StatusCreated, "list")
	expect(t, authdo(r, "POST", base+"/tasks", `{"title":"Read it","notes":"see /w/`+ws.ID+`/p/`+target.ID+`"}`, alice), http.StatusCreated, "task")
	var bl struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
		Tasks []struct {
			Title string `json:"title"`
		} `json:"tasks"`
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+target.ID+"/backlinks", "", bob), &bl)
	if len(bl.Pages) != 1 || bl.Pages[0].ID != notes.ID || len(bl.Tasks) != 1 || bl.Tasks[0].Title != "Read it" {
		t.Fatalf("backlinks: %+v", bl)
	}
	// A page in the trash links no more.
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+notes.ID, "", alice), http.StatusNoContent, "trash")
	decode(t, authdo(r, "GET", "/api/v1/pages/"+target.ID+"/backlinks", "", bob), &bl)
	if len(bl.Pages) != 0 {
		t.Errorf("trashed backlink: %+v", bl.Pages)
	}
}

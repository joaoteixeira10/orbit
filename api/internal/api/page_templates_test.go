package api

import (
	"net/http"
	"testing"
)

// TestPageTemplates: a page marked as a template says so in the tree (and
// back), without a new version of its document.
func TestPageTemplates(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Team","starter":false}`)
	var p pageResponse
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Standup {{date}}","content":"## Yesterday\n\n- "}`, alice), &p)
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"template":true}`, alice), http.StatusOK, "mark")
	var tree []struct {
		ID       string `json:"id"`
		Template bool   `json:"template"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &tree)
	if len(tree) != 1 || !tree[0].Template {
		t.Fatalf("tree: %+v", tree)
	}
	var got struct {
		Template bool   `json:"template"`
		Title    string `json:"title"`
		Version  int    `json:"version"`
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+p.ID, "", alice), &got)
	if !got.Template || got.Title != "Standup {{date}}" || got.Version != 1 {
		t.Fatalf("page: %+v", got)
	}
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+p.ID, `{"template":false}`, alice), http.StatusOK, "unmark")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &tree)
	if tree[0].Template {
		t.Error("still a template")
	}
}

package api

import (
	"net/http"
	"testing"
)

type smartListResp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Shared  bool   `json:"shared"`
	CanEdit bool   `json:"can_edit"`
	Filter  struct {
		Q        string   `json:"q"`
		Due      []string `json:"due"`
		Priority []int    `json:"priority"`
		Tags     []string `json:"tags"`
	} `json:"filter"`
}

// TestSmartLists: a smart list is its owner's until shared (by an editor);
// a shared one is changed by its owner or the workspace's owners; filters
// are checked; leaving takes your own with you.
func TestSmartLists(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Team","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID
	expect(t, authdo(r, "POST", base+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob in")

	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"  "}`, alice), http.StatusBadRequest, "no name")
	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"X","filter":{"due":["someday"]}}`, alice), http.StatusBadRequest, "bad bucket")
	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"X","filter":{"priority":[7]}}`, alice), http.StatusBadRequest, "bad priority")

	var mine smartListResp
	w := authdo(r, "POST", base+"/smart-lists", `{"name":"Urgent","color":"#FF3B30","filter":{"priority":[3],"due":["overdue","today"]}}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &mine)
	if !mine.CanEdit || mine.Shared || len(mine.Filter.Priority) != 1 || mine.Filter.Tags == nil {
		t.Fatalf("created: %+v", mine)
	}

	// Bob (a viewer) keeps his own, can't share, and doesn't see Alice's.
	var his smartListResp
	decode(t, authdo(r, "POST", base+"/smart-lists", `{"name":"Mine","filter":{"tags":["x"]}}`, bob), &his)
	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"Ours","shared":true}`, bob), http.StatusBadRequest, "viewer shares")
	var list []smartListResp
	decode(t, authdo(r, "GET", base+"/smart-lists", "", bob), &list)
	if len(list) != 1 || list[0].ID != his.ID {
		t.Fatalf("bob sees %+v", list)
	}
	expect(t, authdo(r, "PUT", "/api/v1/smart-lists/"+mine.ID, `{"name":"Mine now"}`, bob), http.StatusNotFound, "someone else's")

	// Shared: Bob sees it but can't change it.
	decode(t, authdo(r, "PUT", "/api/v1/smart-lists/"+mine.ID, `{"shared":true}`, alice), &mine)
	decode(t, authdo(r, "GET", base+"/smart-lists", "", bob), &list)
	if len(list) != 2 || list[0].CanEdit {
		t.Fatalf("bob sees %+v", list)
	}
	expect(t, authdo(r, "PUT", "/api/v1/smart-lists/"+mine.ID, `{"name":"Mine now"}`, bob), http.StatusForbidden, "shared, not his")
	expect(t, authdo(r, "DELETE", "/api/v1/smart-lists/"+mine.ID, "", bob), http.StatusForbidden, "delete shared")

	// Bob leaves: his own goes, the shared one stays.
	expect(t, authdo(r, "DELETE", base+"/members/"+meID(t, r, bob), "", alice), http.StatusNoContent, "bob out")
	decode(t, authdo(r, "GET", base+"/smart-lists", "", alice), &list)
	if len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("after bob left: %+v", list)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/smart-lists/"+mine.ID, "", alice), http.StatusNoContent, "delete")
}

// TestSmartListsTravel: export carries the shared smart lists and the
// exporter's own; on import their lists and people follow.
func TestSmartListsTravel(t *testing.T) {
	r, admin, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Travel","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID
	var l struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Errands"}`, alice), &l)
	aliceID := meID(t, r, alice)
	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"Shared","shared":true,"filter":{"lists":["`+l.ID+`"],"assignee":["`+aliceID+`","me"]}}`, alice), http.StatusCreated, "shared")
	expect(t, authdo(r, "POST", base+"/smart-lists", `{"name":"Own","filter":{"q":"milk"}}`, alice), http.StatusCreated, "own")

	w := authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, admin, w.Body.Bytes(), true)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	nb := "/api/v1/workspaces/" + got.Workspace.ID
	var lists []struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "GET", nb+"/lists", "", admin), &lists)
	var smart []struct {
		Name    string `json:"name"`
		CanEdit bool   `json:"can_edit"`
		Filter  struct {
			Q        string   `json:"q"`
			Lists    []string `json:"lists"`
			Assignee []string `json:"assignee"`
		} `json:"filter"`
	}
	decode(t, authdo(r, "GET", nb+"/smart-lists", "", admin), &smart)
	if len(smart) != 2 {
		t.Fatalf("imported smart lists: %+v", smart)
	}
	for _, s := range smart {
		switch s.Name {
		case "Shared":
			if len(s.Filter.Lists) != 1 || s.Filter.Lists[0] == l.ID || len(s.Filter.Assignee) != 2 || s.Filter.Assignee[0] != aliceID {
				t.Errorf("shared filter: %+v (lists %+v)", s.Filter, lists)
			}
		case "Own":
			if !s.CanEdit || s.Filter.Q != "milk" {
				t.Errorf("own: %+v", s)
			}
		}
	}
}

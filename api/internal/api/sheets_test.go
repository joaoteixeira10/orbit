package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/search"
)

type sheetResp struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Version int            `json:"version"`
	Data    map[string]any `json:"data"`
}

// TestSheets: a sheet is made, saved by version (a stale save is refused),
// found by its words, listed, carried by export / import, deleted.
func TestSheets(t *testing.T) {
	r, db, admin, alice, bob := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Skip("no FTS5")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Money","kind":"personal","starter":false,"modules":["home","sheets"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	expect(t, authdo(r, "POST", base+"/sheets", `{"title":"Budget"}`, bob), http.StatusNotFound, "not a member")
	var s sheetResp
	w := authdo(r, "POST", base+"/sheets", `{"title":"Budget","data":{"tabs":[{"id":"t1","name":"Sheet 1","cells":{"A1":{"v":"Rent"},"B1":{"v":"650"},"B3":{"v":"=SUM(B1:B2)"}}}]},"text":"Rent 650"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &s)
	expect(t, authdo(r, "PUT", "/api/v1/sheets/"+s.ID, `{"data":[1,2]}`, alice), http.StatusBadRequest, "not an object")
	var saved sheetResp
	decode(t, authdo(r, "PUT", "/api/v1/sheets/"+s.ID, `{"title":"Budget 2026","version":1,"text":"Rent 650 Food"}`, alice), &saved)
	if saved.Version != 2 || saved.Title != "Budget 2026" || saved.Data["tabs"] == nil {
		t.Fatalf("saved: %+v", saved)
	}
	expect(t, authdo(r, "PUT", "/api/v1/sheets/"+s.ID, `{"title":"stale","version":1}`, alice), http.StatusConflict, "stale save")

	var hits []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=food", "", alice), &hits)
	if len(hits) != 1 || hits[0].Kind != "sheet" || hits[0].ID != s.ID {
		t.Errorf("search: %+v", hits)
	}

	w = authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, admin, w.Body.Bytes(), false)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	var list []sheetResp
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/sheets", "", admin), &list)
	if len(list) != 1 || list[0].Title != "Budget 2026" || list[0].ID == s.ID {
		t.Fatalf("imported: %+v", list)
	}
	var full sheetResp
	decode(t, authdo(r, "GET", "/api/v1/sheets/"+list[0].ID, "", admin), &full)
	if !strings.Contains(string(mustJSON(full.Data)), `=SUM(B1:B2)`) {
		t.Errorf("imported data: %v", full.Data)
	}

	expect(t, authdo(r, "DELETE", "/api/v1/sheets/"+s.ID, "", alice), http.StatusNoContent, "delete")
	expect(t, authdo(r, "GET", "/api/v1/sheets/"+s.ID, "", alice), http.StatusNotFound, "gone")
}

func mustJSON(v any) []byte {
	var b bytes.Buffer
	_ = json.NewEncoder(&b).Encode(v)
	return b.Bytes()
}

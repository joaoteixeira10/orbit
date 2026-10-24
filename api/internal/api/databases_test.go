package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestDatabases: a page becomes a database (its properties checked); its
// sub-pages are rows whose values merge patch by patch; the tree carries
// both; Markdown export writes the table and each row's values.
func TestDatabases(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Library","kind":"personal","starter":false}`)
	base := "/api/v1/workspaces/" + ws.ID
	db := `{"props":[{"id":"st","name":"Status","type":"select","options":[{"id":"o1","name":"Read"}]},{"id":"n","name":"Pages","type":"number"}],"views":[{"id":"v1","name":"All","type":"table"}]}`
	expect(t, authdo(r, "POST", base+"/pages", `{"title":"Books","db":{"props":[{"id":"x","name":"X","type":"weird"}]}}`, alice), http.StatusBadRequest, "bad type")
	var books, dune pageResponse
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Books","db":`+db+`}`, alice), &books)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Dune","parent_id":"`+books.ID+`","props":{"st":"o1"}}`, alice), &dune)
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+dune.ID, `{"props":{"n":412}}`, alice), http.StatusOK, "patch")
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+dune.ID, `{"props":{"n":{"no":1}}}`, alice), http.StatusBadRequest, "bad value")

	var tree []struct {
		ID    string          `json:"id"`
		DB    json.RawMessage `json:"db"`
		Props map[string]any  `json:"props"`
	}
	decode(t, authdo(r, "GET", base+"/pages", "", alice), &tree)
	got := map[string]int{}
	for i, p := range tree {
		got[p.ID] = i
	}
	if b := tree[got[books.ID]]; !strings.Contains(string(b.DB), `"Status"`) {
		t.Errorf("books db: %s", b.DB)
	}
	if d := tree[got[dune.ID]]; d.Props["st"] != "o1" || d.Props["n"] != float64(412) {
		t.Errorf("dune props: %v", d.Props)
	}

	w := authdo(r, "GET", base+"/export", "", alice)
	expect(t, w, http.StatusOK, "export")
	zr, _ := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	if !strings.Contains(files["Books.md"], "| Dune | Read | 412 |") {
		t.Errorf("Books.md: %s", files["Books.md"])
	}
	if !strings.Contains(files["Books/Dune.md"], "Status: Read") {
		t.Errorf("Dune.md: %s", files["Books/Dune.md"])
	}

	// No longer a database: the rows stay as pages.
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+books.ID, `{"db":null}`, alice), http.StatusOK, "undo")
	var again pageResponse
	decode(t, authdo(r, "GET", "/api/v1/pages/"+books.ID, "", alice), &again)
	if len(again.DB) != 0 {
		t.Errorf("still a database: %s", again.DB)
	}
}

package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/search"
)

// TestPageTagsAndFields: any page takes tags (tidied: no "#", each once) and
// properties (a kind each, its value checked); the workspace lists its tags
// with how many pages have each, and its pages with one; search finds them
// by both; a copy keeps them; the Markdown export carries them as a YAML
// header.
func TestPageTagsAndFields(t *testing.T) {
	r, db, _, alice, _ := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Skip("no FTS5")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Notes","kind":"work","starter":false,"modules":["home","pages"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	var a, b struct{ ID string }
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Trip","content":"Pack bags"}`, alice), &a)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Budget"}`, alice), &b)

	expect(t, authdo(r, "PUT", "/api/v1/pages/"+a.ID, `{"tags":["#travel","Q3  plans","Travel"],"fields":[{"key":"Due","type":"date","value":"2026-11-04"},{"key":"Cost","type":"number","value":1200},{"key":"Booked","type":"checkbox","value":true},{"key":"Site","type":"url","value":"https://example.org"},{"key":"Hotel","type":"text","value":"Miramar"}]}`, alice), http.StatusOK, "tags and fields")
	for _, bad := range []string{
		`{"fields":[{"key":"Due","type":"date","value":"tomorrow"}]}`,
		`{"fields":[{"key":"A","type":"text","value":"x"},{"key":"a","type":"text","value":"y"}]}`,
		`{"fields":[{"key":"Site","type":"url","value":"javascript:alert(1)"}]}`,
		`{"fields":[{"key":"X","type":"color","value":"red"}]}`,
		`{"tags":["a,b"]}`,
	} {
		expect(t, authdo(r, "PUT", "/api/v1/pages/"+a.ID, bad, alice), http.StatusBadRequest, bad)
	}
	var got struct {
		Tags   []string
		Fields []struct {
			Key, Type string
			Value     any
		}
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+a.ID, "", alice), &got)
	if strings.Join(got.Tags, "|") != "travel|Q3 plans" || len(got.Fields) != 5 || got.Fields[1].Value != float64(1200) {
		t.Fatalf("saved: %+v", got)
	}
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+b.ID, `{"tags":["travel"]}`, alice), http.StatusOK, "b's tag")

	var tags []struct {
		Tag   string
		Count int
	}
	decode(t, authdo(r, "GET", base+"/tags", "", alice), &tags)
	if len(tags) != 2 || tags[0].Tag != "travel" || tags[0].Count != 2 || tags[1].Tag != "Q3 plans" {
		t.Fatalf("tags: %+v", tags)
	}
	var with []struct{ ID string }
	decode(t, authdo(r, "GET", base+"/pages?tag=q3%20plans", "", alice), &with)
	if len(with) != 1 || with[0].ID != a.ID {
		t.Fatalf("pages tagged q3 plans: %+v", with)
	}

	// Search: by a tag, and by a property's value.
	type hit struct{ Kind, ID string }
	find := func(q string) []string {
		var hs []hit
		decode(t, authdo(r, "GET", "/api/v1/search?q="+q, "", alice), &hs)
		ids := []string{}
		for _, h := range hs {
			if h.Kind == "page" {
				ids = append(ids, h.ID)
			}
		}
		return ids
	}
	if ids := find("Miramar"); len(ids) != 1 || ids[0] != a.ID {
		t.Fatalf("by a property: %v", ids)
	}
	if ids := find("travel"); len(ids) != 2 {
		t.Fatalf("by a tag: %v", ids)
	}

	// A copy keeps them.
	var cp struct{ Tags []string }
	decode(t, authdo(r, "POST", "/api/v1/pages/"+a.ID+"/duplicate", "", alice), &cp)
	if len(cp.Tags) != 2 {
		t.Fatalf("the copy's tags: %+v", cp)
	}

	// The export: a YAML header.
	w := authdo(r, "GET", base+"/export", "", alice)
	expect(t, w, http.StatusOK, "export")
	z, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name != "Trip.md" {
			continue
		}
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		want := "---\ntags: [\"travel\",\"Q3 plans\"]\nDue: \"2026-11-04\"\nCost: 1200\nBooked: true\nSite: \"https://example.org\"\nHotel: \"Miramar\"\n---\n# Trip\n"
		if !strings.HasPrefix(string(body), want) {
			t.Fatalf("Trip.md:\n%s", body)
		}
		return
	}
	t.Fatal("no Trip.md in the export")
}

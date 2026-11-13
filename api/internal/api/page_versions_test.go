package api

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// TestPageHistory: saves are kept as versions (one person's saves close
// together make one; another person's, or later ones, a new one; what the
// page was before history began is kept first); a version is read and put
// back (as a new version); viewers read but don't restore; deleting the page
// for good deletes its history.
func TestPageHistory(t *testing.T) {
	r, db, admin, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Notes","kind":"work","starter":false,"modules":["home","pages"]}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	var pg struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Plan","content":"one"}`, alice), &pg)
	type version struct {
		ID       string `json:"id"`
		Version  int    `json:"version"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		UserID   string `json:"user_id"`
		Restored int    `json:"restored"`
		Chars    int    `json:"chars"`
	}
	list := func() []version {
		var vs []version
		decode(t, authdo(r, "GET", "/api/v1/pages/"+pg.ID+"/versions", "", alice), &vs)
		return vs
	}
	save := func(body string, v int, who string) int {
		var out struct{ Version int }
		w := authdo(r, "PUT", "/api/v1/pages/"+pg.ID, `{"content":"`+body+`","version":`+strconv.Itoa(v)+`}`, who)
		expect(t, w, http.StatusOK, "save "+body)
		decode(t, w, &out)
		return out.Version
	}
	// Every save is a version (the one it was made with first).
	v := save("one two", pg.Version, alice)
	db.Model(&models.PageVersion{}).Where("page_id = ? AND content = ?", pg.ID, "one").Update("created_at", time.Now().Add(-4*time.Hour))
	db.Model(&models.PageVersion{}).Where("page_id = ? AND content = ?", pg.ID, "one two").Update("created_at", time.Now().Add(-3*time.Hour))
	v = save("one two three", v, alice)
	db.Model(&models.PageVersion{}).Where("page_id = ? AND content = ?", pg.ID, "one two three").Update("created_at", time.Now().Add(-2*time.Hour))
	v = save("one two three four", v, bob)
	db.Model(&models.PageVersion{}).Where("page_id = ? AND content = ?", pg.ID, "one two three four").Update("created_at", time.Now().Add(-time.Hour))
	v = save("five", v, bob)
	vs := list()
	if len(vs) != 5 || vs[0].Chars != 4 || vs[4].Chars != 3 {
		t.Fatalf("versions: %+v", vs)
	}
	var old version
	decode(t, authdo(r, "GET", "/api/v1/page-versions/"+vs[2].ID, "", alice), &old)
	if old.Content != "one two three" {
		t.Fatalf("old: %+v", old)
	}
	// Put back: the page is that text again, a new version says so.
	var back struct {
		Content string `json:"content"`
		Version int    `json:"version"`
	}
	decode(t, authdo(r, "POST", "/api/v1/page-versions/"+old.ID+"/restore", "", alice), &back)
	if back.Content != "one two three" || back.Version != v+1 {
		t.Fatalf("restored: %+v", back)
	}
	if vs = list(); len(vs) != 6 || vs[0].Restored != old.Version {
		t.Fatalf("after restore: %+v", vs)
	}

	// Only the newest are kept: as many as the owner says.
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"page_versions_kept":2}`, admin), http.StatusBadRequest, "at least 5")
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"page_versions_kept":5}`, admin), http.StatusOK, "keep 5")
	save("six", back.Version, alice)
	if vs = list(); len(vs) != 5 || vs[0].Chars != 3 {
		t.Fatalf("kept: %d %+v", len(vs), vs)
	}

	// A page from before history: its first save keeps what it was.
	var legacy struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Old","content":"before"}`, alice), &legacy)
	db.Where("page_id = ?", legacy.ID).Delete(&models.PageVersion{})
	var lp struct{ Version int }
	decode(t, authdo(r, "GET", "/api/v1/pages/"+legacy.ID, "", alice), &lp)
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+legacy.ID, `{"content":"after","version":`+strconv.Itoa(lp.Version)+`}`, bob), http.StatusOK, "legacy save")
	var lvs []version
	decode(t, authdo(r, "GET", "/api/v1/pages/"+legacy.ID+"/versions", "", alice), &lvs)
	if len(lvs) != 2 || lvs[1].Chars != len("before") || lvs[0].Chars != len("after") {
		t.Fatalf("legacy: %+v", lvs)
	}

	// Strangers see nothing; deleting for good takes the history too.
	expect(t, authdo(r, "GET", "/api/v1/pages/"+pg.ID+"/versions", "", admin), http.StatusNotFound, "stranger")
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+pg.ID, "", alice), http.StatusNoContent, "trash")
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+pg.ID+"?permanent=1", "", alice), http.StatusNoContent, "destroy")
	var n int64
	db.Model(&models.PageVersion{}).Where("page_id = ?", pg.ID).Count(&n)
	if n != 0 {
		t.Fatalf("history left: %d", n)
	}
}

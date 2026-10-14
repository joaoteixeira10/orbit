package api

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"sort"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// TestPagesBecomeMarkdown: a page still in the old JSON format is converted
// when the server starts, without touching its "edited" time; the export is
// a zip of Markdown files in the page tree.
func TestPagesBecomeMarkdown(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	tok := loginTok(t, r, "admin", "adminpass")
	ws := createWS(t, r, tok, `{"name":"Notes","starter":false}`)
	var parent, child pageResponse
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Plans","content":"Some *plans*"}`, tok), &parent)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Trip: Lisbon","parent_id":"`+parent.ID+`"}`, tok), &child)
	authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Plans","content":"again"}`, tok)

	// An old page, as the previous version stored it.
	old := `{"type":"doc","content":[{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"Pack"}]}]}]}]}`
	db.Model(&models.Page{}).Where("id = ?", child.ID).UpdateColumn("content", old)
	var before models.Page
	db.First(&before, "id = ?", child.ID)

	r = routerOn(t, db, &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000}, nil)
	var after models.Page
	db.First(&after, "id = ?", child.ID)
	if after.Content != "- [x] Pack" || after.ContentText != "Pack" || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("migrated: %q / %q / %v→%v", after.Content, after.ContentText, before.UpdatedAt, after.UpdatedAt)
	}

	w := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/export", "", tok)
	expect(t, w, http.StatusOK, "export")
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	var names []string
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"Plans (2).md", "Plans.md", "Plans/Trip- Lisbon.md"}
	if len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("files: %v", names)
	}
	if files["Plans.md"] != "# Plans\n\nSome *plans*\n" || files["Plans/Trip- Lisbon.md"] != "# Trip: Lisbon\n\n- [x] Pack\n" {
		t.Errorf("contents: %q", files)
	}
}

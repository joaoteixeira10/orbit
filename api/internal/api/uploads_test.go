package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
)

// A 1×1 PNG.
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

func upload(r *gin.Engine, token, target, id, name string, data []byte) *httptest.ResponseRecorder {
	return uploadTo(r, token, "/api/v1/uploads?target="+target+"&target_id="+id, name, data)
}

// uploadTo posts a file (form field "file") to url.
func uploadTo(r http.Handler, token, url, name string, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type uploaded struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Type  string `json:"type"`
	Image bool   `json:"image"`
	Name  string `json:"name"`
}

// TestUploads: a member adds a file to a task; members see it, others don't;
// images show inline, anything else (SVG too) only downloads; deleting the
// task deletes its files.
func TestUploads(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Team"}`)
	var task struct {
		ID string `json:"id"`
	}
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Work"}`, alice), http.StatusCreated, "list")
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Report"}`, alice), &task)

	w := upload(r, alice, "task", task.ID, "chart.png", tinyPNG)
	expect(t, w, http.StatusCreated, "upload png")
	var png uploaded
	decode(t, w, &png)
	if !png.Image || png.Type != "image/png" || !strings.HasPrefix(png.URL, "/api/v1/uploads/"+png.ID+"/") {
		t.Fatalf("png: %+v", png)
	}
	get := authdo(r, "GET", png.URL, "", alice)
	expect(t, get, http.StatusOK, "get png")
	if get.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(get.Header().Get("Content-Disposition"), "inline") ||
		get.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(get.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("png headers: %v", get.Header())
	}
	if !bytes.Equal(get.Body.Bytes(), tinyPNG) {
		t.Fatal("png bytes differ")
	}
	if w := authdo(r, "GET", png.URL, "", bob); w.Code != http.StatusNotFound {
		t.Fatalf("a non-member read the file: %d", w.Code)
	}
	if w := upload(r, bob, "task", task.ID, "x.png", tinyPNG); w.Code != http.StatusNotFound {
		t.Fatalf("a non-member uploaded: %d", w.Code)
	}

	// SVG (it can carry script) and HTML: downloaded, never shown.
	svg := upload(r, alice, "task", task.ID, `../evil".svg`, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	expect(t, svg, http.StatusCreated, "upload svg")
	var s uploaded
	decode(t, svg, &s)
	if s.Image || strings.ContainsAny(s.Name, `/"`) {
		t.Fatalf("svg: %+v", s)
	}
	g := authdo(r, "GET", s.URL, "", alice)
	if g.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(g.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("svg served as %q / %q", g.Header().Get("Content-Type"), g.Header().Get("Content-Disposition"))
	}

	var list []uploaded
	decode(t, authdo(r, "GET", "/api/v1/uploads?target=task&target_id="+task.ID, "", alice), &list)
	if len(list) != 2 {
		t.Fatalf("list: %+v", list)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+task.ID, "", alice), http.StatusNoContent, "delete task")
	if w := authdo(r, "GET", png.URL, "", alice); w.Code != http.StatusNotFound {
		t.Fatalf("the task's file outlived it: %d", w.Code)
	}
}

// TestUploadLimits: a file over UPLOAD_MAX is refused, so is one past the
// instance's quota; a personal note's file is its owner's alone.
func TestUploadLimits(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	var note struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/me/notes", `{"body":"mine"}`, alice), &note)
	w := upload(r, alice, "note", note.ID, "a.png", tinyPNG)
	expect(t, w, http.StatusCreated, "personal upload")
	var u uploaded
	decode(t, w, &u)
	if w := authdo(r, "GET", u.URL, "", bob); w.Code != http.StatusNotFound {
		t.Fatalf("someone else read a Quick Note's file: %d", w.Code)
	}
	if w := upload(r, alice, "note", note.ID, "big.bin", bytes.Repeat([]byte("x"), 26<<20)); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a 26 MB file: %d", w.Code)
	}
}

// TestExportCarriesFiles: the page export has the files a page links to,
// and the links point at them.
func TestExportCarriesFiles(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Docs"}`)
	var page struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Guide"}`, alice), &page)
	var u uploaded
	decode(t, upload(r, alice, "page", page.ID, "screen shot.png", tinyPNG), &u)
	body, _ := json.Marshal(map[string]any{"content": "See ![shot](" + u.URL + ")", "version": 1})
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+page.ID, string(body), alice), http.StatusOK, "save page")

	w := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/export", "", alice)
	expect(t, w, http.StatusOK, "export")
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	md := files["Guide.md"]
	if strings.Contains(md, "/api/v1/uploads/") || !strings.Contains(md, "](attachments/") {
		t.Fatalf("page link not rewritten: %q", md)
	}
	found := false
	for name, data := range files {
		if strings.HasPrefix(name, "attachments/") && strings.HasSuffix(name, "-screen shot.png") && data == string(tinyPNG) {
			found = true
		}
	}
	if !found {
		t.Fatalf("attachment missing: %v", func() []string {
			var n []string
			for k := range files {
				n = append(n, k)
			}
			return n
		}())
	}
}

// TestUploadsShareBytes: the same file added twice is kept once (and counts
// once against the quota); its bytes stay while any file uses them — in the
// database and in a directory store alike.
func TestUploadsShareBytes(t *testing.T) {
	for _, kind := range []string{uploads.StoreDB, uploads.StoreDir} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			if kind == uploads.StoreDir {
				uploads.Use(uploads.DirStore{Root: dir})
				t.Cleanup(func() { uploads.Use(uploads.DBStore{}) })
			}
			r, db, _, alice, _ := plannerSetupDB(t)
			ws := createWS(t, r, alice, `{"name":"Team"}`)
			expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Work"}`, alice), http.StatusCreated, "list")
			var a, b struct {
				ID string `json:"id"`
			}
			decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"One"}`, alice), &a)
			decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Two"}`, alice), &b)
			var ua, ub uploaded
			decode(t, upload(r, alice, "task", a.ID, "chart.png", tinyPNG), &ua)
			decode(t, upload(r, alice, "task", b.ID, "copy.png", tinyPNG), &ub)

			kept := func() int {
				n := 0
				_ = uploads.Current().Each(db, func(string) error { n++; return nil })
				return n
			}
			if kept() != 1 {
				t.Fatalf("two copies of one file kept %d times", kept())
			}
			if got := uploads.Used(db, ""); got != int64(len(tinyPNG)) {
				t.Fatalf("used %d, want %d", got, len(tinyPNG))
			}
			expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+a.ID, "", alice), http.StatusNoContent, "delete one")
			if w := authdo(r, "GET", ub.URL, "", alice); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tinyPNG) {
				t.Fatalf("the other file lost its bytes: %d", w.Code)
			}
			expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+b.ID, "", alice), http.StatusNoContent, "delete two")
			if kept() != 1 {
				t.Fatalf("files in the trash lost their bytes: %d", kept())
			}
			// The trash emptied: bytes no file uses go.
			if err := uploads.EmptyTrash(db, time.Now().Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if kept() != 0 {
				t.Fatalf("bytes no file uses are kept: %d", kept())
			}
		})
	}
}

// TestUploadsMigrateAndMove: bytes kept the old way (a row per file) move
// into the store at start; a store other than the recorded one is refused;
// moving to a directory and back keeps every file.
func TestUploadsMigrateAndMove(t *testing.T) {
	r, db, _, alice, _ := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Team"}`)
	var task struct {
		ID string `json:"id"`
	}
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Work"}`, alice), http.StatusCreated, "list")
	wt := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Old"}`, alice)
	expect(t, wt, http.StatusCreated, "task")
	decode(t, wt, &task)
	var u uploaded
	w0 := upload(r, alice, "task", task.ID, "chart.png", tinyPNG)
	expect(t, w0, http.StatusCreated, "upload")
	decode(t, w0, &u)
	// As an older Orbit kept it: a row of its own, no sum.
	db.Exec("DELETE FROM blobs")
	db.Model(&models.Upload{}).Where("id = ?", u.ID).Update("sha256", "")
	db.Create(&models.UploadBlob{ID: u.ID, Data: tinyPNG})
	if err := uploads.Migrate(db, nil); err != nil {
		t.Fatal(err)
	}
	var left int64
	db.Model(&models.UploadBlob{}).Count(&left)
	if w := authdo(r, "GET", u.URL, "", alice); left != 0 || w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tinyPNG) {
		t.Fatalf("after the move: %d old rows, read %d %q", left, w.Code, w.Body.String())
	}

	// Started with another store while the files are in the database: refused.
	if err := uploads.Record(db, uploads.StoreDB); err != nil {
		t.Fatal(err)
	}
	dir := uploads.DirStore{Root: t.TempDir()}
	uploads.Use(dir)
	t.Cleanup(func() { uploads.Use(uploads.DBStore{}) })
	if err := uploads.Check(db); err == nil || !strings.Contains(err.Error(), "move-files") {
		t.Fatalf("a mismatched store: %v", err)
	}
	// Moved (the old copies cleaned): read from the directory.
	if err := uploads.Move(db, uploads.DBStore{}, dir, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := uploads.Check(db); err != nil {
		t.Fatal(err)
	}
	var inDB int64
	db.Model(&models.Blob{}).Count(&inDB)
	if w := authdo(r, "GET", u.URL, "", alice); inDB != 0 || w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tinyPNG) {
		t.Fatalf("in a directory: %d left in the database, read %d", inDB, w.Code)
	}
	// And back.
	if err := uploads.Move(db, dir, uploads.DBStore{}, false, nil); err != nil {
		t.Fatal(err)
	}
	uploads.Use(uploads.DBStore{})
	if w := authdo(r, "GET", u.URL, "", alice); w.Code != http.StatusOK || uploads.Recorded(db) != uploads.StoreDB {
		t.Fatalf("back in the database: read %d, recorded %q", w.Code, uploads.Recorded(db))
	}
}

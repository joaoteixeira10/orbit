package api

import (
	"archive/zip"
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"
)

type fileRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Target   string `json:"target"`
	FolderID string `json:"folder_id"`
	OnKind   string `json:"on_kind"`
	OnTitle  string `json:"on_title"`
	Versions int    `json:"versions"`
	WasKind  string `json:"was_kind"`
	WasTitle string `json:"was_title"`
}

// fileUpload adds a file of its own to a folder of Files.
func fileUpload(t *testing.T, r http.Handler, token, ws, folder, name string, data []byte) fileRow {
	t.Helper()
	w := uploadTo(r, token, "/api/v1/uploads?target=file&target_id="+ws+"&folder_id="+folder, name, data)
	expect(t, w, http.StatusCreated, "upload "+name)
	var f fileRow
	decode(t, w, &f)
	return f
}

// TestFilesInFolders: a task's attachment shows in its list's folder; a
// file of its own goes in the folder it's added to, and moves; the same
// name again is a new version (links show the newest; an older one comes
// back); deleting puts it in the trash (and back); a task deleted for good
// sends its files to the trash as files of their own, where they were.
func TestFilesInFolders(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Studio","kind":"custom","modules":["home","tasks","files"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	var list struct{ ID string }
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Launch"}`, alice), &list)
	var task struct{ ID string }
	decode(t, authdo(r, "POST", base+"/tasks", `{"title":"Logo","list_id":"`+list.ID+`"}`, alice), &task)
	var folders []struct{ ID, Name string }
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &folders)
	launch := folders[0].ID
	var docs folderResp
	decode(t, authdo(r, "POST", base+"/folders", `{"name":"Docs"}`, alice), &docs)

	files := func(q string) map[string]fileRow {
		var rows []fileRow
		decode(t, authdo(r, "GET", base+"/files"+q, "", alice), &rows)
		out := map[string]fileRow{}
		for _, f := range rows {
			out[f.Name] = f
		}
		return out
	}

	// An attachment, where its task's list is.
	expect(t, upload(r, alice, "task", task.ID, "logo.png", tinyPNG), http.StatusCreated, "attach")
	if f := files("")["logo.png"]; f.FolderID != launch || f.OnKind != "task" || f.OnTitle != "Logo" {
		t.Fatalf("an attachment: %+v", f)
	}
	// A file of its own, in Docs; counted there.
	brief := fileUpload(t, r, alice, ws.ID, docs.ID, "brief.txt", []byte("v1"))
	expect(t, uploadTo(r, bob, "/api/v1/uploads?target=file&target_id="+ws.ID, "x.txt", []byte("x")), http.StatusNotFound, "not a member")
	expect(t, uploadTo(r, alice, "/api/v1/uploads?target=file&target_id="+ws.ID+"&folder_id=nope", "x.txt", []byte("x")), http.StatusBadRequest, "no such folder")
	var counted []struct {
		Name   string
		Counts struct{ Files int }
	}
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &counted)
	for _, f := range counted {
		if want := map[string]int{"Launch": 1, "Docs": 1}[f.Name]; f.Counts.Files != want {
			t.Fatalf("%s holds %d files", f.Name, f.Counts.Files)
		}
	}

	// The same name again: a version; its link shows the newest.
	v2 := fileUpload(t, r, alice, ws.ID, docs.ID, "brief.txt", []byte("v2"))
	if f := files("")["brief.txt"]; f.ID != v2.ID || f.Versions != 2 {
		t.Fatalf("versions: %+v", f)
	}
	if w := authdo(r, "GET", brief.URL, "", alice); w.Body.String() != "v2" {
		t.Fatalf("the old link shows %q", w.Body.String())
	}
	if w := authdo(r, "GET", brief.URL+"?exact=1", "", alice); w.Body.String() != "v1" {
		t.Fatalf("that version: %q", w.Body.String())
	}
	expect(t, authdo(r, "POST", "/api/v1/files/"+brief.ID+"/current", "", alice), http.StatusOK, "v1 back")
	if w := authdo(r, "GET", brief.URL, "", alice); w.Body.String() != "v1" || files("")["brief.txt"].Versions != 3 {
		t.Fatalf("after bringing v1 back: %q", w.Body.String())
	}
	var vs []struct {
		ID      string
		Current bool
	}
	decode(t, authdo(r, "GET", "/api/v1/files/"+brief.ID+"/versions", "", alice), &vs)
	if len(vs) != 3 || !vs[0].Current || vs[1].Current {
		t.Fatalf("versions list: %+v", vs)
	}

	// Renamed (every version); moved; an attachment can't be moved apart.
	expect(t, authdo(r, "PUT", "/api/v1/files/"+v2.ID, `{"name":"Brief.txt"}`, alice), http.StatusOK, "rename")
	expect(t, authdo(r, "PUT", "/api/v1/files/"+v2.ID, `{"folder_id":""}`, alice), http.StatusOK, "to the top")
	if f := files("")["Brief.txt"]; f.FolderID != "" {
		t.Fatalf("moved: %+v", f)
	}
	expect(t, authdo(r, "POST", base+"/files/move", `{"folder_id":"`+docs.ID+`","files":["`+files("")["Brief.txt"].ID+`"]}`, alice), http.StatusNoContent, "move back")
	if f := files("")["Brief.txt"]; f.FolderID != docs.ID {
		t.Fatalf("moved back: %+v", f)
	}
	expect(t, authdo(r, "PUT", "/api/v1/files/"+files("")["logo.png"].ID, `{"folder_id":"`+docs.ID+`"}`, alice), http.StatusBadRequest, "an attachment moved apart")

	// To the trash and back.
	cur := files("")["Brief.txt"]
	expect(t, authdo(r, "DELETE", "/api/v1/uploads/"+cur.ID, "", alice), http.StatusNoContent, "to the trash")
	if _, there := files("")["Brief.txt"]; there || files("?trash=1")["Brief.txt"].ID == "" {
		t.Fatal("not in the trash")
	}
	if w := authdo(r, "GET", cur.URL, "", alice); w.Code != http.StatusNotFound {
		t.Fatalf("a file in the trash is served: %d", w.Code)
	}
	expect(t, authdo(r, "POST", "/api/v1/files/"+cur.ID+"/restore", "", alice), http.StatusOK, "restore")
	if f := files("")["Brief.txt"]; f.FolderID != docs.ID || f.Versions != 3 {
		t.Fatalf("restored: %+v", f)
	}

	// The task deleted for good: its file to the trash, as one of its own.
	expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+task.ID, "", alice), http.StatusNoContent, "delete task")
	trashed := files("?trash=1")["logo.png"]
	if trashed.Target != uploads.File || trashed.WasKind != "task" || trashed.WasTitle != "Logo" || trashed.FolderID != launch {
		t.Fatalf("a deleted task's file: %+v", trashed)
	}
	expect(t, authdo(r, "POST", "/api/v1/files/"+trashed.ID+"/restore", "", alice), http.StatusOK, "restore it")
	if f := files("")["logo.png"]; f.Target != uploads.File || f.FolderID != launch {
		t.Fatalf("back as a file of its own: %+v", f)
	}

	// Emptied after its days.
	expect(t, authdo(r, "DELETE", "/api/v1/uploads/"+files("")["logo.png"].ID, "", alice), http.StatusNoContent, "trash again")
	if err := uploads.EmptyTrash(db, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(files("?trash=1")) != 0 {
		t.Fatal("the trash wasn't emptied")
	}
	// Brief.txt's two sets of bytes ("v1", "v2") take 4.
	var usage struct{ Used, Files int64 }
	decode(t, authdo(r, "GET", base+"/files/usage", "", alice), &usage)
	if usage.Files != 1 || usage.Used != 4 {
		t.Fatalf("usage: %+v", usage)
	}
}

// TestMyFiles: someone's own folders hold their sketches and files, and
// their Quick Notes' attachments show at the top; nobody else sees them.
func TestMyFiles(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	var docs folderResp
	w := authdo(r, "POST", "/api/v1/me/folders", `{"name":"Receipts"}`, alice)
	expect(t, w, http.StatusCreated, "my folder")
	decode(t, w, &docs)
	expect(t, authdo(r, "POST", "/api/v1/me/folders", `{"name":"X","list_id":"l"}`, alice), http.StatusBadRequest, "a list on a personal folder")

	f := fileUpload(t, r, alice, "me", docs.ID, "taxi.pdf", []byte("%PDF-1.4"))
	expect(t, uploadTo(r, bob, "/api/v1/uploads?target=file&target_id=me&folder_id="+docs.ID, "x.txt", []byte("x")), http.StatusBadRequest, "into someone else's folder")
	var sketch struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/me/drawings", `{"title":"Napkin","folder_id":"`+docs.ID+`"}`, alice), &sketch)
	var note struct{ ID string }
	decode(t, authdo(r, "POST", "/api/v1/me/notes", `{"title":"Ideas","body":"x"}`, alice), &note)
	expect(t, upload(r, alice, "note", note.ID, "sketch.png", tinyPNG), http.StatusCreated, "a note's picture")

	var folders []struct {
		ID     string
		Counts struct{ Files, Boards int }
	}
	decode(t, authdo(r, "GET", "/api/v1/me/folders", "", alice), &folders)
	if len(folders) != 1 || folders[0].Counts.Files != 1 || folders[0].Counts.Boards != 1 {
		t.Fatalf("my folders: %+v", folders)
	}
	var mine []fileRow
	decode(t, authdo(r, "GET", "/api/v1/me/files", "", alice), &mine)
	byName := map[string]fileRow{}
	for _, x := range mine {
		byName[x.Name] = x
	}
	if byName["taxi.pdf"].FolderID != docs.ID || byName["sketch.png"].OnKind != "note" || byName["sketch.png"].FolderID != "" {
		t.Fatalf("my files: %+v", mine)
	}

	// Nobody else's.
	var theirs []fileRow
	decode(t, authdo(r, "GET", "/api/v1/me/files", "", bob), &theirs)
	if len(theirs) != 0 {
		t.Fatalf("bob sees %+v", theirs)
	}
	expect(t, authdo(r, "GET", f.URL, "", bob), http.StatusNotFound, "someone else's file")
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+docs.ID, `{"name":"Mine"}`, bob), http.StatusNotFound, "someone else's folder")
	expect(t, authdo(r, "POST", "/api/v1/me/files/move", `{"folder_id":"`+docs.ID+`","files":["`+f.ID+`"]}`, bob), http.StatusBadRequest, "into someone else's folder")

	// Moved to the top; the folder deleted keeps the sketch (it moves up).
	expect(t, authdo(r, "POST", "/api/v1/me/files/move", `{"folder_id":"","files":["`+f.ID+`"]}`, alice), http.StatusNoContent, "move")
	expect(t, authdo(r, "DELETE", "/api/v1/folders/"+docs.ID, "", alice), http.StatusNoContent, "delete folder")
	var sketches []struct {
		ID       string
		FolderID string `json:"folder_id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/drawings", "", alice), &sketches)
	if len(sketches) != 1 || sketches[0].FolderID != "" {
		t.Fatalf("the sketch after its folder went: %+v", sketches)
	}
	var usage struct{ Used, Files int64 }
	decode(t, authdo(r, "GET", "/api/v1/me/files/usage", "", alice), &usage)
	if usage.Files != 2 || usage.Used != int64(len(tinyPNG)+8) {
		t.Fatalf("usage: %+v", usage)
	}
}

// docx is a tiny Word file saying text.
func docx(t *testing.T, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("word/document.xml")
	_, _ = w.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	_ = z.Close()
	return b.Bytes()
}

// TestFileSearch: files are found by name and by what they say (a Word
// file read here, a PDF's text sent by the app); not once in the trash,
// nor someone else's own.
func TestFileSearch(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Fatal("no search index")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Studio","kind":"custom","modules":["home","files"]}`)
	type hit struct {
		Kind, ID, Title, Snippet string
		FolderID                 string `json:"folder_id"`
	}
	find := func(token, q string) []hit {
		var hs []hit
		decode(t, authdo(r, "GET", "/api/v1/search?q="+q, "", token), &hs)
		out := []hit{}
		for _, x := range hs {
			if x.Kind == "file" {
				out = append(out, x)
			}
		}
		return out
	}
	w := fileUpload(t, r, alice, ws.ID, "", "contract.docx", docx(t, "The lessee pays quarterly"))
	if hs := find(alice, "contract"); len(hs) != 1 || hs[0].ID != w.ID {
		t.Fatalf("by name: %+v", hs)
	}
	if hs := find(alice, "quarterly"); len(hs) != 1 || !strings.Contains(hs[0].Snippet, "quarterly") {
		t.Fatalf("by what it says: %+v", hs)
	}
	pdf := fileUpload(t, r, alice, ws.ID, "", "scan.pdf", []byte("%PDF-1.4\n%%EOF"))
	expect(t, authdo(r, "PUT", "/api/v1/files/"+pdf.ID+"/text", `{"text":"invoice number 4471"}`, alice), http.StatusNoContent, "a PDF's text")
	expect(t, authdo(r, "PUT", "/api/v1/files/"+w.ID+"/text", `{"text":"x"}`, alice), http.StatusBadRequest, "only a PDF's")
	if hs := find(alice, "4471"); len(hs) != 1 || hs[0].ID != pdf.ID {
		t.Fatalf("a PDF by its text: %+v", hs)
	}
	if hs := find(bob, "contract"); len(hs) != 0 {
		t.Fatalf("bob found %+v", hs)
	}
	mine := fileUpload(t, r, bob, "me", "", "private-contract.txt", []byte("mine"))
	if hs := find(alice, "private"); len(hs) != 0 {
		t.Fatalf("someone else's own file: %+v", hs)
	}
	if hs := find(bob, "private"); len(hs) != 1 || hs[0].ID != mine.ID {
		t.Fatalf("my own file: %+v", hs)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/uploads/"+w.ID, "", alice), http.StatusNoContent, "trash")
	if hs := find(alice, "quarterly"); len(hs) != 0 {
		t.Fatalf("a file in the trash: %+v", hs)
	}
	var one struct {
		Name     string
		Versions int
	}
	decode(t, authdo(r, "GET", "/api/v1/files/"+pdf.ID, "", alice), &one)
	if one.Name != "scan.pdf" || one.Versions != 1 {
		t.Fatalf("one file: %+v", one)
	}
	expect(t, authdo(r, "GET", "/api/v1/files/"+pdf.ID, "", bob), http.StatusNotFound, "someone else's")
}

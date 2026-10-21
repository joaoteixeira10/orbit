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

	"github.com/gin-gonic/gin"
)

func importWS(r *gin.Engine, token string, data []byte, people bool) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "ws.orbit.zip")
	_, _ = fw.Write(data)
	if !people {
		_ = mw.WriteField("people", "false")
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", "/api/v1/workspaces/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

type importResp struct {
	Workspace wsResp `json:"workspace"`
	Report    struct {
		Lists, Tasks, Pages, Notes, Events, Comments, Files int
		Added, Missing, Skipped                             []string
	} `json:"report"`
}

// TestWorkspaceExportImport: a workspace goes out as a zip and comes back as
// a new one — new ids, links between its pages and to its files following,
// people matched by username, the importer its owner; someone with no
// account here keeps their name on their comments.
func TestWorkspaceExportImport(t *testing.T) {
	r, admin, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Year 11 — Maths","kind":"school"}`)
	base := "/api/v1/workspaces/" + ws.ID
	expect(t, authdo(r, "POST", base+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	bobID := meID(t, r, bob)

	var parent, child struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Parent"}`, alice), &parent)
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Child","parent_id":"`+parent.ID+`"}`, alice), &child)
	var up uploaded
	w := upload(r, alice, "page", child.ID, "dot.png", tinyPNG)
	expect(t, w, http.StatusCreated, "upload")
	decode(t, w, &up)
	doc, _ := json.Marshal(map[string]any{"content": "See [the parent](/w/" + ws.ID + "/p/" + parent.ID + ")\n\n![dot](" + up.URL + ")", "version": 1})
	expect(t, authdo(r, "PUT", "/api/v1/pages/"+child.ID, string(doc), alice), http.StatusOK, "child content")

	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", base+"/tasks", `{"title":"Revise chapter 3","notes":"page /w/`+ws.ID+`/p/`+parent.ID+`","assignee_ids":["`+bobID+`"]}`, alice), &task)
	expect(t, authdo(r, "POST", base+"/tasks", `{"title":"Exercises","parent_id":"`+task.ID+`"}`, alice), http.StatusCreated, "subtask")
	expect(t, authdo(r, "POST", "/api/v1/tasks/"+task.ID+"/comments", `{"body":"On it"}`, bob), http.StatusCreated, "comment")
	expect(t, authdo(r, "POST", base+"/events", `{"title":"Test","starts_at":"2026-11-05T09:00:00Z","ends_at":"2026-11-05T10:00:00Z","tz":"Europe/Lisbon"}`, alice), http.StatusCreated, "event")

	// Export: owners only.
	if w := authdo(r, "GET", base+"/transfer", "", bob); w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("an editor exports: %d", w.Code)
	}
	w = authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	if !strings.Contains(w.Header().Get("Content-Disposition"), `filename="Year-11-Maths-`) {
		t.Errorf("file name: %s", w.Header().Get("Content-Disposition"))
	}
	archive := w.Body.Bytes()

	// The admin imports it, with the people.
	w = importWS(r, admin, archive, true)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	rep := got.Report
	if got.Workspace.ID == ws.ID || got.Workspace.Name != "Year 11 — Maths" || got.Workspace.Role != "owner" {
		t.Fatalf("imported workspace: %+v", got.Workspace)
	}
	if rep.Tasks != 2 || rep.Pages < 3 || rep.Files != 1 || rep.Comments != 1 || rep.Events != 1 || len(rep.Added) != 2 || len(rep.Missing) != 0 {
		t.Fatalf("report: %+v", rep)
	}
	nb := "/api/v1/workspaces/" + got.Workspace.ID
	var pages []struct {
		ID       string  `json:"id"`
		Title    string  `json:"title"`
		ParentID *string `json:"parent_id"`
	}
	decode(t, authdo(r, "GET", nb+"/pages", "", admin), &pages)
	var newParent, newChild string
	for _, p := range pages {
		switch p.Title {
		case "Parent":
			newParent = p.ID
		case "Child":
			newChild = p.ID
			if p.ParentID == nil {
				t.Error("the child lost its parent")
			}
		}
	}
	var page struct {
		Content string `json:"content"`
	}
	decode(t, authdo(r, "GET", "/api/v1/pages/"+newChild, "", admin), &page)
	if !strings.Contains(page.Content, "/w/"+got.Workspace.ID+"/p/"+newParent) || strings.Contains(page.Content, ws.ID) || strings.Contains(page.Content, up.ID) {
		t.Errorf("links not rewritten: %s", page.Content)
	}
	// The image link opens the imported file.
	i := strings.Index(page.Content, "/api/v1/uploads/")
	link := page.Content[i : strings.Index(page.Content[i:], ")")+i]
	if w := authdo(r, "GET", link, "", admin); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tinyPNG) {
		t.Errorf("the imported image: %d", w.Code)
	}
	// Bob is a member again, still on the task, his comment his.
	var tasks []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		AssigneeIDs []string `json:"assignee_ids"`
		ParentID    *string  `json:"parent_id"`
	}
	decode(t, authdo(r, "GET", nb+"/tasks", "", bob), &tasks)
	var revise string
	for _, x := range tasks {
		if x.Title == "Revise chapter 3" {
			revise = x.ID
			if len(x.AssigneeIDs) != 1 || x.AssigneeIDs[0] != bobID {
				t.Errorf("assignees: %v", x.AssigneeIDs)
			}
		}
	}
	var comments []struct {
		AuthorID   string `json:"author_id"`
		AuthorName string `json:"author_name"`
	}
	decode(t, authdo(r, "GET", "/api/v1/tasks/"+revise+"/comments", "", bob), &comments)
	if len(comments) != 1 || comments[0].AuthorID != bobID {
		t.Errorf("comments: %+v", comments)
	}

	// Without the people: only the importer is in; Bob's task loses him.
	w = importWS(r, alice, archive, false)
	expect(t, w, http.StatusCreated, "import alone")
	decode(t, w, &got)
	if len(got.Report.Added) != 0 {
		t.Errorf("added: %v", got.Report.Added)
	}
	if w := authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID, "", bob); w.Code == http.StatusOK {
		t.Error("bob is in a workspace imported without people")
	}

	// Someone with no account here: their comment keeps their name.
	zr, _ := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == "workspace.json" {
			data = bytes.ReplaceAll(data, []byte(`"username": "bob"`), []byte(`"username": "robert"`))
		}
		fw, _ := zw.Create(f.Name)
		_, _ = fw.Write(data)
	}
	_ = zw.Close()
	w = importWS(r, alice, out.Bytes(), true)
	expect(t, w, http.StatusCreated, "import with a stranger")
	decode(t, w, &got)
	if len(got.Report.Missing) != 1 || got.Report.Missing[0] != "robert" {
		t.Errorf("missing: %v", got.Report.Missing)
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/tasks", "", alice), &tasks)
	for _, x := range tasks {
		if x.Title == "Revise chapter 3" {
			decode(t, authdo(r, "GET", "/api/v1/tasks/"+x.ID+"/comments", "", alice), &comments)
			if len(comments) != 1 || comments[0].AuthorID != "" || comments[0].AuthorName != "robert" {
				t.Errorf("a stranger's comment: %+v", comments)
			}
		}
	}

	// Not an export.
	expect(t, importWS(r, alice, []byte("hello"), true), http.StatusBadRequest, "not a zip")
}

package api

import (
	"net/http"
	"testing"
)

// TestFolderProjects: a folder joined to a task list (one list per folder, a
// list in one folder at most) is a project — the list says its folder and
// how far it is; pages are filed in folders too; folders count what they
// hold; deleting the folder or the list undoes the join; export / import
// keeps it.
func TestFolderProjects(t *testing.T) {
	r, _, admin, alice, _ := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Agency","kind":"work","starter":false,"modules":["home","tasks","pages","sheets","files"]}`)
	other := createWS(t, r, alice, `{"name":"Other","kind":"personal","starter":false,"modules":["home","tasks"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	type list struct {
		ID        string `json:"id"`
		FolderID  string `json:"folder_id"`
		OpenCount int    `json:"open_count"`
		DoneCount int    `json:"done_count"`
	}
	type folder struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		ListID string `json:"list_id"`
		Counts struct {
			Sheets, Diagrams, Boards, Pages, Folders int
		} `json:"counts"`
	}
	var app, away list
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"App launch"}`, alice), &app)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+other.ID+"/lists", `{"name":"Away"}`, alice), &away)
	var task struct{ ID string }
	decode(t, authdo(r, "POST", base+"/tasks", `{"title":"Ship","list_id":"`+app.ID+`"}`, alice), &task)
	authdo(r, "POST", base+"/tasks", `{"title":"Test","list_id":"`+app.ID+`"}`, alice)
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"completed":true}`, alice), http.StatusOK, "done")

	// A folder made for the list; another list's folder can't take a list from elsewhere.
	var proj, second folder
	w := authdo(r, "POST", base+"/folders", `{"name":"App","list_id":"`+app.ID+`"}`, alice)
	expect(t, w, http.StatusCreated, "folder with list")
	decode(t, w, &proj)
	expect(t, authdo(r, "POST", base+"/folders", `{"name":"X","list_id":"`+away.ID+`"}`, alice), http.StatusBadRequest, "another workspace's list")
	decode(t, authdo(r, "POST", base+"/folders", `{"name":"Second"}`, alice), &second)

	var all []list
	decode(t, authdo(r, "GET", base+"/lists", "", alice), &all)
	var got list
	for _, l := range all {
		if l.ID == app.ID {
			got = l
		}
	}
	if got.FolderID != proj.ID || got.OpenCount != 1 || got.DoneCount != 1 {
		t.Fatalf("list: %+v", got)
	}
	// Joined to another folder, the list leaves the first.
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+second.ID, `{"list_id":"`+app.ID+`"}`, alice), http.StatusOK, "move list")
	var fs []folder
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &fs)
	by := map[string]folder{}
	for _, f := range fs {
		by[f.Name] = f
	}
	if by["App"].ListID != "" || by["Second"].ListID != app.ID {
		t.Fatalf("one folder per list: %+v", fs)
	}
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+second.ID, `{"list_id":""}`, alice), http.StatusOK, "unjoin")
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+proj.ID, `{"list_id":"`+app.ID+`"}`, alice), http.StatusOK, "join again")

	// Pages filed in it (made there, or moved), counted with the rest.
	var pg, pg2 struct {
		ID       string `json:"id"`
		FolderID string `json:"folder_id"`
	}
	w = authdo(r, "POST", base+"/pages", `{"title":"Brief","folder_id":"`+proj.ID+`"}`, alice)
	expect(t, w, http.StatusCreated, "page in folder")
	decode(t, w, &pg)
	expect(t, authdo(r, "POST", base+"/pages", `{"title":"X","folder_id":"nope"}`, alice), http.StatusBadRequest, "no such folder")
	decode(t, authdo(r, "POST", base+"/pages", `{"title":"Notes"}`, alice), &pg2)
	expect(t, authdo(r, "POST", base+"/files/move", `{"folder_id":"`+proj.ID+`","pages":["`+pg2.ID+`"]}`, alice), http.StatusNoContent, "move page")
	authdo(r, "POST", base+"/sheets", `{"title":"Budget","folder_id":"`+proj.ID+`","data":{"tabs":[]}}`, alice)
	authdo(r, "POST", base+"/folders", `{"name":"Assets","parent_id":"`+proj.ID+`"}`, alice)
	fs = nil
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &fs)
	for _, f := range fs {
		if f.ID == proj.ID && (f.Counts.Pages != 2 || f.Counts.Sheets != 1 || f.Counts.Folders != 1 || f.ListID != app.ID) {
			t.Fatalf("counts: %+v", f)
		}
	}
	var tree []struct {
		ID       string `json:"id"`
		FolderID string `json:"folder_id"`
	}
	decode(t, authdo(r, "GET", base+"/pages", "", alice), &tree)
	in := 0
	for _, p := range tree {
		if p.FolderID == proj.ID {
			in++
		}
	}
	if in != 2 {
		t.Fatalf("tree: %+v", tree)
	}

	// Export / import keeps the project together.
	w = authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, admin, w.Body.Bytes(), false)
	expect(t, w, http.StatusCreated, "import")
	var imp importResp
	decode(t, w, &imp)
	fs = nil
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+imp.Workspace.ID+"/folders", "", admin), &fs)
	var lists []list
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+imp.Workspace.ID+"/lists", "", admin), &lists)
	joined := false
	for _, f := range fs {
		for _, l := range lists {
			if f.Name == "App" && f.ListID == l.ID && l.FolderID == f.ID && f.Counts.Pages == 2 {
				joined = true
			}
		}
	}
	if !joined {
		t.Fatalf("imported: %+v %+v", fs, lists)
	}

	// Deleting the folder files its pages one up; deleting the list unjoins.
	expect(t, authdo(r, "DELETE", "/api/v1/folders/"+proj.ID, "", alice), http.StatusNoContent, "delete folder")
	tree = nil // a reused slice keeps fields the answer leaves out
	decode(t, authdo(r, "GET", base+"/pages", "", alice), &tree)
	for _, p := range tree {
		if p.FolderID != "" {
			t.Fatalf("pages after delete: %+v", tree)
		}
	}
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+second.ID, `{"list_id":"`+app.ID+`"}`, alice), http.StatusOK, "join second")
	expect(t, authdo(r, "DELETE", "/api/v1/lists/"+app.ID, "", alice), http.StatusNoContent, "delete list")
	fs = nil
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &fs)
	for _, f := range fs {
		if f.ListID != "" {
			t.Fatalf("list gone: %+v", f)
		}
	}
}

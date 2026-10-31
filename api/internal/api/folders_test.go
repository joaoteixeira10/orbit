package api

import (
	"net/http"
	"slices"
	"testing"
)

type folderResp struct {
	ID       string `json:"id"`
	ParentID string `json:"parent_id"`
	Name     string `json:"name"`
}

type fileResp struct {
	ID       string `json:"id"`
	FolderID string `json:"folder_id"`
	Title    string `json:"title"`
	Size     int    `json:"size"`
}

// TestFolders: folders inside folders hold sheets, diagrams and
// whiteboards; things move (not into themselves, not to another
// workspace's folder); deleting a folder moves what it held up; export /
// import carries them.
func TestFolders(t *testing.T) {
	r, _, admin, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Studio","kind":"personal","starter":false,"modules":["home","sheets","diagrams","whiteboards","files"]}`)
	other := createWS(t, r, alice, `{"name":"Other","kind":"personal","starter":false,"modules":["home"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	var design, logos, elsewhere folderResp
	decode(t, authdo(r, "POST", base+"/folders", `{"name":"Design"}`, alice), &design)
	w := authdo(r, "POST", base+"/folders", `{"name":"Logos","parent_id":"`+design.ID+`"}`, alice)
	expect(t, w, http.StatusCreated, "nested")
	decode(t, w, &logos)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+other.ID+"/folders", `{"name":"Away"}`, alice), &elsewhere)
	expect(t, authdo(r, "POST", base+"/folders", `{"name":"  "}`, alice), http.StatusBadRequest, "no name")
	expect(t, authdo(r, "POST", base+"/folders", `{"name":"X","parent_id":"`+elsewhere.ID+`"}`, alice), http.StatusBadRequest, "another workspace's")
	expect(t, authdo(r, "POST", base+"/folders", `{"name":"X"}`, bob), http.StatusNotFound, "not a member")

	// Made in a folder, or moved there.
	var s, d, b fileResp
	decode(t, authdo(r, "POST", base+"/sheets", `{"title":"Budget","folder_id":"`+logos.ID+`","data":{"tabs":[]}}`, alice), &s)
	decode(t, authdo(r, "POST", base+"/diagrams", `{"title":"Flow"}`, alice), &d)
	decode(t, authdo(r, "POST", base+"/drawings", `{"title":"Board"}`, alice), &b)
	expect(t, authdo(r, "POST", base+"/diagrams", `{"title":"X","folder_id":"`+elsewhere.ID+`"}`, alice), http.StatusBadRequest, "folder elsewhere")
	if s.FolderID != logos.ID {
		t.Fatalf("made in: %+v", s)
	}
	expect(t, authdo(r, "POST", base+"/files/move", `{"folder_id":"`+design.ID+`","diagrams":["`+d.ID+`"],"drawings":["`+b.ID+`"]}`, alice), http.StatusNoContent, "move")
	expect(t, authdo(r, "POST", base+"/files/move", `{"folder_id":"`+logos.ID+`","folders":["`+design.ID+`"]}`, alice), http.StatusBadRequest, "into itself")
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+design.ID, `{"parent_id":"`+logos.ID+`"}`, alice), http.StatusBadRequest, "into itself (put)")
	expect(t, authdo(r, "POST", base+"/files/move", `{"folder_id":"`+elsewhere.ID+`","sheets":["`+s.ID+`"]}`, alice), http.StatusBadRequest, "move elsewhere")
	var ds []fileResp
	decode(t, authdo(r, "GET", base+"/diagrams", "", alice), &ds)
	var bs []fileResp
	decode(t, authdo(r, "GET", base+"/drawings", "", alice), &bs)
	var ss []fileResp
	decode(t, authdo(r, "GET", base+"/sheets", "", alice), &ss)
	if ds[0].FolderID != design.ID || bs[0].FolderID != design.ID || ss[0].Size == 0 {
		t.Fatalf("moved: %+v %+v %+v", ds, bs, ss)
	}
	// Moving changes nothing else: the version stays.
	var full struct {
		Version int `json:"version"`
	}
	decode(t, authdo(r, "GET", "/api/v1/diagrams/"+d.ID, "", alice), &full)
	if full.Version != 1 {
		t.Errorf("version after a move: %d", full.Version)
	}

	// Export / import carries the folders and who's in which.
	w = authdo(r, "GET", base+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, admin, w.Body.Bytes(), false)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	var fs []folderResp
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/folders", "", admin), &fs)
	byName := map[string]folderResp{}
	for _, f := range fs {
		byName[f.Name] = f
	}
	if len(fs) != 2 || byName["Logos"].ParentID != byName["Design"].ID || byName["Design"].ID == design.ID {
		t.Fatalf("imported folders: %+v", fs)
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/sheets", "", admin), &ss)
	if len(ss) != 1 || ss[0].FolderID != byName["Logos"].ID {
		t.Errorf("imported sheet: %+v", ss)
	}

	// Deleting Design: Logos and what Design held move up to the top.
	expect(t, authdo(r, "DELETE", "/api/v1/folders/"+design.ID, "", alice), http.StatusNoContent, "delete")
	decode(t, authdo(r, "GET", base+"/folders", "", alice), &fs)
	if len(fs) != 1 || fs[0].Name != "Logos" || fs[0].ParentID != "" {
		t.Errorf("after delete: %+v", fs)
	}
	decode(t, authdo(r, "GET", base+"/diagrams", "", alice), &ds)
	decode(t, authdo(r, "GET", base+"/sheets", "", alice), &ss)
	if ds[0].FolderID != "" || ss[0].FolderID != logos.ID {
		t.Errorf("contents after delete: %+v %+v", ds, ss)
	}
}

// TestListFolders: every list gets its own folder (made with it: by hand,
// from a template); a rename follows while the folder kept the list's name;
// a folder of the same name with no list is joined instead of a second one;
// deleting the list keeps the folder; templates turn Files on.
func TestListFolders(t *testing.T) {
	r, _, _, alice, _ := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Studio","kind":"work"}`) // the template's Projects list
	if !slices.Contains(ws.Modules, "files") {
		t.Fatalf("a template without Files: %v", ws.Modules)
	}
	base := "/api/v1/workspaces/" + ws.ID
	type folder struct {
		ID, Name, Color string
		ListID          string `json:"list_id"`
	}
	folders := func() map[string]folder {
		var fs []folder
		decode(t, authdo(r, "GET", base+"/folders", "", alice), &fs)
		out := map[string]folder{}
		for _, f := range fs {
			out[f.Name] = f
		}
		return out
	}
	var lists []struct {
		ID, Name string
		FolderID string `json:"folder_id"`
	}
	decode(t, authdo(r, "GET", base+"/lists", "", alice), &lists)
	if len(lists) != 1 || lists[0].FolderID == "" || folders()[lists[0].Name].ListID != lists[0].ID {
		t.Fatalf("the template's list has no folder: %+v %+v", lists, folders())
	}

	// By hand: its own folder, coloured as it.
	var l struct{ ID string }
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Launch","color":"#ff9500"}`, alice), &l)
	if f := folders()["Launch"]; f.ListID != l.ID || f.Color != "#ff9500" {
		t.Fatalf("a new list's folder: %+v", f)
	}
	// Renamed: the folder follows; renamed apart, it no longer does.
	expect(t, authdo(r, "PUT", "/api/v1/lists/"+l.ID, `{"name":"Launch 2027","color":"#34c759"}`, alice), http.StatusOK, "rename")
	f := folders()["Launch 2027"]
	if f.ListID != l.ID || f.Color != "#34c759" {
		t.Fatalf("after the rename: %+v", folders())
	}
	expect(t, authdo(r, "PUT", "/api/v1/folders/"+f.ID, `{"name":"Our launch"}`, alice), http.StatusOK, "folder renamed apart")
	expect(t, authdo(r, "PUT", "/api/v1/lists/"+l.ID, `{"name":"Launch 2028"}`, alice), http.StatusOK, "rename again")
	if _, ok := folders()["Our launch"]; !ok {
		t.Fatalf("a folder named apart was renamed: %+v", folders())
	}

	// A folder of that name with no list is joined (no second one).
	var ads folderResp
	decode(t, authdo(r, "POST", base+"/folders", `{"name":"Ads"}`, alice), &ads)
	var adsList struct{ ID string }
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Ads"}`, alice), &adsList)
	if fs := folders(); len(fs) != 3 || fs["Ads"].ID != ads.ID || fs["Ads"].ListID != adsList.ID {
		t.Fatalf("joined the folder there was: %+v", fs)
	}

	// The list deleted: its folder stays, unjoined.
	expect(t, authdo(r, "DELETE", "/api/v1/lists/"+adsList.ID, "", alice), http.StatusNoContent, "delete list")
	if f, ok := folders()["Ads"]; !ok || f.ListID != "" {
		t.Fatalf("after the list went: %+v", folders())
	}

	// Made with no folder, or into one chosen.
	var bare, into struct {
		ID       string
		FolderID string `json:"folder_id"`
	}
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Loose","folder_id":"none"}`, alice), &bare)
	decode(t, authdo(r, "POST", base+"/lists", `{"name":"Ads again","folder_id":"`+ads.ID+`"}`, alice), &into)
	if _, made := folders()["Loose"]; made || bare.FolderID != "" || into.FolderID != ads.ID {
		t.Fatalf("chosen folders: %+v %+v %+v", bare, into, folders())
	}
	expect(t, authdo(r, "POST", base+"/lists", `{"name":"X","folder_id":"nope"}`, alice), http.StatusBadRequest, "a folder elsewhere")
}

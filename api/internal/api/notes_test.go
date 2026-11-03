package api

import (
	"net/http"
	"testing"
)

type noteResp struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Checklist bool   `json:"checklist"`
	Color     string `json:"color"`
	Pinned    bool   `json:"pinned"`
	Items     []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
		Done bool   `json:"done"`
	} `json:"items"`
}

// TestQuickNotes: a workspace with quick notes starts with a pinned tip; new
// notes go on top; a text note becomes a checklist line by line and back;
// notes are searchable, private to members and read-only for viewers.
func TestQuickNotes(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"personal","locale":"pt-PT"}`)
	base := "/api/v1/workspaces/" + ws.ID + "/notes"

	var notes []noteResp
	decode(t, authdo(r, "GET", base, "", alice), &notes)
	if len(notes) != 1 || !notes[0].Pinned || notes[0].Title != "Notas rápidas" || notes[0].Color != "yellow" {
		t.Fatalf("starter note: %+v", notes)
	}

	expect(t, authdo(r, "POST", base, `{"title":"  "}`, alice), http.StatusBadRequest, "empty note")
	expect(t, authdo(r, "POST", base, `{"body":"x","color":"neon"}`, alice), http.StatusBadRequest, "unknown colour")

	w := authdo(r, "POST", base, `{"title":"Groceries","body":"- milk\n• bread\n\n eggs ","color":"green"}`, alice)
	expect(t, w, http.StatusCreated, "create note")
	var n noteResp
	decode(t, w, &n)
	second := authdo(r, "POST", base, `{"checklist":true,"items":[{"text":"call mum"},{"text":"  "}]}`, alice)
	expect(t, second, http.StatusCreated, "create checklist")
	var list noteResp
	decode(t, second, &list)
	if !list.Checklist || len(list.Items) != 1 || list.Items[0].ID == "" {
		t.Fatalf("checklist: %+v", list)
	}

	// Newest first after the pinned ones.
	decode(t, authdo(r, "GET", base, "", alice), &notes)
	if len(notes) != 3 || notes[1].ID != list.ID || notes[2].ID != n.ID {
		t.Fatalf("order: %+v", notes)
	}

	// Text → checklist → text.
	w = authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"checklist":true}`, alice)
	expect(t, w, http.StatusOK, "to checklist")
	decode(t, w, &n)
	if !n.Checklist || n.Body != "" || len(n.Items) != 3 || n.Items[0].Text != "milk" || n.Items[1].Text != "bread" {
		t.Fatalf("to checklist: %+v", n)
	}
	n.Items[0].Done = true
	w = authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"items":[{"id":"`+n.Items[0].ID+`","text":"milk","done":true},{"id":"`+n.Items[1].ID+`","text":"bread"}]}`, alice)
	expect(t, w, http.StatusOK, "tick")
	decode(t, w, &n)
	if len(n.Items) != 2 || !n.Items[0].Done {
		t.Fatalf("tick: %+v", n)
	}
	w = authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"checklist":false}`, alice)
	expect(t, w, http.StatusOK, "to text")
	decode(t, w, &n)
	if n.Checklist || n.Body != "milk\nbread" || len(n.Items) != 0 {
		t.Fatalf("to text: %+v", n)
	}

	// Search finds the text, not the JSON around it.
	var hits []searchHit
	decode(t, authdo(r, "GET", "/api/v1/search?q=bread", "", alice), &hits)
	if len(hits) != 1 || hits[0].Kind != "note" || hits[0].ID != n.ID {
		t.Fatalf("search: %+v", hits)
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=done", "", alice), &hits)
	for _, h := range hits {
		if h.Kind == "note" {
			t.Fatalf("matched the JSON: %+v", hits)
		}
	}

	// Private to members; viewers read only.
	expect(t, authdo(r, "GET", base, "", bob), http.StatusNotFound, "outsider list")
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"pinned":true}`, bob), http.StatusNotFound, "outsider edit")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "invite")
	expect(t, authdo(r, "GET", base, "", bob), http.StatusOK, "viewer list")
	expect(t, authdo(r, "DELETE", "/api/v1/notes/"+n.ID, "", bob), http.StatusForbidden, "viewer delete")

	// Reorder, then delete.
	expect(t, authdo(r, "PUT", base+"/order", `{"ids":["`+n.ID+`","`+list.ID+`"]}`, alice), http.StatusNoContent, "order")
	decode(t, authdo(r, "GET", base, "", alice), &notes)
	if notes[1].ID != n.ID {
		t.Fatalf("reorder: %+v", notes)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/notes/"+n.ID, "", alice), http.StatusNoContent, "delete")
	decode(t, authdo(r, "GET", base, "", alice), &notes)
	if len(notes) != 2 {
		t.Fatalf("after delete: %+v", notes)
	}
}

// TestTaskStart: a task's start stays on or before its due date, and a
// repeating task's start moves with it.
func TestTaskStart(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	type task struct {
		ID    string  `json:"id"`
		Start *string `json:"start"`
		Due   *string `json:"due"`
	}
	w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Report","start":"2026-10-05","due":"2026-10-09"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	var tk task
	decode(t, w, &tk)
	if tk.Start == nil || *tk.Start != "2026-10-05" {
		t.Fatalf("start: %+v", tk)
	}
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"start":"2026-10-10","due":"2026-10-09"}`, alice), http.StatusBadRequest, "start after due")
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"start":"bad"}`, alice), http.StatusBadRequest, "bad start")

	var up struct {
		Task     task `json:"task"`
		Repeated bool `json:"repeated"`
	}
	// Moving the due date before the start pulls the start along.
	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"due":"2026-10-03"}`, alice), &up)
	if *up.Task.Start != "2026-10-03" {
		t.Fatalf("pulled start: %+v", up.Task)
	}
	// Moving the start past the due date pushes the due date.
	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"start":"2026-10-06"}`, alice), &up)
	if *up.Task.Due != "2026-10-06" {
		t.Fatalf("pushed due: %+v", up.Task)
	}

	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"start":"2026-10-05","due":"2026-10-09","repeat":"FREQ=WEEKLY"}`, alice), &up)
	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+tk.ID, `{"completed":true}`, alice), &up)
	if !up.Repeated || *up.Task.Due != "2026-10-16" || *up.Task.Start != "2026-10-12" {
		t.Fatalf("repeat: %+v", up)
	}
}

// TestPersonalNotes: my own notes are mine alone; I can move one into a
// workspace I edit (not one I only view) and back; Today and search see them.
func TestPersonalNotes(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	w := authdo(r, "POST", "/api/v1/me/notes", `{"body":"call the dentist about Friday"}`, alice)
	expect(t, w, http.StatusCreated, "jot")
	var n struct {
		noteResp
		WorkspaceID string `json:"workspace_id"`
		OwnerID     string `json:"owner_id"`
	}
	decode(t, w, &n)
	if n.WorkspaceID != "" || n.OwnerID == "" {
		t.Fatalf("personal note: %+v", n)
	}
	var mine []noteResp
	decode(t, authdo(r, "GET", "/api/v1/me/notes", "", alice), &mine)
	if len(mine) != 1 {
		t.Fatalf("my notes: %+v", mine)
	}
	decode(t, authdo(r, "GET", "/api/v1/me/notes", "", bob), &mine)
	if len(mine) != 0 {
		t.Fatalf("bob sees alice's notes: %+v", mine)
	}
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"pinned":true}`, bob), http.StatusNotFound, "bob edits")

	// Today and search.
	var home struct {
		Notes []noteResp `json:"notes"`
		Mine  []noteResp `json:"mine"`
	}
	decode(t, authdo(r, "GET", "/api/v1/home?today=2026-10-13&from=2026-10-13T00:00:00Z&to=2026-10-20T00:00:00Z", "", alice), &home)
	if len(home.Mine) != 1 || len(home.Notes) != 0 {
		t.Errorf("today: %+v", home)
	}
	var hits []searchHit
	decode(t, authdo(r, "GET", "/api/v1/search?q=dentist", "", alice), &hits)
	if len(hits) != 1 || hits[0].ID != n.ID {
		t.Errorf("search mine: %+v", hits)
	}
	decode(t, authdo(r, "GET", "/api/v1/search?q=dentist", "", bob), &hits)
	if len(hits) != 0 {
		t.Errorf("bob finds alice's note: %+v", hits)
	}

	// Into a workspace: not one bob only views; yes one he edits.
	ws := createWS(t, r, bob, `{"name":"Casa","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"alice","role":"viewer"}`, bob), http.StatusCreated, "invite viewer")
	expect(t, authdo(r, "POST", "/api/v1/notes/"+n.ID+"/move", `{"workspace_id":"`+ws.ID+`"}`, alice), http.StatusForbidden, "move as viewer")
	var members []struct {
		UserID string `json:"user_id"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/members", "", bob), &members)
	for _, m := range members {
		authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/members/"+m.UserID, `{"role":"editor"}`, bob)
	}
	w = authdo(r, "POST", "/api/v1/notes/"+n.ID+"/move", `{"workspace_id":"`+ws.ID+`"}`, alice)
	expect(t, w, http.StatusOK, "move in")
	decode(t, w, &n)
	if n.WorkspaceID != ws.ID || n.OwnerID != "" {
		t.Fatalf("moved: %+v", n)
	}
	decode(t, authdo(r, "GET", "/api/v1/me/notes", "", alice), &mine)
	var wall []noteResp
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/notes", "", bob), &wall)
	if len(mine) != 0 || len(wall) != 2 || wall[1].ID != n.ID && wall[0].ID != n.ID {
		t.Errorf("after the move: mine %d, wall %+v", len(mine), wall)
	}
	// And back to my own notes: bob can't see it any more.
	expect(t, authdo(r, "POST", "/api/v1/notes/"+n.ID+"/move", `{"workspace_id":""}`, alice), http.StatusOK, "move out")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/notes", "", bob), &wall)
	for _, x := range wall {
		if x.ID == n.ID {
			t.Errorf("still in the workspace")
		}
	}
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"pinned":true}`, bob), http.StatusNotFound, "bob after the move out")
}

// TestNoteLang: a note keeps the language its text is coloured as ("" =
// guessed), a name of letters only.
func TestNoteLang(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Ops","kind":"work"}`)
	var n struct {
		ID   string `json:"id"`
		Lang string `json:"lang"`
	}
	w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/notes", `{"body":"{\"a\":1}","lang":"json"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &n)
	if n.Lang != "json" {
		t.Fatalf("lang: %q", n.Lang)
	}
	expect(t, authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lang":"<b>"}`, alice), http.StatusBadRequest, "not a name")
	w = authdo(r, "PUT", "/api/v1/notes/"+n.ID, `{"lang":""}`, alice)
	expect(t, w, http.StatusOK, "back to guessed")
	decode(t, w, &n)
	if n.Lang != "" {
		t.Fatalf("lang: %q", n.Lang)
	}
}

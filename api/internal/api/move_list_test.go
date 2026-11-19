package api

import (
	"net/http"
	"testing"
)

// TestMoveTaskToAnotherList: the task sheet's List picker (PUT list_id) and a
// drag onto another list (POST move) both move a task within its workspace.
func TestMoveTaskToAnotherList(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	var a, b struct {
		ID      string `json:"id"`
		Columns []struct {
			ID string `json:"id"`
		} `json:"columns"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Tasks"}`, alice), &a)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"FOFB - DSBR"}`, alice), &b)
	var task struct {
		ID     string `json:"id"`
		ListID string `json:"list_id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Check pmdb01","list_id":"`+a.ID+`","due":"2026-11-16"}`, alice), &task)

	var moved struct {
		ListID   string `json:"list_id"`
		ColumnID string `json:"column_id"`
	}
	var put struct {
		Task *struct {
			ListID   string `json:"list_id"`
			ColumnID string `json:"column_id"`
		} `json:"task"`
	}
	w := authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"list_id":"`+b.ID+`"}`, alice)
	expect(t, w, http.StatusOK, "move by the sheet: "+w.Body.String())
	decode(t, w, &put)
	if put.Task != nil {
		moved.ListID, moved.ColumnID = put.Task.ListID, put.Task.ColumnID
	}
	if moved.ListID != b.ID || moved.ColumnID != b.Columns[0].ID {
		t.Fatalf("after the sheet's move: list %s column %s, want %s %s", moved.ListID, moved.ColumnID, b.ID, b.Columns[0].ID)
	}
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"list_id":"no-such-list"}`, alice), http.StatusBadRequest, "unknown list")
	w = authdo(r, "POST", "/api/v1/tasks/"+task.ID+"/move", `{"list_id":"`+a.ID+`","column_id":"`+a.Columns[0].ID+`"}`, alice)
	expect(t, w, http.StatusOK, "move by drag: "+w.Body.String())
}

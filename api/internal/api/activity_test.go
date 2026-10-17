package api

import (
	"net/http"
	"testing"
)

// TestTaskActivity: creating, renaming, dating, assigning, moving and
// completing a task are each a line of its activity; notes edits in a row
// count once.
func TestTaskActivity(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	var list struct {
		ID      string `json:"id"`
		Columns []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"columns"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Chores"}`, alice), &list)
	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Fix tap","list_id":"`+list.ID+`"}`, alice), &task)
	bobID := meID(t, r, bob)
	put := func(body string) {
		expect(t, authdo(r, "PUT", "/api/v1/tasks/"+task.ID, body, alice), http.StatusOK, body)
	}
	put(`{"title":"Fix the tap"}`)
	put(`{"due":"2026-11-02","priority":2}`)
	put(`{"assignee_ids":["` + bobID + `"]}`)
	put(`{"notes":"a"}`)
	put(`{"notes":"ab"}`)
	put(`{"completed":true}`)

	var acts []struct {
		Kind      string         `json:"kind"`
		ActorName string         `json:"actor_name"`
		Params    map[string]any `json:"params"`
	}
	decode(t, authdo(r, "GET", "/api/v1/tasks/"+task.ID+"/activity", "", bob), &acts)
	var kinds []string
	for _, a := range acts {
		kinds = append(kinds, a.Kind)
	}
	want := []string{"created", "renamed", "due", "priority", "assigned", "notes", "completed"}
	if len(kinds) != len(want) {
		t.Fatalf("kinds %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds %v, want %v", kinds, want)
		}
	}
	if acts[0].ActorName != "alice" || acts[4].Params["names"].([]any)[0] != "bob" || acts[1].Params["to"] != "Fix the tap" {
		t.Fatalf("activity: %+v", acts)
	}
}

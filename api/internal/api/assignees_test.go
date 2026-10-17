package api

import (
	"net/http"
	"testing"
)

// TestSeveralAssignees: a task can be for several members, each told once;
// only members can be given it; the single assignee_id still works.
func TestSeveralAssignees(t *testing.T) {
	r, admin, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	id := func(tok string) string { return meID(t, r, tok) }
	aliceID, bobID, adminID := id(alice), id(bob), id(admin)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Chores"}`, alice), http.StatusCreated, "list")

	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"X","assignee_ids":["`+adminID+`"]}`, alice), http.StatusBadRequest, "non-member")
	w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Paint the fence","assignee_ids":["`+aliceID+`","`+bobID+`","`+bobID+`"]}`, alice)
	expect(t, w, http.StatusCreated, "task")
	var task struct {
		ID          string   `json:"id"`
		AssigneeIDs []string `json:"assignee_ids"`
		AssigneeID  *string  `json:"assignee_id"`
	}
	decode(t, w, &task)
	if len(task.AssigneeIDs) != 2 || task.AssigneeID == nil || *task.AssigneeID != aliceID {
		t.Fatalf("task: %+v", task)
	}
	count := func(tok string) (n int) {
		for _, it := range inboxOf(t, r, tok).Items {
			if it.Verb == "task_assigned" {
				n++
			}
		}
		return n
	}
	if count(bob) != 1 || count(alice) != 0 {
		t.Fatalf("assigned: bob %d, alice %d", count(bob), count(alice))
	}
	// The older single field replaces the list.
	var upd struct {
		Task struct {
			AssigneeIDs []string `json:"assignee_ids"`
			AssigneeID  *string  `json:"assignee_id"`
		} `json:"task"`
	}
	w = authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"assignee_id":"`+bobID+`"}`, alice)
	expect(t, w, http.StatusOK, "legacy")
	decode(t, w, &upd)
	task.AssigneeIDs, task.AssigneeID = upd.Task.AssigneeIDs, upd.Task.AssigneeID
	if len(task.AssigneeIDs) != 1 || task.AssigneeIDs[0] != bobID {
		t.Fatalf("after legacy: %+v", task)
	}
	w = authdo(r, "PUT", "/api/v1/tasks/"+task.ID, `{"assignee_ids":[]}`, alice)
	decode(t, w, &upd)
	task.AssigneeIDs, task.AssigneeID = upd.Task.AssigneeIDs, upd.Task.AssigneeID
	if len(task.AssigneeIDs) != 0 || task.AssigneeID != nil {
		t.Fatalf("after clearing: %+v", task)
	}
}

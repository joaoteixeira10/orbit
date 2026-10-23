package api

import (
	"net/http"
	"testing"
)

type goalResp struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Due        *string `json:"due"`
	Progress   int     `json:"progress"`
	Tasks      int     `json:"tasks"`
	Done       int     `json:"done"`
	AchievedAt *string `json:"achieved_at"`
}

// TestGoals: a goal's progress comes from its tasks; a task can only count
// towards a goal of its workspace; deleting a goal keeps its tasks.
func TestGoals(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Launch","kind":"work"}`)
	other := createWS(t, r, alice, `{"name":"Else","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID

	expect(t, authdo(r, "POST", base+"/goals", `{"title":"Ship v1","due":"2026-13-40"}`, alice), http.StatusBadRequest, "bad date")
	var g goalResp
	w := authdo(r, "POST", base+"/goals", `{"title":"Ship v1","due":"2026-12-31","icon":"🚀"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &g)
	var og goalResp
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+other.ID+"/goals", `{"title":"Other"}`, alice), &og)

	ids := []string{}
	for _, title := range []string{"Docs", "Tests", "Release notes"} {
		var tk struct {
			ID string `json:"id"`
		}
		decode(t, authdo(r, "POST", base+"/tasks", `{"title":"`+title+`","goal_id":"`+g.ID+`"}`, alice), &tk)
		ids = append(ids, tk.ID)
	}
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+ids[0], `{"completed":true}`, alice), http.StatusOK, "done one")
	if w := authdo(r, "PUT", "/api/v1/tasks/"+ids[1], `{"goal_id":"`+og.ID+`"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("a goal of another workspace: %d", w.Code)
	}
	var list []goalResp
	decode(t, authdo(r, "GET", base+"/goals", "", alice), &list)
	if len(list) != 1 || list[0].Tasks != 3 || list[0].Done != 1 {
		t.Fatalf("goals: %+v", list)
	}
	expect(t, authdo(r, "PUT", "/api/v1/goals/"+g.ID, `{"progress":120}`, alice), http.StatusBadRequest, "120%")
	decode(t, authdo(r, "PUT", "/api/v1/goals/"+g.ID, `{"achieved":true}`, alice), &g)
	if g.AchievedAt == nil {
		t.Fatal("not achieved")
	}
	expect(t, authdo(r, "DELETE", "/api/v1/goals/"+g.ID, "", alice), http.StatusNoContent, "delete")
	var tasks []struct {
		ID     string `json:"id"`
		GoalID string `json:"goal_id"`
	}
	decode(t, authdo(r, "GET", base+"/tasks?state=all", "", alice), &tasks)
	if len(tasks) < 3 {
		t.Fatalf("tasks gone: %d", len(tasks))
	}
	for _, tk := range tasks {
		if tk.GoalID != "" {
			t.Errorf("still towards the deleted goal: %+v", tk)
		}
	}
}

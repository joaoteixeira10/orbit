package api

import (
	"net/http"
	"testing"
)

// TestEstimatesAndBlocks: a task's estimate; a plan of time blocks replaces
// mine over its range (not someone else's, not outside it), is mine alone,
// and goes with its task.
func TestEstimatesAndBlocks(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID
	var a, b struct {
		ID       string `json:"id"`
		Estimate int    `json:"estimate"`
	}
	decode(t, authdo(r, "POST", base+"/tasks", `{"title":"Report","estimate":90}`, alice), &a)
	decode(t, authdo(r, "POST", base+"/tasks", `{"title":"Slides"}`, alice), &b)
	if a.Estimate != 90 {
		t.Fatalf("estimate: %d", a.Estimate)
	}
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+b.ID, `{"estimate":20000}`, alice), http.StatusBadRequest, "over a week")

	week := `"from":"2026-11-02T00:00:00Z","to":"2026-11-09T00:00:00Z","workspace_ids":["` + ws.ID + `"]`
	plan := `{` + week + `,"blocks":[{"task_id":"` + a.ID + `","starts_at":"2026-11-02T09:00:00Z","ends_at":"2026-11-02T10:00:00Z"},{"task_id":"` + a.ID + `","starts_at":"2026-11-03T09:00:00Z","ends_at":"2026-11-03T09:30:00Z"},{"task_id":"` + b.ID + `","starts_at":"2026-11-03T10:00:00Z","ends_at":"2026-11-03T11:00:00Z"}]}`
	w := authdo(r, "PUT", "/api/v1/me/blocks", plan, alice)
	expect(t, w, http.StatusOK, "plan")
	var blocks []blockResponse
	decode(t, w, &blocks)
	if len(blocks) != 3 || blocks[0].Title != "Report" || blocks[2].Title != "Slides" {
		t.Fatalf("planned: %+v", blocks)
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/blocks", `{`+week+`,"blocks":[{"task_id":"`+a.ID+`","starts_at":"2026-11-10T09:00:00Z","ends_at":"2026-11-10T10:00:00Z"}]}`, alice), http.StatusBadRequest, "outside")
	expect(t, authdo(r, "PUT", "/api/v1/me/blocks", `{`+week+`,"blocks":[]}`, bob), http.StatusNotFound, "not a member")

	// Planned again: replaced, not added.
	expect(t, authdo(r, "PUT", "/api/v1/me/blocks", `{`+week+`,"blocks":[{"task_id":"`+a.ID+`","starts_at":"2026-11-04T09:00:00Z","ends_at":"2026-11-04T10:30:00Z"}]}`, alice), http.StatusOK, "replan")
	decode(t, authdo(r, "GET", "/api/v1/me/blocks?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z", "", alice), &blocks)
	if len(blocks) != 1 {
		t.Fatalf("after replan: %+v", blocks)
	}
	decode(t, authdo(r, "GET", "/api/v1/me/blocks?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z", "", bob), &blocks)
	if len(blocks) != 0 {
		t.Errorf("someone else's: %+v", blocks)
	}
	// The task goes; its blocks too.
	expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+a.ID, "", alice), http.StatusNoContent, "delete task")
	decode(t, authdo(r, "GET", "/api/v1/me/blocks?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z", "", alice), &blocks)
	if len(blocks) != 0 {
		t.Errorf("after the task went: %+v", blocks)
	}
}

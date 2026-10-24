package api

import (
	"net/http"
	"testing"
)

type depTask struct {
	ID      string   `json:"id"`
	Start   *string  `json:"start"`
	Due     *string  `json:"due"`
	WaitsOn []string `json:"waits_on"`
}

// TestTaskDependencies: a task waits on others of its workspace (no loops);
// moving one's end moves what waits on it; deleting one lets go of it.
func TestTaskDependencies(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Build","kind":"work"}`)
	other := createWS(t, r, alice, `{"name":"Else","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID
	mk := func(wsBase, body string) depTask {
		var x depTask
		w := authdo(r, "POST", wsBase+"/tasks", body, alice)
		expect(t, w, http.StatusCreated, "task")
		decode(t, w, &x)
		return x
	}
	design := mk(base, `{"title":"Design","start":"2026-11-02","due":"2026-11-04"}`)
	build := mk(base, `{"title":"Build","start":"2026-11-05","due":"2026-11-10","waits_on":["`+design.ID+`"]}`)
	if len(build.WaitsOn) != 1 || build.WaitsOn[0] != design.ID {
		t.Fatalf("build waits on %v", build.WaitsOn)
	}
	test := mk(base, `{"title":"Test","due":"2026-11-12","waits_on":["`+build.ID+`"]}`)
	elsewhere := mk("/api/v1/workspaces/"+other.ID, `{"title":"Elsewhere"}`)

	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+design.ID, `{"waits_on":["`+test.ID+`"]}`, alice), http.StatusBadRequest, "a loop")
	expect(t, authdo(r, "PUT", "/api/v1/tasks/"+design.ID, `{"waits_on":["`+elsewhere.ID+`"]}`, alice), http.StatusBadRequest, "another workspace")

	// Design slips two days: build and test follow.
	var res struct {
		Shifted int `json:"shifted"`
	}
	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+design.ID, `{"due":"2026-11-06"}`, alice), &res)
	if res.Shifted != 2 {
		t.Fatalf("shifted %d", res.Shifted)
	}
	var list []depTask
	decode(t, authdo(r, "GET", base+"/tasks", "", alice), &list)
	got := map[string]depTask{}
	for _, x := range list {
		got[x.ID] = x
	}
	if b := got[build.ID]; *b.Start != "2026-11-07" || *b.Due != "2026-11-12" {
		t.Errorf("build: %s – %s", *b.Start, *b.Due)
	}
	if x := got[test.ID]; *x.Due != "2026-11-14" {
		t.Errorf("test: %s", *x.Due)
	}

	// Build goes: test no longer waits on it.
	expect(t, authdo(r, "DELETE", "/api/v1/tasks/"+build.ID, "", alice), http.StatusNoContent, "delete")
	decode(t, authdo(r, "GET", base+"/tasks", "", alice), &list)
	for _, x := range list {
		if x.ID == test.ID && len(x.WaitsOn) != 0 {
			t.Errorf("test still waits on %v", x.WaitsOn)
		}
	}
}

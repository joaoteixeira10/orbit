package api

import (
	"net/http"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/focus"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

type focusStateResp struct {
	Running *struct {
		ID        string `json:"id"`
		TaskID    string `json:"task_id"`
		TaskTitle string `json:"task_title"`
		Minutes   int    `json:"minutes"`
	} `json:"running"`
	Today []struct {
		Spent int `json:"spent"`
	} `json:"today"`
	Minutes int `json:"minutes"`
}

// TestFocus: one running session at a time; stopping early counts the time
// so far, on the task; a session whose time is up ends at its planned end.
func TestFocus(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	var task struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Write the report"}`, alice), &task)

	if w := authdo(r, "POST", "/api/v1/me/focus", `{"workspace_id":"`+ws.ID+`","minutes":25}`, bob); w.Code != http.StatusNotFound && w.Code != http.StatusForbidden {
		t.Errorf("an outsider focuses in it: %d", w.Code)
	}
	var st focusStateResp
	w := authdo(r, "POST", "/api/v1/me/focus", `{"workspace_id":"`+ws.ID+`","task_id":"`+task.ID+`","minutes":25}`, alice)
	expect(t, w, http.StatusCreated, "start")
	decode(t, w, &st)
	if st.Running == nil || st.Running.TaskTitle != "Write the report" || st.Running.Minutes != 25 {
		t.Fatalf("running: %+v", st.Running)
	}
	// Ten minutes in, stopped: ten minutes on the task.
	db.Model(&models.FocusSession{}).Where("id = ?", st.Running.ID).Update("started_at", time.Now().Add(-10*time.Minute))
	w = authdo(r, "POST", "/api/v1/me/focus/stop", "", alice)
	decode(t, w, &st)
	if st.Running != nil || len(st.Today) != 1 || st.Minutes != 10 {
		t.Fatalf("stopped: %+v", st)
	}
	var tk models.Task
	db.First(&tk, "id = ?", task.ID)
	if tk.FocusMinutes != 10 {
		t.Fatalf("on the task: %d", tk.FocusMinutes)
	}

	// A new one replaces a running one; its time up, it ends at 25.
	decode(t, authdo(r, "POST", "/api/v1/me/focus", `{"workspace_id":"`+ws.ID+`","task_id":"`+task.ID+`","minutes":25}`, alice), &st)
	decode(t, authdo(r, "POST", "/api/v1/me/focus", `{"workspace_id":"`+ws.ID+`","task_id":"`+task.ID+`","minutes":25}`, alice), &st)
	var running int64
	db.Model(&models.FocusSession{}).Where("ended_at IS NULL").Count(&running)
	if running != 1 {
		t.Fatalf("running: %d", running)
	}
	db.Model(&models.FocusSession{}).Where("ended_at IS NULL").Update("started_at", time.Now().Add(-40*time.Minute))
	due := focus.Due(db, time.Now())
	if len(due) != 1 || !focus.End(db, &due[0], time.Now()) || due[0].Spent != 25 || focus.End(db, &due[0], time.Now()) {
		t.Fatalf("time up: %+v", due)
	}
	db.First(&tk, "id = ?", task.ID)
	if tk.FocusMinutes != 35 {
		t.Fatalf("on the task after: %d", tk.FocusMinutes)
	}
}

// TestFocusLengths: a person's own lengths for the focus timer are kept with
// their preferences, each 1 to 180 minutes, a few at most.
func TestFocusLengths(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p struct {
		FocusLengths []int `json:"focus_lengths"`
	}
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if p.FocusLengths == nil || len(p.FocusLengths) != 0 {
		t.Errorf("none yet: %#v", p.FocusLengths)
	}
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"focus_lengths":[0]}`, owner), http.StatusBadRequest, "too short")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"focus_lengths":[181]}`, owner), http.StatusBadRequest, "too long")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"focus_lengths":[1,2,3,4,5,6,7,8,9]}`, owner), http.StatusBadRequest, "too many")
	expect(t, authdo(r, "PUT", "/api/v1/me/prefs", `{"focus_lengths":[40,90]}`, owner), http.StatusOK, "save")
	decode(t, authdo(r, "GET", "/api/v1/me/prefs", "", owner), &p)
	if len(p.FocusLengths) != 2 || p.FocusLengths[0] != 40 || p.FocusLengths[1] != 90 {
		t.Errorf("saved: %#v", p.FocusLengths)
	}
}

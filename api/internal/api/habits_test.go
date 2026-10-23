package api

import (
	"net/http"
	"testing"
	"time"
)

type habitResp struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Schedule string   `json:"schedule"`
	Days     string   `json:"days"`
	Target   int      `json:"target"`
	Checks   []string `json:"checks"`
}

// TestHabits: habits are their owner's alone; a day is checked and
// unchecked (not in the future); schedules are checked; deleting takes the
// history.
func TestHabits(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Home","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	base := "/api/v1/workspaces/" + ws.ID + "/habits"

	expect(t, authdo(r, "POST", base, `{"title":" "}`, alice), http.StatusBadRequest, "no name")
	expect(t, authdo(r, "POST", base, `{"title":"Run","days":"0000000"}`, alice), http.StatusBadRequest, "no days")
	expect(t, authdo(r, "POST", base, `{"title":"Gym","schedule":"weekly","target":9}`, alice), http.StatusBadRequest, "9 a week")
	var hb habitResp
	w := authdo(r, "POST", base, `{"title":"Read","icon":"📚","days":"1111100","remind_at":"21:00"}`, alice)
	expect(t, w, http.StatusCreated, "create")
	decode(t, w, &hb)
	if hb.Schedule != "daily" || hb.Days != "1111100" || len(hb.Checks) != 0 {
		t.Fatalf("habit: %+v", hb)
	}
	expect(t, authdo(r, "POST", base, `{"title":"Piano","schedule":"weekly","target":3}`, bob), http.StatusCreated, "bob's")

	var mine []habitResp
	decode(t, authdo(r, "GET", base, "", alice), &mine)
	if len(mine) != 1 || mine[0].Title != "Read" {
		t.Fatalf("alice sees: %+v", mine)
	}
	if w := authdo(r, "PUT", "/api/v1/habits/"+hb.ID, `{"title":"x"}`, bob); w.Code != http.StatusNotFound {
		t.Errorf("bob changed alice's habit: %d", w.Code)
	}

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	expect(t, authdo(r, "PUT", "/api/v1/habits/"+hb.ID+"/checks/"+yesterday, `{"done":true}`, alice), http.StatusOK, "yesterday")
	expect(t, authdo(r, "PUT", "/api/v1/habits/"+hb.ID+"/checks/"+yesterday, `{"done":true}`, alice), http.StatusOK, "again")
	w = authdo(r, "PUT", "/api/v1/habits/"+hb.ID+"/checks/"+today, `{"done":true}`, alice)
	decode(t, w, &hb)
	if len(hb.Checks) != 2 || hb.Checks[0] != yesterday || hb.Checks[1] != today {
		t.Fatalf("checks: %v", hb.Checks)
	}
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	expect(t, authdo(r, "PUT", "/api/v1/habits/"+hb.ID+"/checks/"+future, `{"done":true}`, alice), http.StatusBadRequest, "the future")
	decode(t, authdo(r, "PUT", "/api/v1/habits/"+hb.ID+"/checks/"+today, `{"done":false}`, alice), &hb)
	if len(hb.Checks) != 1 {
		t.Fatalf("unchecked: %v", hb.Checks)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/habits/"+hb.ID, "", alice), http.StatusNoContent, "delete")
	decode(t, authdo(r, "GET", base, "", alice), &mine)
	if len(mine) != 0 {
		t.Fatalf("after delete: %+v", mine)
	}
}

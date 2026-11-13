package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/search"
)

type journalDayResp struct {
	Entry *struct {
		Date    string `json:"date"`
		Content string `json:"content"`
		Mood    int    `json:"mood"`
	} `json:"entry"`
	OnThisDay []struct {
		Date    string `json:"date"`
		Excerpt string `json:"excerpt"`
	} `json:"on_this_day"`
}

// TestJournal: a day is written (text, mood), listed by month, found only
// by its writer, shown again on the same day a year on; an emptied day goes.
func TestJournal(t *testing.T) {
	r, db, _, alice, bob := plannerSetupDB(t)
	if !search.Setup(db) {
		t.Skip("no FTS5")
	}
	t.Cleanup(func() { search.Setup(nil) })
	ws := createWS(t, r, alice, `{"name":"Home","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")
	base := "/api/v1/workspaces/" + ws.ID + "/journal/"

	expect(t, authdo(r, "PUT", base+"2025-10-23", `{"content":"Walked to the **lighthouse**.","mood":4}`, alice), http.StatusOK, "last year")
	expect(t, authdo(r, "PUT", base+"2026-10-23", `{"content":"A quiet day at home.","mood":3}`, alice), http.StatusOK, "this year")
	expect(t, authdo(r, "PUT", base+"2026-10-23", `{"mood":6}`, alice), http.StatusBadRequest, "mood 6")
	var day journalDayResp
	decode(t, authdo(r, "GET", base+"2026-10-23", "", alice), &day)
	if day.Entry == nil || day.Entry.Mood != 3 || len(day.OnThisDay) != 1 || day.OnThisDay[0].Date != "2025-10-23" || day.OnThisDay[0].Excerpt != "Walked to the lighthouse." {
		t.Fatalf("day: %+v", day)
	}
	// Bob's journal in the same workspace is his own.
	decode(t, authdo(r, "GET", base+"2026-10-23", "", bob), &day)
	if day.Entry != nil || len(day.OnThisDay) != 0 {
		t.Fatalf("bob sees alice's: %+v", day)
	}
	var month []struct{ Date string }
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/journal?month=2026-10", "", alice), &month)
	if len(month) != 1 {
		t.Fatalf("month: %+v", month)
	}
	if s := authdo(r, "GET", "/api/v1/search?q=lighthouse", "", alice).Body.String(); !strings.Contains(s, `"journal"`) || !strings.Contains(s, "2025-10-23") {
		t.Errorf("alice doesn't find it: %s", s)
	}
	if s := authdo(r, "GET", "/api/v1/search?q=lighthouse", "", bob).Body.String(); strings.Contains(s, "2025-10-23") {
		t.Errorf("bob finds alice's journal: %s", s)
	}
	// Emptied: gone.
	expect(t, authdo(r, "PUT", base+"2026-10-23", `{"content":"","mood":0}`, alice), http.StatusOK, "empty")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/journal?month=2026-10", "", alice), &month)
	if len(month) != 0 {
		t.Fatalf("emptied day kept: %+v", month)
	}
}

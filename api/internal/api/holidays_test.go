package api

import (
	"net/http"
	"testing"
)

// TestCalendarHolidays: a workspace's owner picks a country; its holidays
// show on the calendar as read-only all-day entries (once on the merged
// calendar, whatever the number of workspaces with it).
func TestCalendarHolidays(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Casa"}`)
	ws2 := createWS(t, r, alice, `{"name":"Escola"}`)
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID, `{"holidays":"XX"}`, alice), http.StatusBadRequest, "unknown")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID, `{"holidays":"PT"}`, alice), http.StatusOK, "PT")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws2.ID, `{"holidays":"PT"}`, alice), http.StatusOK, "PT again")
	var cal struct {
		Events []struct {
			Kind   string `json:"kind"`
			Title  string `json:"title"`
			AllDay bool   `json:"all_day"`
		} `json:"events"`
	}
	decode(t, authdo(r, "GET", "/api/v1/calendar?from=2026-12-01T00:00:00Z&to=2026-12-31T00:00:00Z", "", alice), &cal)
	n := 0
	for _, e := range cal.Events {
		if e.Kind == "holiday" {
			n++
			if !e.AllDay {
				t.Fatalf("not all day: %+v", e)
			}
		}
	}
	if n != 3 { // 1 Dec, 8 Dec, 25 Dec
		t.Fatalf("holidays in December: %d (%+v)", n, cal.Events)
	}
	var countries []struct {
		Code string `json:"code"`
	}
	decode(t, authdo(r, "GET", "/api/v1/holidays/countries", "", alice), &countries)
	if len(countries) < 5 || countries[0].Code != "PT" {
		t.Fatalf("countries: %+v", countries)
	}
}

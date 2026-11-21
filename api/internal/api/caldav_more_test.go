package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/emersion/go-webdav/caldav"
	"gorm.io/gorm"

	dbpkg "github.com/joaoteixeira10/orbit/api/internal/db"
	"github.com/joaoteixeira10/orbit/api/internal/google"
	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

func ics(uid, title, start string) []byte {
	return []byte(strings.ReplaceAll("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Google//EN\nBEGIN:VEVENT\nUID:"+uid+"\nDTSTAMP:20261001T080000Z\nDTSTART:"+start+"\nDTEND:"+start+"\nSUMMARY:"+title+"\nEND:VEVENT\nEND:VCALENDAR\n", "\n", "\r\n"))
}

// TestExtraCalendars: more calendars of the same Google account in one
// workspace — each its own calendar (its events, its colour), a writable
// one synced both ways, a read-only one only coming in (its events not
// changed in Orbit); an event moved between calendars moves there too; an
// extra calendar taken away takes its events with it.
func TestExtraCalendars(t *testing.T) {
	mem := &gCal{objs: map[string][]byte{
		"/caldav/v2/work-cal/events/a.ics":  ics("a", "Standup", "20261105T090000Z"),
		"/caldav/v2/team/events/b.ics":      ics("b", "Team lunch", "20261106T120000Z"),
		"/caldav/v2/birthdays/events/c.ics": ics("c", "Ana's birthday", "20261107T000000Z"),
	}}
	davH := &caldav.Handler{Backend: mem, Prefix: "/caldav"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"acc-1","expires_in":3600}`))
		case r.URL.Path == "/calendarList":
			_, _ = w.Write([]byte(`{"items":[
				{"id":"birthdays","summary":"Aniversários","accessRole":"reader","backgroundColor":"#f6bf26"},
				{"id":"team","summary":"Eventos","accessRole":"writer","backgroundColor":"#33b679"},
				{"id":"work-cal","summary":"João","accessRole":"owner","primary":true}]}`))
		case strings.HasPrefix(r.URL.Path, "/caldav/v2/"):
			davH.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := []string{google.TokenURL, google.CalendarListURL, google.CalDAVBase}
	oldHTTP := google.HTTP
	t.Cleanup(func() {
		google.TokenURL, google.CalendarListURL, google.CalDAVBase = old[0], old[1], old[2]
		google.HTTP = oldHTTP
		testDAVClient = nil
	})
	google.TokenURL, google.CalendarListURL, google.CalDAVBase = srv.URL+"/token", srv.URL+"/calendarList", srv.URL+"/caldav/v2/"
	google.HTTP = func() *http.Client { return srv.Client() }
	testDAVClient = &http.Client{}

	r, db := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	expect(t, authdo(r, "PUT", "/api/v1/google/app", `{"client_id":"cid.apps.googleusercontent.com","client_secret":"shh"}`, owner), http.StatusOK, "app")
	ws := createWS(t, r, owner, `{"name":"Me","kind":"personal","starter":false}`)
	base := "/api/v1/workspaces/" + ws.ID
	sealer := testSealer(t, db)
	refresh, _ := sealer.Seal([]byte("ref-1"))
	main := models.CalDAVLink{WorkspaceID: ws.ID, Provider: "google", CalendarURL: google.CalDAVURL("work-cal"), CalendarName: "João", Host: "calendar.google.com", Username: "joao@example.com", PasswordSealed: refresh}
	db.Create(&main)
	expect(t, authdo(r, "POST", base+"/caldav/sync", "", owner), http.StatusOK, "first sync")

	// The account's other calendars.
	var cals []accountCalendar
	decode(t, authdo(r, "GET", base+"/caldav/"+main.ID+"/calendars", "", owner), &cals)
	if len(cals) != 3 || cals[0].Key != "work-cal" || !cals[0].Linked || cals[2].Key != "birthdays" || cals[2].Writable {
		t.Fatalf("account calendars: %+v", cals)
	}
	expect(t, authdo(r, "POST", base+"/caldav/"+main.ID+"/calendars", `{"key":"work-cal"}`, owner), http.StatusBadRequest, "synced already")
	expect(t, authdo(r, "POST", base+"/caldav/"+main.ID+"/calendars", `{"key":"team"}`, owner), http.StatusOK, "add a writable one")
	w := authdo(r, "POST", base+"/caldav/"+main.ID+"/calendars", `{"key":"birthdays"}`, owner)
	expect(t, w, http.StatusOK, "add a read-only one")
	var got caldavLinkResponse
	decode(t, w, &got)
	if len(got.Links) != 3 || got.Link == nil || got.Link.ID != main.ID {
		t.Fatalf("links: %+v", got)
	}
	var team, bdays models.CalDAVLink
	for _, l := range got.Links {
		switch l.CalendarName {
		case "Eventos":
			team = l
		case "Aniversários":
			bdays = l
		}
	}
	if !team.Extra || team.ReadOnly || team.Color != "#33b679" || !bdays.ReadOnly || bdays.LastError != "" {
		t.Fatalf("extras: %+v / %+v", team, bdays)
	}

	// Each event in its calendar.
	byTitle := func() map[string]models.Event {
		var evs []models.Event
		db.Where("workspace_id = ?", ws.ID).Find(&evs)
		m := map[string]models.Event{}
		for _, e := range evs {
			m[e.Title] = e
		}
		return m
	}
	evs := byTitle()
	if evs["Standup"].CalendarID != "" || evs["Team lunch"].CalendarID != team.ID || evs["Ana's birthday"].CalendarID != bdays.ID || len(evs) != 3 {
		t.Fatalf("events: %+v", evs)
	}
	var cal struct {
		Events []occurrence `json:"events"`
	}
	decode(t, authdo(r, "GET", base+"/calendar?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z", "", owner), &cal)
	for _, o := range cal.Events {
		if o.Title == "Ana's birthday" && (!o.ReadOnly || o.CalendarID != bdays.ID || o.Color != "#f6bf26") {
			t.Errorf("birthday: %+v", o)
		}
	}

	// A read-only calendar's events aren't changed here, nor added to.
	expect(t, authdo(r, "PUT", "/api/v1/events/"+evs["Ana's birthday"].ID, `{"title":"x"}`, owner), http.StatusForbidden, "read-only event")
	expect(t, authdo(r, "DELETE", "/api/v1/events/"+evs["Ana's birthday"].ID, "", owner), http.StatusForbidden, "read-only delete")
	expect(t, authdo(r, "POST", base+"/events", `{"title":"Party","starts_at":"2026-11-10T20:00:00Z","ends_at":"2026-11-10T22:00:00Z","calendar_id":"`+bdays.ID+`"}`, owner), http.StatusForbidden, "into read-only")
	expect(t, authdo(r, "POST", base+"/events", `{"title":"Party","starts_at":"2026-11-10T20:00:00Z","ends_at":"2026-11-10T22:00:00Z","calendar_id":"nope"}`, owner), http.StatusBadRequest, "no such calendar")

	// A new event in the team calendar goes there only.
	expect(t, authdo(r, "POST", base+"/events", `{"title":"Retro","starts_at":"2026-11-12T15:00:00Z","ends_at":"2026-11-12T16:00:00Z","calendar_id":"`+team.ID+`"}`, owner), http.StatusCreated, "into team")
	expect(t, authdo(r, "POST", base+"/caldav/sync", "", owner), http.StatusOK, "sync")
	where := func(title string) []string {
		mem.mu.Lock()
		defer mem.mu.Unlock()
		var out []string
		for p, b := range mem.objs {
			if strings.Contains(string(b), "SUMMARY:"+title) {
				out = append(out, strings.Split(p, "/")[3])
			}
		}
		return out
	}
	if got := where("Retro"); len(got) != 1 || got[0] != "team" {
		t.Fatalf("Retro is in %v", got)
	}
	if got := where("Ana's birthday"); len(got) != 1 {
		t.Fatalf("the birthday went somewhere: %v", got)
	}

	// Moved to the workspace's Events: it moves to that calendar.
	retro := byTitle()["Retro"]
	expect(t, authdo(r, "PUT", "/api/v1/events/"+retro.ID, `{"calendar_id":""}`, owner), http.StatusOK, "move")
	expect(t, authdo(r, "POST", base+"/caldav/sync", "", owner), http.StatusOK, "sync")
	if got := where("Retro"); len(got) != 1 || got[0] != "work-cal" {
		t.Fatalf("after the move Retro is in %v", got)
	}

	// The team calendar taken away: its events leave Orbit; the rest stay.
	expect(t, authdo(r, "DELETE", base+"/caldav/"+team.ID, "", owner), http.StatusOK, "remove team")
	evs = byTitle()
	if _, ok := evs["Team lunch"]; ok || evs["Standup"].ID == "" || evs["Retro"].ID == "" {
		t.Fatalf("after removing team: %+v", evs)
	}
	if got := where("Team lunch"); len(got) != 1 {
		t.Errorf("it stays on Google: %v", got)
	}
}

// testSealer is the test routers' sealer (the same master key).
func testSealer(t *testing.T, db *gorm.DB) *keystore.Sealer {
	t.Helper()
	s, err := dbpkg.OpenSealer(db, "test-master-key-0123456789-abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// decode unmarshals a response body into v, failing the test on bad JSON.
func decode(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
}

// expect fails the test unless the response has the wanted status.
func expect(t *testing.T, w *httptest.ResponseRecorder, code int, what string) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("%s: got %d, want %d: %s", what, w.Code, code, w.Body.String())
	}
}

// plannerSetup returns a router, the admin's token, and two members' tokens
// (alice and bob, created with the default member role).
func plannerSetup(t *testing.T) (r *gin.Engine, admin, alice, bob string) {
	t.Helper()
	r, _, admin, alice, bob = plannerSetupDB(t)
	return
}

// plannerSetupDB is plannerSetup with the database.
func plannerSetupDB(t *testing.T) (r *gin.Engine, db *gorm.DB, admin, alice, bob string) {
	t.Helper()
	r, db = newTestRouterDB(t, false)
	admin = loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"alice","email":"alice@example.org","password":"alicepassword"}`)
	createUserVia(t, r, admin, `{"username":"bob","password":"bobpassword"}`)
	return r, db, admin, loginTok(t, r, "alice", "alicepassword"), loginTok(t, r, "bob", "bobpassword")
}

type wsResp struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Modules []string `json:"modules"`
	Role    string   `json:"role"`
}

func createWS(t *testing.T, r *gin.Engine, tok, body string) wsResp {
	t.Helper()
	w := authdo(r, "POST", "/api/v1/workspaces", body, tok)
	expect(t, w, http.StatusCreated, "create workspace")
	var ws wsResp
	decode(t, w, &ws)
	return ws
}

// TestWorkspaceFromTemplate: a school workspace starts with its modules, its
// localised lists (with board columns) and a welcome page; its creator owns it.
func TestWorkspaceFromTemplate(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Escola","kind":"school","locale":"pt-PT"}`)
	if ws.Role != "owner" || ws.Kind != "school" || len(ws.Modules) != 9 || ws.Modules[8] != "files" {
		t.Fatalf("workspace: %+v", ws)
	}
	var lists []struct {
		Name    string `json:"name"`
		Columns []struct {
			Name string `json:"name"`
			Done bool   `json:"done"`
		} `json:"columns"`
	}
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/lists", "", alice), &lists)
	if len(lists) != 2 || lists[0].Name != "Trabalhos" || lists[0].Columns[2].Name != "Entregue" || !lists[0].Columns[2].Done {
		t.Errorf("lists: %+v", lists)
	}
	var pages []pageSummary
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &pages)
	if len(pages) != 2 || pages[0].Title != "Bem-vindo" {
		t.Errorf("pages: %+v", pages)
	}
	// Custom modules, without starter content.
	ws2 := createWS(t, r, alice, `{"name":"Side project","modules":["board","calendar","board"],"starter":false}`)
	if strings.Join(ws2.Modules, ",") != "board,calendar" {
		t.Errorf("modules: %v", ws2.Modules)
	}
	if w := authdo(r, "POST", "/api/v1/workspaces", `{"name":"x","modules":["teleporter"]}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown module: %d", w.Code)
	}
}

// TestWorkspaceIsPrivate: a non-member — even the instance admin — can't see
// a workspace or its content (404, not 403), and it isn't in their list.
func TestWorkspaceIsPrivate(t *testing.T) {
	r, admin, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Diary","kind":"personal"}`)
	for _, tok := range []string{admin, bob} {
		if w := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID, "", tok); w.Code != http.StatusNotFound {
			t.Errorf("non-member GET: %d", w.Code)
		}
		if w := authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", tok); w.Code != http.StatusNotFound {
			t.Errorf("non-member pages: %d", w.Code)
		}
		var mine []wsResp
		decode(t, authdo(r, "GET", "/api/v1/workspaces", "", tok), &mine)
		if len(mine) != 0 {
			t.Errorf("non-member sees %d workspaces", len(mine))
		}
	}
	var pages []pageSummary
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &pages)
	if w := authdo(r, "GET", "/api/v1/pages/"+pages[0].ID, "", bob); w.Code != http.StatusNotFound {
		t.Errorf("non-member reads a page by id: %d", w.Code)
	}
	if w := authdo(r, "GET", "/api/v1/search?q=welcome", "", bob); !strings.HasPrefix(w.Body.String(), "[]") {
		t.Errorf("search leaks another user's pages: %s", w.Body.String())
	}
}

// TestMembership: owners invite by username or email; viewers read but can't
// write; the last owner can't leave; a member can leave.
func TestMembership(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Home","kind":"shared"}`)
	base := "/api/v1/workspaces/" + ws.ID
	if w := authdo(r, "POST", base+"/members", `{"user":"nobody","role":"editor"}`, alice); w.Code != http.StatusNotFound {
		t.Errorf("unknown account: %d", w.Code)
	}
	expect(t, authdo(r, "POST", base+"/members", `{"user":"BOB","role":"viewer"}`, alice), http.StatusCreated, "add bob")
	if w := authdo(r, "POST", base+"/members", `{"user":"bob","role":"viewer"}`, alice); w.Code != http.StatusConflict {
		t.Errorf("added twice: %d", w.Code)
	}
	// Bob, a viewer: reads, can't write or manage.
	expect(t, authdo(r, "GET", base+"/tasks", "", bob), 200, "viewer reads tasks")
	if w := authdo(r, "POST", base+"/tasks", `{"title":"x"}`, bob); w.Code != http.StatusForbidden {
		t.Errorf("viewer writes: %d", w.Code)
	}
	if w := authdo(r, "POST", base+"/members", `{"user":"admin","role":"owner"}`, bob); w.Code != http.StatusForbidden {
		t.Errorf("viewer manages members: %d", w.Code)
	}
	// Alice makes Bob an editor: he can write now.
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	decode(t, authdo(r, "GET", "/api/v1/auth/me", "", bob), &me)
	expect(t, authdo(r, "PUT", base+"/members/"+me.User.ID, `{"role":"editor"}`, alice), 200, "make bob editor")
	expect(t, authdo(r, "POST", base+"/tasks", `{"title":"Milk"}`, bob), http.StatusCreated, "editor writes")
	// Alice is the last owner.
	decode(t, authdo(r, "GET", "/api/v1/auth/me", "", alice), &me)
	aliceID := me.User.ID
	if w := authdo(r, "DELETE", base+"/members/"+aliceID, "", alice); w.Code != http.StatusConflict {
		t.Errorf("last owner leaves: %d", w.Code)
	}
	if w := authdo(r, "PUT", base+"/members/"+aliceID, `{"role":"editor"}`, alice); w.Code != http.StatusConflict {
		t.Errorf("last owner demoted: %d", w.Code)
	}
	// Bob leaves.
	decode(t, authdo(r, "GET", "/api/v1/auth/me", "", bob), &me)
	expect(t, authdo(r, "DELETE", base+"/members/"+me.User.ID, "", bob), http.StatusNoContent, "bob leaves")
	if w := authdo(r, "GET", base, "", bob); w.Code != http.StatusNotFound {
		t.Errorf("after leaving: %d", w.Code)
	}
}

// TestArchivedWorkspaceIsReadOnly: archiving freezes content until restored.
func TestArchivedWorkspaceIsReadOnly(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"2025","kind":"school"}`)
	base := "/api/v1/workspaces/" + ws.ID
	expect(t, authdo(r, "PUT", base, `{"archived":true}`, alice), 200, "archive")
	if w := authdo(r, "POST", base+"/tasks", `{"title":"x"}`, alice); w.Code != http.StatusConflict {
		t.Errorf("write in archived: %d", w.Code)
	}
	expect(t, authdo(r, "PUT", base, `{"archived":false}`, alice), 200, "restore")
	expect(t, authdo(r, "POST", base+"/tasks", `{"title":"x"}`, alice), http.StatusCreated, "write after restore")
}

// TestPageVersionsAndTree: a stale save gets 409; a page can't move under
// itself; trash and restore take sub-pages along.
func TestPageVersionsAndTree(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Notes","starter":false}`)
	var parent, child pageResponse
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Parent"}`, alice), &parent)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/pages", `{"title":"Child","parent_id":"`+parent.ID+`"}`, alice), &child)

	// Pages are Markdown.
	doc := "Hello **orbit**"
	body := func(v int) string {
		b, _ := json.Marshal(map[string]any{"content": doc, "version": v})
		return string(b)
	}
	var saved pageResponse
	w := authdo(r, "PUT", "/api/v1/pages/"+parent.ID, body(1), alice)
	expect(t, w, 200, "first save")
	decode(t, w, &saved)
	if saved.Version != 2 {
		t.Errorf("version after save: %d", saved.Version)
	}
	if w := authdo(r, "PUT", "/api/v1/pages/"+parent.ID, body(1), alice); w.Code != http.StatusConflict {
		t.Errorf("stale save: %d", w.Code)
	}
	// An older client's editor JSON is stored as the same Markdown.
	old, _ := json.Marshal(map[string]any{"version": 2, "content": `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"text","marks":[{"type":"bold"}],"text":"orbit"}]}]}`})
	w = authdo(r, "PUT", "/api/v1/pages/"+parent.ID, string(old), alice)
	expect(t, w, 200, "JSON save")
	decode(t, w, &saved)
	if saved.Content != "Hello **orbit**" {
		t.Errorf("JSON stored as %q", saved.Content)
	}
	var hits []searchHit
	decode(t, authdo(r, "GET", "/api/v1/search?q=ORBIT", "", alice), &hits)
	if len(hits) != 1 || hits[0].ID != parent.ID || !strings.Contains(hits[0].Snippet, "Hello orbit") {
		t.Errorf("search: %+v", hits)
	}

	if w := authdo(r, "POST", "/api/v1/pages/"+parent.ID+"/move", `{"parent_id":"`+child.ID+`"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("move under own child: %d", w.Code)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/pages/"+parent.ID, "", alice), http.StatusNoContent, "trash")
	var live, trash []pageSummary
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &live)
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/trash", "", alice), &trash)
	if len(live) != 0 || len(trash) != 2 {
		t.Fatalf("after trash: %d live, %d trashed", len(live), len(trash))
	}
	expect(t, authdo(r, "POST", "/api/v1/pages/"+parent.ID+"/restore", "", alice), 200, "restore")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/pages", "", alice), &live)
	if len(live) != 2 {
		t.Errorf("restore brings the child back: %d", len(live))
	}
	expect(t, authdo(r, "POST", "/api/v1/pages/"+child.ID+"/favorite", "", alice), http.StatusNoContent, "favourite")
	var favs []pageSummary
	decode(t, authdo(r, "GET", "/api/v1/me/favorites", "", alice), &favs)
	if len(favs) != 1 || favs[0].ID != child.ID {
		t.Errorf("favourites: %+v", favs)
	}
}

type taskOut struct {
	ID       string  `json:"id"`
	ListID   string  `json:"list_id"`
	ColumnID string  `json:"column_id"`
	Done     bool    `json:"done"`
	Due      *string `json:"due"`
	Rank     float64 `json:"rank"`
}

// TestBoardAndRepeats: a done column completes a task; completing a
// repeating task moves its date on instead; removing a column re-homes its
// tasks; a read-only API token can read but not write.
func TestBoardAndRepeats(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	var lists []listResponse
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/lists", "", alice), &lists)
	board := lists[0] // Backlog, In progress, Review, Done
	cols := board.Columns

	var a, b taskOut
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"A"}`, alice), &a)
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"B"}`, alice), &b)
	if a.ColumnID != cols[0].ID || b.Rank <= a.Rank {
		t.Fatalf("new tasks go to the end of the first column: %+v %+v", a, b)
	}
	// Drag B to the top of Done: completed.
	var moved taskOut
	decode(t, authdo(r, "POST", "/api/v1/tasks/"+b.ID+"/move", `{"column_id":"`+cols[3].ID+`","after_id":"^"}`, alice), &moved)
	if !moved.Done {
		t.Error("dropping in a done column completes")
	}
	// Back to In progress: open again.
	decode(t, authdo(r, "POST", "/api/v1/tasks/"+b.ID+"/move", `{"column_id":"`+cols[1].ID+`"}`, alice), &moved)
	if moved.Done {
		t.Error("leaving the done column reopens")
	}

	// A weekly task: completing it moves the date on.
	var rep taskOut
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"Report","due":"2026-10-13","repeat":"FREQ=WEEKLY"}`, alice), &rep)
	var upd struct {
		Task     taskOut `json:"task"`
		Repeated bool    `json:"repeated"`
	}
	decode(t, authdo(r, "PUT", "/api/v1/tasks/"+rep.ID, `{"completed":true}`, alice), &upd)
	if !upd.Repeated || upd.Task.Done || *upd.Task.Due != "2026-10-20" {
		t.Errorf("repeat: %+v", upd)
	}
	if w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"x","repeat":"FREQ=HOURLY"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("hourly repeat accepted: %d", w.Code)
	}

	// Remove "In progress": B goes back to the first open column.
	newCols, _ := json.Marshal([]any{cols[0], cols[2], cols[3]})
	expect(t, authdo(r, "PUT", "/api/v1/lists/"+board.ID, `{"columns":`+string(newCols)+`}`, alice), 200, "remove a column")
	var tasks []taskOut
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/tasks", "", alice), &tasks)
	for _, tk := range tasks {
		if tk.ID == b.ID && tk.ColumnID != cols[0].ID {
			t.Errorf("task of a removed column: %+v", tk)
		}
	}

	// A read-only API token.
	var tokResp struct {
		Token string `json:"token"`
	}
	decode(t, authdo(r, "POST", "/api/v1/me/api-tokens", `{"name":"ro","read_only":true}`, alice), &tokResp)
	expect(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/tasks", "", tokResp.Token), 200, "read-only token reads")
	if w := authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", `{"title":"x"}`, tokResp.Token); w.Code != http.StatusForbidden {
		t.Errorf("read-only token writes: %d", w.Code)
	}
}

// TestCalendar: a weekly event expands over a range with due tasks; one
// occurrence can be removed; the private feed serves events and tasks and
// stops when revoked.
func TestCalendar(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"School","kind":"school"}`)
	base := "/api/v1/workspaces/" + ws.ID
	var ev eventResponse
	w := authdo(r, "POST", base+"/events", `{"title":"Maths","starts_at":"2026-10-05T08:00:00Z","ends_at":"2026-10-05T09:30:00Z","tz":"Europe/Lisbon","rrule":"FREQ=WEEKLY;BYDAY=MO"}`, alice)
	expect(t, w, http.StatusCreated, "create event")
	decode(t, w, &ev)
	expect(t, authdo(r, "POST", base+"/tasks", `{"title":"Essay","due":"2026-10-14"}`, alice), http.StatusCreated, "task due")
	if w := authdo(r, "POST", base+"/events", `{"title":"x","starts_at":"2026-10-05T10:00:00Z","ends_at":"2026-10-05T09:00:00Z"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("ends before it starts: %d", w.Code)
	}

	rng := "?from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z"
	var cal calendarResponse
	decode(t, authdo(r, "GET", base+"/calendar"+rng, "", alice), &cal)
	if len(cal.Events) != 4 || len(cal.Tasks) != 1 { // Mondays 5, 12, 19, 26 Oct
		t.Fatalf("calendar: %d events, %d tasks", len(cal.Events), len(cal.Tasks))
	}
	// After the clocks change (25 Oct) it is still 09:00 in Lisbon: 09:00Z.
	if got := cal.Events[3].Start.UTC().Format("15:04"); got != "09:00" {
		t.Errorf("after DST the class starts at %s UTC", got)
	}
	if cal.Events[0].TZ != "Europe/Lisbon" {
		t.Errorf("an occurrence carries its event's zone: %q", cal.Events[0].TZ)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/events/"+ev.ID+"?occurrence=2026-10-12T08:00:00Z", "", alice), http.StatusNoContent, "remove one")
	decode(t, authdo(r, "GET", "/api/v1/calendar"+rng, "", alice), &cal)
	if len(cal.Events) != 3 {
		t.Errorf("after removing one: %d", len(cal.Events))
	}
	if w := authdo(r, "GET", "/api/v1/calendar?from=2026-01-01T00:00:00Z&to=2026-12-01T00:00:00Z", "", alice); w.Code != http.StatusBadRequest {
		t.Errorf("a range of 11 months: %d", w.Code)
	}
	decode(t, authdo(r, "GET", "/api/v1/calendar"+rng, "", bob), &cal)
	if len(cal.Events) != 0 {
		t.Error("someone else's calendar is empty")
	}

	// The feed.
	var feed struct {
		URL string `json:"url"`
	}
	decode(t, authdo(r, "POST", "/api/v1/me/feeds", `{"name":"Orbit"}`, alice), &feed)
	path := feed.URL[strings.Index(feed.URL, "/ical/"):]
	w = do(r, "GET", path, "")
	expect(t, w, 200, "feed")
	ics := w.Body.String()
	if !strings.Contains(w.Header().Get("Content-Type"), "text/calendar") || !strings.Contains(ics, "SUMMARY:Maths") ||
		!strings.Contains(ics, "RRULE:FREQ=WEEKLY;BYDAY=MO") || !strings.Contains(ics, "EXDATE;TZID=Europe/Lisbon:20261012T090000") || !strings.Contains(ics, "☐ Essay") {
		t.Errorf("feed content:\n%s", ics)
	}
	if w := do(r, "GET", "/ical/ocf_nope.ics", ""); w.Code != http.StatusNotFound {
		t.Errorf("unknown feed: %d", w.Code)
	}
	var feeds []feedResponse
	decode(t, authdo(r, "GET", "/api/v1/me/feeds", "", alice), &feeds)
	expect(t, authdo(r, "DELETE", "/api/v1/me/feeds/"+feeds[0].ID, "", alice), http.StatusNoContent, "revoke")
	if w := do(r, "GET", path, ""); w.Code != http.StatusNotFound {
		t.Errorf("revoked feed: %d", w.Code)
	}
}

// TestSubscription: an ICS calendar is fetched at once (a bad one is refused),
// its events appear in the range, and the address never comes back.
func TestSubscription(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:t\r\nBEGIN:VEVENT\r\nUID:h1\r\nDTSTAMP:20261001T000000Z\r\n" +
		"SUMMARY:Holiday\r\nDTSTART;VALUE=DATE:20261015\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/cal.ics" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/calendar")
		_, _ = w.Write([]byte(ics))
	}))
	defer srv.Close()

	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"alice","password":"alicepassword"}`)
	alice := loginTok(t, r, "alice", "alicepassword")
	ws := createWS(t, r, alice, `{"name":"S","kind":"school"}`)
	base := "/api/v1/workspaces/" + ws.ID

	// The test server is on loopback, which the guarded dialer refuses: the
	// refusal is the first thing to check.
	if w := authdo(r, "POST", base+"/subscriptions", `{"url":"`+srv.URL+`/cal.ics"}`, alice); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "reach") {
		t.Fatalf("loopback address: %d %s", w.Code, w.Body.String())
	}
	if w := authdo(r, "POST", base+"/subscriptions", `{"url":"ftp://x/cal.ics"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("ftp address: %d", w.Code)
	}
	// The same instance with a syncer that may reach the test server.
	r = routerWithOpenClient(t, db)
	w := authdo(r, "POST", base+"/subscriptions", `{"name":"Holidays","url":"`+srv.URL+`/cal.ics"}`, alice)
	expect(t, w, http.StatusCreated, "subscribe")
	if strings.Contains(w.Body.String(), "cal.ics") {
		t.Error("the address is echoed back")
	}
	var cal calendarResponse
	decode(t, authdo(r, "GET", base+"/calendar?from=2026-10-01T00:00:00Z&to=2026-11-01T00:00:00Z", "", alice), &cal)
	if len(cal.Events) != 1 || cal.Events[0].Kind != "external" || !cal.Events[0].AllDay {
		t.Errorf("subscribed events: %+v", cal.Events)
	}
	if w := authdo(r, "POST", base+"/subscriptions", `{"url":"`+srv.URL+`/missing.ics"}`, alice); w.Code != http.StatusBadRequest {
		t.Errorf("404 calendar: %d", w.Code)
	}
}

// TestDeleteUserReleasesWorkspaces: deleting an account deletes the
// workspaces it was alone in and hands shared ones to another member.
func TestDeleteUserReleasesWorkspaces(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	aliceID := createUserVia(t, r, admin, `{"username":"alice","password":"alicepassword"}`)
	createUserVia(t, r, admin, `{"username":"bob","password":"bobpassword"}`)
	alice := loginTok(t, r, "alice", "alicepassword")
	bob := loginTok(t, r, "bob", "bobpassword")
	solo := createWS(t, r, alice, `{"name":"Mine","kind":"personal"}`)
	shared := createWS(t, r, alice, `{"name":"Ours","kind":"shared"}`)
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+shared.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "add bob")

	expect(t, authdo(r, "DELETE", "/api/v1/users/"+aliceID, "", admin), http.StatusNoContent, "delete alice")
	var n int64
	db.Model(&models.Workspace{}).Where("id = ?", solo.ID).Count(&n)
	if n != 0 {
		t.Error("the solo workspace remains")
	}
	db.Model(&models.Page{}).Where("workspace_id = ?", solo.ID).Count(&n)
	if n != 0 {
		t.Error("its pages remain")
	}
	w := authdo(r, "GET", "/api/v1/workspaces/"+shared.ID, "", bob)
	expect(t, w, 200, "bob still has the shared one")
	var detail struct {
		Workspace wsResp `json:"workspace"`
	}
	decode(t, w, &detail)
	if detail.Workspace.Role != "owner" {
		t.Errorf("bob should own it now: %+v", detail.Workspace)
	}
}

// TestHome: overdue, today and upcoming tasks for the viewer's local date.
func TestHome(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"P","kind":"personal"}`)
	for _, b := range []string{
		`{"title":"late","due":"2026-10-12"}`, `{"title":"now","due":"2026-10-13","due_time":"09:00"}`,
		`{"title":"soon","due":"2026-10-16"}`, `{"title":"later","due":"2026-12-01"}`, `{"title":"undated"}`,
	} {
		expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/tasks", b, alice), http.StatusCreated, "task")
	}
	var home homeResponse
	w := authdo(r, "GET", "/api/v1/home?today=2026-10-13&from=2026-10-12T23:00:00Z&to=2026-10-20T23:00:00Z", "", alice)
	expect(t, w, 200, "home")
	decode(t, w, &home)
	if len(home.Overdue) != 1 || len(home.Today) != 1 || len(home.Upcoming) != 1 || len(home.Pages) == 0 {
		t.Errorf("home: %d overdue, %d today, %d upcoming, %d pages", len(home.Overdue), len(home.Today), len(home.Upcoming), len(home.Pages))
	}
}

// TestReadOnlyCatalog: the catalog lists modules (some marked soon) and templates.
func TestReadOnlyCatalog(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	var cat struct {
		Modules []struct {
			Key  string `json:"key"`
			Soon bool   `json:"soon"`
		} `json:"modules"`
		Templates []struct {
			Kind string `json:"kind"`
		} `json:"templates"`
	}
	decode(t, authdo(r, "GET", "/api/v1/catalog", "", alice), &cat)
	if len(cat.Templates) != 5 || len(cat.Modules) < 5 || cat.Modules[0].Soon {
		t.Errorf("catalog: %+v", cat)
	}
}

// TestSeriesScopes: one occurrence changed alone, then the series changed
// from one occurrence on; then deleted from one on.
func TestSeriesScopes(t *testing.T) {
	r, _, alice, _ := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID
	var ev eventResponse
	decode(t, authdo(r, "POST", base+"/events", `{"title":"Standup","starts_at":"2026-11-02T09:00:00Z","ends_at":"2026-11-02T09:15:00Z","tz":"UTC","rrule":"FREQ=WEEKLY;BYDAY=MO;COUNT=8"}`, alice), &ev)
	id := "/api/v1/events/" + ev.ID
	rng := "?from=2026-11-01T00:00:00Z&to=2027-01-31T00:00:00Z"
	titles := func() []string {
		var cal calendarResponse
		decode(t, authdo(r, "GET", base+"/calendar"+rng, "", alice), &cal)
		out := []string{}
		for _, e := range cal.Events {
			out = append(out, e.Start.UTC().Format("01-02 15:04")+" "+e.Title)
		}
		return out
	}
	expect(t, authdo(r, "PUT", id+"?scope=this&occurrence=2026-11-04T09:00:00Z", `{"title":"x"}`, alice), http.StatusBadRequest, "not an occurrence")

	// Only the 9 Nov one: later, renamed, on its own (not repeating).
	var one eventResponse
	w := authdo(r, "PUT", id+"?scope=this&occurrence=2026-11-09T09:00:00Z", `{"title":"Standup (late)","starts_at":"2026-11-09T11:00:00Z","ends_at":"2026-11-09T11:15:00Z","rrule":"FREQ=DAILY"}`, alice)
	expect(t, w, http.StatusOK, "this one")
	decode(t, w, &one)
	if one.ID == ev.ID || one.RRule != "" {
		t.Fatalf("this one: %+v", one.Event)
	}
	got := titles()
	if len(got) != 8 || got[1] != "11-09 11:00 Standup (late)" {
		t.Fatalf("after this one: %v", got)
	}

	// From 23 Nov on: at 10:00, renamed; the 5 left keep counting.
	w = authdo(r, "PUT", id+"?scope=following&occurrence=2026-11-23T09:00:00Z", `{"title":"Sync","starts_at":"2026-11-23T10:00:00Z","ends_at":"2026-11-23T10:30:00Z"}`, alice)
	expect(t, w, http.StatusOK, "following")
	got = titles()
	want := []string{"11-02 09:00 Standup", "11-09 11:00 Standup (late)", "11-16 09:00 Standup", "11-23 10:00 Sync", "11-30 10:00 Sync", "12-07 10:00 Sync", "12-14 10:00 Sync", "12-21 10:00 Sync"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("after following:\n%v\nwant\n%v", got, want)
	}

	// Deleting the new series from 7 Dec on.
	var cal calendarResponse
	decode(t, authdo(r, "GET", base+"/calendar"+rng, "", alice), &cal)
	sync := cal.Events[3].ID
	expect(t, authdo(r, "DELETE", "/api/v1/events/"+sync+"?scope=following&occurrence=2026-12-07T10:00:00Z", "", alice), http.StatusNoContent, "delete following")
	if got := titles(); len(got) != 5 || got[4] != "11-30 10:00 Sync" {
		t.Errorf("after delete following: %v", got)
	}
	// From the first: the whole series goes.
	expect(t, authdo(r, "DELETE", id+"?scope=following&occurrence=2026-11-02T09:00:00Z", "", alice), http.StatusNoContent, "delete all from first")
	if got := titles(); len(got) != 3 {
		t.Errorf("after deleting the first series: %v", got)
	}
}

package caldav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	dav "github.com/emersion/go-webdav/caldav"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// memCal is a CalDAV server's account in memory: one user, two calendars.
type memCal struct {
	mu   sync.Mutex
	objs map[string][]byte // path → iCalendar
}

const (
	principal = "/dav/alice/"
	home      = "/dav/alice/calendars/"
	work      = "/dav/alice/calendars/work/"
)

func etagOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func (m *memCal) CurrentUserPrincipal(context.Context) (string, error) { return principal, nil }
func (m *memCal) CalendarHomeSetPath(context.Context) (string, error)  { return home, nil }
func (m *memCal) CreateCalendar(context.Context, *dav.Calendar) error  { return nil }
func (m *memCal) ListCalendars(context.Context) ([]dav.Calendar, error) {
	return []dav.Calendar{
		{Path: work, Name: "Work", SupportedComponentSet: []string{"VEVENT"}},
		{Path: home + "todo/", Name: "Reminders", SupportedComponentSet: []string{"VTODO"}},
	}, nil
}
func (m *memCal) GetCalendar(_ context.Context, p string) (*dav.Calendar, error) {
	cals, _ := m.ListCalendars(context.Background())
	for _, c := range cals {
		if c.Path == p {
			return &c, nil
		}
	}
	return nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("no calendar"))
}

func (m *memCal) object(p string) (*dav.CalendarObject, error) {
	data, ok := m.objs[p]
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("not found"))
	}
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	return &dav.CalendarObject{Path: p, ETag: etagOf(data), Data: cal, ContentLength: int64(len(data))}, nil
}

func (m *memCal) GetCalendarObject(_ context.Context, p string, _ *dav.CalendarCompRequest) (*dav.CalendarObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.object(p)
}

func (m *memCal) ListCalendarObjects(_ context.Context, p string, _ *dav.CalendarCompRequest) ([]dav.CalendarObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []dav.CalendarObject
	for k := range m.objs {
		if strings.HasPrefix(k, p) {
			o, err := m.object(k)
			if err != nil {
				return nil, err
			}
			out = append(out, *o)
		}
	}
	return out, nil
}

func (m *memCal) QueryCalendarObjects(ctx context.Context, p string, q *dav.CalendarQuery) ([]dav.CalendarObject, error) {
	all, err := m.ListCalendarObjects(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	return dav.Filter(q, all)
}

func (m *memCal) PutCalendarObject(_ context.Context, p string, cal *ical.Calendar, opts *dav.PutCalendarObjectOptions) (*dav.CalendarObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, exists := m.objs[p]
	if opts.IfNoneMatch.IsWildcard() && exists {
		return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, errors.New("exists"))
	}
	if opts.IfMatch.IsSet() {
		if ok, _ := opts.IfMatch.MatchETag(etagOf(cur)); !exists || !ok {
			return nil, webdav.NewHTTPError(http.StatusPreconditionFailed, errors.New("changed"))
		}
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	m.objs[p] = buf.Bytes()
	return m.object(p)
}

func (m *memCal) DeleteCalendarObject(_ context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objs, p)
	return nil
}

// put / get / del change the server behind Orbit's back.
func (m *memCal) put(p, data string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objs[p] = []byte(strings.ReplaceAll(data, "\n", "\r\n"))
}

func (m *memCal) get(p string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.objs[p])
}

func (m *memCal) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objs)
}

func vevent(uid, summary, start string) string {
	return "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VEVENT\nUID:" + uid + "\nDTSTAMP:20261001T080000Z\n" +
		"DTSTART:" + start + "\nDTEND:" + start[:9] + "235900Z\nSUMMARY:" + summary + "\nEND:VEVENT\nEND:VCALENDAR\n"
}

type fixture struct {
	db   *gorm.DB
	mem  *memCal
	srv  *httptest.Server
	s    *Syncer
	ws   string
	link models.CalDAVLink
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatal(err)
	}
	sealer, _ := keystore.New("caldav-test-master-key-0123456789abcdef", []byte("0123456789abcdef"))
	mem := &memCal{objs: map[string][]byte{}}
	h := &dav.Handler{Backend: mem, Prefix: "/dav"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "alice" || p != "app-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/.well-known/caldav" {
			http.Redirect(w, r, principal, http.StatusMovedPermanently)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	s := New(db, sealer, zap.NewNop()).UseClient(HTTPClient(nil))
	ws := models.Workspace{Name: "Work", Kind: "work"}
	db.Create(&ws)
	sealed, _ := s.Seal("app-password")
	link := models.CalDAVLink{WorkspaceID: ws.ID, CalendarURL: srv.URL + work, CalendarName: "Work", Username: "alice", PasswordSealed: sealed}
	db.Create(&link)
	return &fixture{db: db, mem: mem, srv: srv, s: s, ws: ws.ID, link: link}
}

func (f *fixture) sync(t *testing.T) {
	t.Helper()
	if err := f.s.Sync(context.Background(), f.link.ID); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (f *fixture) events() map[string]models.Event {
	var evs []models.Event
	f.db.Where("workspace_id = ?", f.ws).Find(&evs)
	out := map[string]models.Event{}
	for _, e := range evs {
		out[e.Title] = e
	}
	return out
}

func TestCalendarsDiscovery(t *testing.T) {
	f := setup(t)
	cl := f.s.Account("alice", "app-password")
	// From the server's address: the well-known redirect (a PROPFIND kept a
	// PROPFIND), the principal, its home, and the calendars that hold events.
	cals, err := cl.Calendars(context.Background(), f.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(cals) != 1 || cals[0].Name != "Work" || cals[0].URL != f.srv.URL+work {
		t.Fatalf("calendars: %+v", cals)
	}
	if _, err := f.s.Account("alice", "wrong").Calendars(context.Background(), f.srv.URL); !errors.Is(err, ErrAuth) {
		t.Errorf("a wrong password: %v", err)
	}
}

// TestSyncBothWays: what's on either side reaches the other — new, changed
// and deleted — and what isn't an event is left alone.
func TestSyncBothWays(t *testing.T) {
	f := setup(t)
	f.mem.put(work+"dentist.ics", vevent("d1", "Dentist", "20261105T090000Z"))
	f.mem.put(work+"todo.ics", "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//EN\nBEGIN:VTODO\nUID:t1\nDTSTAMP:20261001T080000Z\nSUMMARY:Buy milk\nEND:VTODO\nEND:VCALENDAR\n")
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	standup := models.Event{WorkspaceID: f.ws, Title: "Standup", StartsAt: start, EndsAt: start.Add(15 * time.Minute), TZ: "Europe/Lisbon"}
	f.db.Create(&standup)
	class := models.Event{WorkspaceID: f.ws, Title: "Maths class", StartsAt: start, EndsAt: start.Add(time.Hour), CourseID: "c1"}
	f.db.Create(&class)

	f.sync(t)
	evs := f.events()
	if _, ok := evs["Dentist"]; !ok {
		t.Fatalf("the server's event didn't come: %v", evs)
	}
	up := f.mem.get(work + standup.ID + ".ics")
	if !strings.Contains(up, "SUMMARY:Standup") || !strings.Contains(up, "DTSTART;TZID=Europe/Lisbon:20261106T100000") {
		t.Fatalf("Orbit's event didn't go up:\n%s", up)
	}
	if f.mem.count() != 3 {
		t.Errorf("the course class went up too: %d objects", f.mem.count())
	}
	var link models.CalDAVLink
	f.db.First(&link, "id = ?", f.link.ID)
	if link.LastError != "" || link.EventCount != 2 || link.SyncedAt == nil {
		t.Errorf("link after sync: %+v", link)
	}

	// Nothing changed: nothing moves.
	before := f.mem.get(work + standup.ID + ".ics")
	f.sync(t)
	if f.mem.get(work+standup.ID+".ics") != before || len(f.events()) != 3 {
		t.Error("a quiet sync changed something")
	}

	// Renamed in Orbit: the object follows, the rest of it kept.
	time.Sleep(2 * time.Millisecond)
	f.db.Model(&standup).Update("title", "Daily standup")
	f.sync(t)
	if up := f.mem.get(work + standup.ID + ".ics"); !strings.Contains(up, "SUMMARY:Daily standup") {
		t.Fatalf("the rename didn't go up:\n%s", up)
	}

	// Moved on the server: Orbit follows.
	f.mem.put(work+"dentist.ics", vevent("d1", "Dentist (moved)", "20261112T090000Z"))
	f.sync(t)
	d, ok := f.events()["Dentist (moved)"]
	if !ok || !d.StartsAt.Equal(time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("the server's change didn't come: %+v", f.events())
	}

	// Deleted in Orbit: gone from the server.
	f.db.Delete(&standup)
	f.sync(t)
	if f.mem.get(work+standup.ID+".ics") != "" {
		t.Error("deleted in Orbit, still on the server")
	}

	// Deleted on the server: gone from Orbit.
	f.mem.mu.Lock()
	delete(f.mem.objs, work+"dentist.ics")
	f.mem.mu.Unlock()
	f.sync(t)
	if _, ok := f.events()["Dentist (moved)"]; ok {
		t.Error("deleted on the server, still in Orbit")
	}
	// The to-do is still there, untouched.
	if !strings.Contains(f.mem.get(work+"todo.ics"), "Buy milk") {
		t.Error("the to-do was touched")
	}
}

// TestSyncConflict: changed on both sides, the later change wins.
func TestSyncConflict(t *testing.T) {
	f := setup(t)
	f.mem.put(work+"a.ics", vevent("a1", "Review", "20261105T090000Z"))
	f.sync(t)
	ev := f.events()["Review"]
	// The server's change is older than Orbit's (LAST-MODIFIED in the past).
	f.mem.put(work+"a.ics", strings.Replace(vevent("a1", "Review (server)", "20261105T090000Z"), "DTSTAMP", "LAST-MODIFIED:20200101T000000Z\nDTSTAMP", 1))
	time.Sleep(2 * time.Millisecond)
	f.db.Model(&ev).Update("title", "Review (Orbit)")
	f.sync(t)
	if _, ok := f.events()["Review (Orbit)"]; !ok {
		t.Fatalf("Orbit's later change lost: %v", f.events())
	}
	if !strings.Contains(f.mem.get(work+"a.ics"), "SUMMARY:Review (Orbit)") {
		t.Errorf("Orbit's change didn't win on the server:\n%s", f.mem.get(work+"a.ics"))
	}
}

func TestSyncWrongPassword(t *testing.T) {
	f := setup(t)
	sealed, _ := f.s.Seal("expired")
	f.db.Model(&f.link).Update("password_sealed", sealed)
	if err := f.s.Sync(context.Background(), f.link.ID); !errors.Is(err, ErrAuth) {
		t.Fatalf("sync: %v", err)
	}
	var link models.CalDAVLink
	f.db.First(&link, "id = ?", f.link.ID)
	if !strings.Contains(link.LastError, "password") {
		t.Errorf("last error: %q", link.LastError)
	}
}

package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/google"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	ical "github.com/emersion/go-ical"
	"github.com/emersion/go-webdav"
	dav "github.com/emersion/go-webdav/caldav"
)

// gCal is Google's CalDAV for one calendar, in memory.
type gCal struct {
	mu   sync.Mutex
	objs map[string][]byte
}

const gEvents = "/caldav/v2/work-cal/events/"

func gEtag(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:8]) }

// (go-webdav's server tells a path's kind by its depth under the prefix:
// /v2 the principal, /v2/<calendar> the home, …/events the calendar.)
func (m *gCal) CurrentUserPrincipal(context.Context) (string, error) { return "/caldav/v2/", nil }
func (m *gCal) CalendarHomeSetPath(context.Context) (string, error) {
	return "/caldav/v2/work-cal/", nil
}
func (m *gCal) CreateCalendar(context.Context, *dav.Calendar) error { return nil }
func (m *gCal) ListCalendars(context.Context) ([]dav.Calendar, error) {
	return []dav.Calendar{{Path: gEvents, Name: "Work", SupportedComponentSet: []string{"VEVENT"}}}, nil
}
func (m *gCal) GetCalendar(_ context.Context, p string) (*dav.Calendar, error) {
	if p == gEvents {
		return &dav.Calendar{Path: gEvents, Name: "Work", SupportedComponentSet: []string{"VEVENT"}}, nil
	}
	return nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("no calendar"))
}
func (m *gCal) object(p string) (*dav.CalendarObject, error) {
	data, ok := m.objs[p]
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, errors.New("not found"))
	}
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	return &dav.CalendarObject{Path: p, ETag: gEtag(data), Data: cal, ContentLength: int64(len(data))}, nil
}
func (m *gCal) GetCalendarObject(_ context.Context, p string, _ *dav.CalendarCompRequest) (*dav.CalendarObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.object(p)
}
func (m *gCal) ListCalendarObjects(_ context.Context, p string, _ *dav.CalendarCompRequest) ([]dav.CalendarObject, error) {
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
func (m *gCal) QueryCalendarObjects(ctx context.Context, p string, q *dav.CalendarQuery) ([]dav.CalendarObject, error) {
	all, err := m.ListCalendarObjects(ctx, p, nil)
	if err != nil {
		return nil, err
	}
	return dav.Filter(q, all)
}
func (m *gCal) PutCalendarObject(_ context.Context, p string, cal *ical.Calendar, _ *dav.PutCalendarObjectOptions) (*dav.CalendarObject, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	m.objs[p] = buf.Bytes()
	return m.object(p)
}
func (m *gCal) DeleteCalendarObject(_ context.Context, p string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objs, p)
	return nil
}

// TestGoogleCalendar: the owner registers the OAuth client; a workspace
// owner signs in with Google (state and its cookie checked), picks a
// calendar, and the two calendars meet — over CalDAV, with access tokens
// (refreshed when they run out) instead of a password.
func TestGoogleCalendar(t *testing.T) {
	mem := &gCal{objs: map[string][]byte{
		gEvents + "dentist.ics": []byte(strings.ReplaceAll("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Google//EN\nBEGIN:VEVENT\nUID:d1\nDTSTAMP:20261001T080000Z\nDTSTART:20261105T090000Z\nDTEND:20261105T100000Z\nSUMMARY:Dentist\nEND:VEVENT\nEND:VCALENDAR\n", "\n", "\r\n")),
	}}
	var refreshes atomic.Int32
	var bearer atomic.Value
	davH := &dav.Handler{Backend: mem, Prefix: "/caldav"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = r.ParseForm()
			w.Header().Set("Content-Type", "application/json")
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				if r.Form.Get("code") != "good-code" || r.Form.Get("client_secret") != "shh" || r.Form.Get("code_verifier") == "" {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
					return
				}
				// Short-lived: the first sync has to refresh it.
				_, _ = w.Write([]byte(`{"access_token":"acc-first","refresh_token":"ref-1","expires_in":30}`))
			case "refresh_token":
				if r.Form.Get("refresh_token") != "ref-1" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				refreshes.Add(1)
				_, _ = w.Write([]byte(`{"access_token":"acc-fresh","expires_in":3600}`))
			}
		case r.URL.Path == "/userinfo":
			_, _ = w.Write([]byte(`{"email":"ana@example.com"}`))
		case r.URL.Path == "/calendarList":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer acc-") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"team@group.calendar.google.com","summary":"Team","accessRole":"writer"},{"id":"work-cal","summary":"Work","accessRole":"owner","primary":true,"backgroundColor":"#9fe1e7"}]}`))
		case strings.HasPrefix(r.URL.Path, "/caldav/v2/"):
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer acc-") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			bearer.Store(auth)
			davH.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := []string{google.AuthURL, google.TokenURL, google.UserInfoURL, google.CalendarListURL, google.CalDAVBase}
	oldHTTP := google.HTTP
	t.Cleanup(func() {
		google.AuthURL, google.TokenURL, google.UserInfoURL, google.CalendarListURL, google.CalDAVBase = old[0], old[1], old[2], old[3], old[4]
		google.HTTP = oldHTTP
		testDAVClient = nil
	})
	google.AuthURL, google.TokenURL, google.UserInfoURL, google.CalendarListURL, google.CalDAVBase = srv.URL+"/auth", srv.URL+"/token", srv.URL+"/userinfo", srv.URL+"/calendarList", srv.URL+"/caldav/v2/"
	google.HTTP = func() *http.Client { return srv.Client() }
	testDAVClient = &http.Client{}

	r, db := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	ws := createWS(t, r, owner, `{"name":"Team","kind":"work","starter":false}`)
	base := "/api/v1/workspaces/" + ws.ID

	var st struct {
		Available bool `json:"available"`
	}
	decode(t, authdo(r, "GET", "/api/v1/google/status", "", owner), &st)
	if st.Available {
		t.Fatal("available before it's set up")
	}
	expect(t, authdo(r, "POST", base+"/google/start", "", owner), http.StatusNotFound, "not set up")
	expect(t, authdo(r, "PUT", "/api/v1/google/app", `{"client_id":"cid.apps.googleusercontent.com"}`, owner), http.StatusBadRequest, "no secret")
	var app googleAppResponse
	decode(t, authdo(r, "PUT", "/api/v1/google/app", `{"client_id":"cid.apps.googleusercontent.com","client_secret":"shh"}`, owner), &app)
	if !app.HasSecret || !strings.HasSuffix(app.RedirectURI, "/api/v1/google/oauth/callback") {
		t.Fatalf("app: %+v", app)
	}
	decode(t, authdo(r, "GET", "/api/v1/google/status", "", owner), &st)
	if !st.Available {
		t.Fatal("not available once set up")
	}

	// Sign in: Google's page (state, PKCE, offline), then back with a code.
	w := authdo(r, "POST", base+"/google/start", "", owner)
	expect(t, w, http.StatusOK, "start")
	var start struct {
		URL string `json:"url"`
	}
	decode(t, w, &start)
	u, _ := url.Parse(start.URL)
	q := u.Query()
	if q.Get("access_type") != "offline" || q.Get("code_challenge_method") != "S256" || !strings.Contains(q.Get("scope"), "auth/calendar") {
		t.Fatalf("authorize url: %s", start.URL)
	}
	var nonce string
	for _, c := range w.Result().Cookies() {
		if c.Name == googleOAuthCookie {
			nonce = c.Value
		}
	}
	callback := func(state, code, cookie string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/v1/google/oauth/callback?state="+url.QueryEscape(state)+"&code="+code, nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: googleOAuthCookie, Value: cookie})
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	if loc := callback(q.Get("state"), "good-code", "").Header().Get("Location"); !strings.Contains(loc, "google_error=expired") {
		t.Fatalf("no cookie: %s", loc)
	}
	// That state is used up; a new sign-in.
	w = authdo(r, "POST", base+"/google/start", "", owner)
	decode(t, w, &start)
	u, _ = url.Parse(start.URL)
	for _, c := range w.Result().Cookies() {
		if c.Name == googleOAuthCookie {
			nonce = c.Value
		}
	}
	loc := callback(u.Query().Get("state"), "good-code", nonce).Header().Get("Location")
	lu, _ := url.Parse(loc)
	pending := lu.Query().Get("google")
	if lu.Path != "/w/"+ws.ID+"/calendar" || pending == "" {
		t.Fatalf("back to the calendar: %s", loc)
	}

	var cals struct {
		Email     string            `json:"email"`
		Calendars []google.Calendar `json:"calendars"`
	}
	decode(t, authdo(r, "GET", base+"/google/calendars?pending="+url.QueryEscape(pending), "", owner), &cals)
	if cals.Email != "ana@example.com" || len(cals.Calendars) != 2 || cals.Calendars[0].ID != "work-cal" {
		t.Fatalf("calendars: %+v", cals)
	}

	// An Orbit event, then the link: both sides meet.
	expect(t, authdo(r, "POST", base+"/events", `{"title":"Standup","starts_at":"2026-11-04T09:00:00Z","ends_at":"2026-11-04T09:15:00Z"}`, owner), http.StatusCreated, "event")
	w = authdo(r, "POST", base+"/google/link", `{"pending":"`+pending+`","calendar_id":"work-cal","calendar_name":"Work"}`, owner)
	expect(t, w, http.StatusOK, "link")
	var linked caldavLinkResponse
	decode(t, w, &linked)
	if linked.Link == nil || linked.Link.Provider != "google" || linked.Link.Username != "ana@example.com" || linked.Link.LastError != "" {
		t.Fatalf("link: %+v", linked.Link)
	}
	var titles []string
	db.Model(&models.Event{}).Where("workspace_id = ?", ws.ID).Order("title").Pluck("title", &titles)
	if strings.Join(titles, ",") != "Dentist,Standup" {
		var items []models.CalDAVItem
		db.Find(&items)
		for _, it := range items {
			t.Logf("item %s event=%q etag=%s", it.URL, it.EventID, it.ETag)
		}
		t.Errorf("events in Orbit: %v", titles)
	}
	mem.mu.Lock()
	n := len(mem.objs)
	mem.mu.Unlock()
	if n != 2 {
		t.Errorf("events at Google: %d", n)
	}
	// The short-lived token ran out: refreshed, and used.
	if refreshes.Load() < 1 || bearer.Load() != "Bearer acc-fresh" {
		t.Errorf("refresh: %d, %v", refreshes.Load(), bearer.Load())
	}
	// The sign-in is used up.
	expect(t, authdo(r, "POST", base+"/google/link", `{"pending":"`+pending+`","calendar_id":"work-cal"}`, owner), http.StatusGone, "used up")
}

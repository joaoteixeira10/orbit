package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/config"
	dbpkg "github.com/joaoteixeira10/orbit/api/internal/db"
	"github.com/joaoteixeira10/orbit/api/internal/google"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/rbac"

	"go.uber.org/zap"
)

// TestGooglePush: a Google link is watched (Google is given Orbit's public
// address and a secret token); Google's calls with that token sync the
// calendar now, others are refused or ignored; stopping the sync stops the
// watch.
func TestGooglePush(t *testing.T) {
	var mu sync.Mutex
	var watched map[string]any
	var stopped []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"acc-1","expires_in":3600}`))
		case r.URL.Path == "/calendar/v3/calendars/work-cal/events/watch":
			if r.Header.Get("Authorization") != "Bearer acc-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			watched = body
			mu.Unlock()
			exp := strconv.FormatInt(time.Now().Add(7*24*time.Hour).UnixMilli(), 10)
			_, _ = w.Write([]byte(`{"id":"` + body["id"].(string) + `","resourceId":"res-1","expiration":"` + exp + `"}`))
		case r.URL.Path == "/calendar/v3/channels/stop":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			stopped = append(stopped, body["id"]+"/"+body["resourceId"])
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := []string{google.TokenURL, google.CalDAVBase, google.CalendarAPI}
	oldHTTP := google.HTTP
	t.Cleanup(func() {
		google.TokenURL, google.CalDAVBase, google.CalendarAPI = old[0], old[1], old[2]
		google.HTTP = oldHTTP
		testDAVClient = nil
	})
	google.TokenURL, google.CalDAVBase, google.CalendarAPI = srv.URL+"/token", srv.URL+"/caldav/v2/", srv.URL+"/calendar/v3"
	google.HTTP = func() *http.Client { return srv.Client() }
	testDAVClient = &http.Client{}

	// An Orbit at a public https address.
	db := openTestDB(t)
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.SeedBuiltins(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := auth.Bootstrap(db, zap.NewNop(), "admin", "adminpass"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	db.Model(&models.User{}).Where("username = ?", "admin").Update("must_change_password", false)
	cfg := &config.Config{GinMode: "test", AuthRateBurst: 100, AuthRatePerMinute: 6000, PublicURL: "https://orbit.example.org/"}
	r := routerOn(t, db, cfg, nil)
	owner := loginTok(t, r, "admin", "adminpass")
	expect(t, authdo(r, "PUT", "/api/v1/google/app", `{"client_id":"cid.apps.googleusercontent.com","client_secret":"shh"}`, owner), http.StatusOK, "app")
	ws := createWS(t, r, owner, `{"name":"Team","kind":"work","starter":false}`)
	base := "/api/v1/workspaces/" + ws.ID

	sealer, err := dbpkg.OpenSealer(db, "test-master-key-0123456789-abcdef")
	if err != nil {
		t.Fatalf("sealer: %v", err)
	}
	refresh, _ := sealer.Seal([]byte("ref-1"))
	link := models.CalDAVLink{WorkspaceID: ws.ID, Provider: "google", CalendarURL: google.CalDAVURL("work-cal"), CalendarName: "Work", Host: "calendar.google.com", Username: "ana@example.com", PasswordSealed: refresh}
	if err := db.Create(&link).Error; err != nil {
		t.Fatal(err)
	}

	// Sync now also starts the watch.
	expect(t, authdo(r, "POST", base+"/caldav/sync", "", owner), http.StatusOK, "sync now")
	db.First(&link, "id = ?", link.ID)
	mu.Lock()
	got := watched
	mu.Unlock()
	if got == nil || got["address"] != "https://orbit.example.org/api/v1/google/push" || got["type"] != "web_hook" {
		t.Fatalf("watch asked: %#v", got)
	}
	if link.WatchChannel != got["id"] || link.WatchResource != "res-1" || link.WatchUntil == nil || link.WatchError != "" {
		t.Fatalf("link watched: %+v", link)
	}
	token, _ := got["token"].(string)

	push := func(channel, tok, state string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/google/push", nil)
		req.Header.Set("X-Goog-Channel-ID", channel)
		req.Header.Set("X-Goog-Channel-Token", tok)
		req.Header.Set("X-Goog-Resource-State", state)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	expect(t, push(link.WatchChannel, "not-it", "exists"), http.StatusForbidden, "wrong token")
	expect(t, push(link.WatchChannel, token, "sync"), http.StatusOK, "the watch began")
	db.First(&link, "id = ?", link.ID)
	if link.PushedAt != nil {
		t.Error("a 'sync' call is no change")
	}
	expect(t, push(link.WatchChannel, token, "exists"), http.StatusOK, "a change")
	db.First(&link, "id = ?", link.ID)
	if link.PushedAt == nil {
		t.Error("a change should be noted")
	}
	expect(t, push("some-old-channel", "x", "exists"), http.StatusOK, "an old watch: ignored")
	expect(t, push("", "x", "exists"), http.StatusBadRequest, "no channel")

	// Stopping the sync stops the watch (in the background).
	expect(t, authdo(r, "DELETE", base+"/caldav", "", owner), http.StatusNoContent, "stop syncing")
	want := link.WatchChannel + "/res-1"
	for i := 0; i < 100; i++ {
		mu.Lock()
		done := len(stopped) > 0
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(stopped) != 1 || !strings.EqualFold(stopped[0], want) {
		t.Errorf("stopped: %v, want %s", stopped, want)
	}
}

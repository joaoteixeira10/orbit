package inbox

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/events"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
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
	return db
}

// TestRender: the same notification in English and Portuguese, one and many.
func TestRender(t *testing.T) {
	v := View{Verb: "task_added", Count: 1, Params: map[string]any{"actor": "Ana", "title": "Milk", "ws": "Home"}}
	if ti, b := Render("en", v); ti != "Ana added a task" || b != "Milk · Home" {
		t.Errorf("en one: %q / %q", ti, b)
	}
	v.Count = 3
	if ti, b := Render("pt-PT", v); ti != "Ana adicionou 3 tarefas" || b != "A mais recente: Milk · Home" {
		t.Errorf("pt many: %q / %q", ti, b)
	}
	r := View{Verb: "added", Count: 1, Params: map[string]any{"actor": "Ana", "ws": "Home", "role": "viewer"}}
	if ti, b := Render("pt", r); ti != "Ana deu-lhe acesso a Home" || b != "Como leitor" {
		t.Errorf("pt role: %q / %q", ti, b)
	}
	// No workspace name: no dangling separator.
	if _, b := Render("en", View{Verb: "task_due", Params: map[string]any{"title": "Essay", "ws": ""}}); b != "Due now" {
		t.Errorf("trailing separator: %q", b)
	}
}

// TestPrefsDefaults: what's about you reaches your devices; others'
// activity stays in the inbox; reminders' channels follow the older switch.
func TestPrefsDefaults(t *testing.T) {
	p := PrefsOf(models.UserPrefs{NotifyEvents: true})
	if !p[Reminder].Push || !p[Reminder].Channels || !p[Assigned].Push || p[Activity].Push || p[Assigned].Channels {
		t.Errorf("defaults: %+v", p)
	}
	p = PrefsOf(models.UserPrefs{NotifyPrefs: EncodePrefs(map[string]Delivery{Activity: {Push: true}, "bogus": {Push: true}})})
	if !p[Activity].Push || !p[Assigned].Push {
		t.Errorf("stored: %+v", p)
	}
}

// TestGroupAndPublish: a burst of the same activity updates one unread
// entry; each change reaches the open tabs; read entries start a new one.
func TestGroupAndPublish(t *testing.T) {
	db := testDB(t)
	hub := events.New()
	s := New(db, hub, nil, nil, zap.NewNop(), "")
	ch, stop := hub.Subscribe("u")
	defer stop()
	msg := Message{UserID: "u", Kind: Activity, Verb: "task_added", Params: map[string]any{"title": "a"}, Group: "w:ana:task_added"}
	s.Notify(msg)
	msg.Params = map[string]any{"title": "b"}
	s.Notify(msg)
	var rows []models.Notification
	db.Find(&rows)
	if len(rows) != 1 || rows[0].Count != 2 || !strings.Contains(rows[0].Params, `"b"`) {
		t.Fatalf("grouped: %+v", rows)
	}
	if n := len(drainHub(ch)); n != 2 {
		t.Errorf("published %d times, want 2", n)
	}
	now := time.Now()
	db.Model(&models.Notification{}).Where("1 = 1").Update("read_at", &now)
	s.Notify(msg)
	var n int64
	db.Model(&models.Notification{}).Count(&n)
	if n != 2 {
		t.Errorf("after reading, a new entry: %d", n)
	}
}

func drainHub(ch <-chan string) []string {
	var out []string
	for {
		select {
		case m := <-ch:
			out = append(out, m)
		default:
			return out
		}
	}
}

// TestPush: a device gets an encrypted, VAPID-signed message; one the push
// service says is gone is forgotten.
func TestPush(t *testing.T) {
	db := testDB(t)
	s := New(db, nil, nil, nil, zap.NewNop(), "https://orbit.example.org")
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	s.vapidPub, s.vapidPriv = pub, priv

	var mu sync.Mutex
	var got []*http.Request
	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, r)
		code := status
		mu.Unlock()
		w.WriteHeader(code)
	}))
	defer srv.Close()
	s.client = srv.Client()

	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	b64 := base64.RawURLEncoding.EncodeToString
	db.Create(&models.PushSubscription{UserID: "u", Endpoint: srv.URL + "/push/1", P256dh: b64(key.PublicKey().Bytes()), Auth: b64(auth)})

	if n, err := s.TestPush("u", "en"); err != nil || n != 1 {
		t.Fatalf("test push: %d %v", n, err)
	}
	mu.Lock()
	r := got[0]
	mu.Unlock()
	if !strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") || r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("TTL") == "" {
		t.Errorf("headers: %v", r.Header)
	}

	mu.Lock()
	status = http.StatusGone
	mu.Unlock()
	_, _ = s.TestPush("u", "en")
	var n int64
	db.Model(&models.PushSubscription{}).Count(&n)
	if n != 0 {
		t.Errorf("a gone subscription is kept")
	}
	if !ValidEndpoint("https://fcm.googleapis.com/fcm/send/x") || ValidEndpoint("http://example.org/x") || ValidEndpoint("javascript:alert(1)") {
		t.Errorf("endpoint validation")
	}
}

// TestPing: a shared channel's ping names the kind only, in the person's
// language, with a link to the app.
func TestPing(t *testing.T) {
	title, body := Ping("pt-PT", Assigned, "https://orbit.example.org")
	if title != "Orbit: Alguém lhe atribuiu uma tarefa" || body != "Abra o Orbit para a ver: https://orbit.example.org" {
		t.Fatalf("ping: %q / %q", title, body)
	}
	if title, body := Ping("en", "something-new", ""); title != "Orbit: You have a new notification" || body != "Open Orbit to see it" {
		t.Fatalf("ping: %q / %q", title, body)
	}
	personal, shared := SplitChannels([]string{"slack", "email", "ntfy"})
	if len(personal) != 1 || personal[0] != "email" || len(shared) != 2 {
		t.Fatalf("split: %v %v", personal, shared)
	}
}

// TestQuietHours: between the hours (past midnight too, in the user's zone)
// nothing goes out — it waits; reminders only if let through; when the
// hours end, what waited is released (once).
func TestQuietHours(t *testing.T) {
	p := models.UserPrefs{Timezone: "Europe/Lisbon", QuietFrom: "22:00", QuietTo: "07:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 12, 1, h, m, 0, 0, time.UTC) } // Lisbon = UTC in December
	for _, c := range []struct {
		t     time.Time
		quiet bool
	}{{at(21, 59), false}, {at(22, 0), true}, {at(3, 0), true}, {at(6, 59), true}, {at(7, 0), false}, {at(12, 0), false}} {
		if Quiet(p, c.t) != c.quiet {
			t.Errorf("%s: quiet %v", c.t.Format("15:04"), !c.quiet)
		}
	}
	if Quiet(models.UserPrefs{QuietFrom: "22:00"}, at(23, 0)) {
		t.Error("half set counts as quiet")
	}

	db := testDB(t)
	s := New(db, nil, nil, nil, zap.NewNop(), "")
	s.now = func() time.Time { return at(23, 30) }
	p.UserID = "u1"
	db.Create(&p)
	s.Notify(Message{UserID: "u1", Kind: Assigned, Verb: "task_assigned", Params: map[string]any{"title": "A"}})
	s.Notify(Message{UserID: "u1", Kind: Reminder, Verb: "task_due", Params: map[string]any{"title": "B"}})
	var held int64
	db.Model(&models.Notification{}).Where("held = ?", true).Count(&held)
	if held != 2 {
		t.Fatalf("held: %d", held)
	}
	// Reminders let through: not held.
	db.Model(&models.UserPrefs{}).Where("user_id = ?", "u1").Update("quiet_reminders", true)
	s.Notify(Message{UserID: "u1", Kind: Reminder, Verb: "task_due", Params: map[string]any{"title": "C"}})
	db.Model(&models.Notification{}).Where("held = ?", true).Count(&held)
	if held != 2 {
		t.Fatalf("a let-through reminder was held: %d", held)
	}
	s.ReleaseHeld(at(5, 0))
	db.Model(&models.Notification{}).Where("held = ?", true).Count(&held)
	if held != 2 {
		t.Fatalf("released while still quiet: %d", held)
	}
	s.ReleaseHeld(at(7, 1))
	db.Model(&models.Notification{}).Where("held = ?", true).Count(&held)
	if held != 0 {
		t.Fatalf("not released after: %d", held)
	}
}

// TestSnooze: a snoozed notification leaves the list, comes back unread at
// its time (and is shown again); an action link is signed for its
// notification only.
func TestSnooze(t *testing.T) {
	db := testDB(t)
	hub := events.New()
	s := New(db, hub, nil, nil, zap.NewNop(), "")
	s.Notify(Message{UserID: "u1", Kind: Reminder, Verb: "task_due", Params: map[string]any{"title": "Essay"}})
	var n models.Notification
	db.First(&n)
	until := time.Now().Add(time.Hour)
	if err := s.Snooze("u2", n.ID, until); err == nil {
		t.Fatal("someone else snoozed it")
	}
	if err := s.Snooze("u1", n.ID, until); err != nil {
		t.Fatal(err)
	}
	db.First(&n, "id = ?", n.ID)
	if n.SnoozedUntil == nil || n.ReadAt == nil {
		t.Fatalf("snoozed: %+v", n)
	}
	ch, stop := hub.Subscribe("u1")
	defer stop()
	s.Resurface(time.Now())
	if len(ch) != 0 {
		t.Fatal("back too early")
	}
	s.Resurface(until.Add(time.Minute))
	var back models.Notification
	db.First(&back, "id = ?", n.ID)
	if back.SnoozedUntil != nil || back.ReadAt != nil || len(ch) != 1 {
		t.Fatalf("not back: %+v (%d events)", back, len(ch))
	}

	s.vapidPriv = "k"
	tok := s.ActionToken(n.ID)
	if !s.ValidAction(n.ID, tok) || s.ValidAction("other", tok) || s.ValidAction(n.ID, "00") {
		t.Error("action tokens")
	}
}

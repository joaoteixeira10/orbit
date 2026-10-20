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

	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

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

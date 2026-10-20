// Package inbox is Orbit's notifications: every notification lands in the
// user's inbox (the bell), shows at once in their open tabs (SSE), and — as
// each user chooses per kind — goes to their devices by Web Push and to the
// channels the owner set up (email, ntfy, Telegram, …).
//
// A notification records what happened (Kind, Verb, Params), not a sentence;
// the app words it in the reader's language, and this package does the same
// (Render) for push and channels, in the language the user last used.
package inbox

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/events"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Kinds of notification. Each user chooses, per kind, whether it also goes
// to their devices (push) and to their channels.
const (
	Reminder  = "reminder"  // a task is due, an event starts
	Assigned  = "assigned"  // someone gave you a task
	Workspace = "workspace" // added to a workspace, role changed, removed
	Comment   = "comment"   // a comment on your task (or one you follow), an @-mention
	Activity  = "activity"  // others' changes in a shared workspace
	Digest    = "digest"    // the daily summary
)

// Kinds lists them in the order the settings show them.
var Kinds = []string{Reminder, Assigned, Comment, Workspace, Activity, Digest}

// groupWindow is how long a burst of the same activity keeps updating one
// unread entry ("Ana added 4 tasks") instead of adding new ones.
const groupWindow = 15 * time.Minute

// keepFor is how long notifications stay in the inbox.
const keepFor = 90 * 24 * time.Hour

// Message is a notification to send.
type Message struct {
	UserID      string
	Kind        string
	Verb        string
	WorkspaceID string
	ActorID     string
	Params      map[string]any
	Link        string
	// Group, when set, folds a repeat within groupWindow into the unread
	// entry with the same group (its count goes up; no new push).
	Group string
}

// Service sends notifications.
type Service struct {
	db         *gorm.DB
	hub        *events.Hub
	channels   *notify.Service
	log        *zap.Logger
	client     *http.Client
	subscriber string // VAPID "sub": mailto: or https URL of the instance
	publicURL  string // the app's address, for a ping's link ("" = none)
	vapidPub   string
	vapidPriv  string
}

// New builds the service and loads (or, the first time, creates) the Web
// Push keys. publicURL identifies the instance to push services.
func New(db *gorm.DB, hub *events.Hub, channels *notify.Service, sealer *keystore.Sealer, log *zap.Logger, publicURL string) *Service {
	s := &Service{db: db, hub: hub, channels: channels, log: log, client: safedial.PublicClient(15 * time.Second)}
	s.subscriber = "mailto:orbit@localhost"
	s.publicURL = strings.TrimRight(publicURL, "/")
	if strings.HasPrefix(publicURL, "https://") {
		s.subscriber = strings.TrimRight(publicURL, "/")
	}
	if sealer != nil {
		pub, priv, err := loadVAPID(db, sealer)
		if err != nil {
			log.Warn("web push keys unavailable; push is off", zap.Error(err))
		}
		s.vapidPub, s.vapidPriv = pub, priv
	}
	return s
}

// PushReady reports whether Web Push can be used.
func (s *Service) PushReady() bool { return s != nil && s.vapidPub != "" }

// PublicKey is the VAPID public key browsers subscribe with.
func (s *Service) PublicKey() string {
	if s == nil {
		return ""
	}
	return s.vapidPub
}

// Notify stores a notification and delivers it. It is safe on a nil Service
// (tests, or notifications not wired).
func (s *Service) Notify(m Message) {
	if s == nil || m.UserID == "" {
		return
	}
	params := m.Params
	if params == nil {
		params = map[string]any{}
	}
	n, fresh := s.store(m, params)
	if n.ID == "" {
		return
	}
	s.publish(n)
	if fresh {
		s.deliver(n)
	}
}

// store saves the notification, or folds it into an open group. fresh is
// false when an existing entry was updated (no new push or message then).
func (s *Service) store(m Message, params map[string]any) (models.Notification, bool) {
	if m.Group != "" {
		var cur models.Notification
		err := s.db.Where("user_id = ? AND group_key = ? AND read_at IS NULL AND created_at > ?", m.UserID, m.Group, time.Now().Add(-groupWindow)).
			Order("created_at DESC").First(&cur).Error
		if err == nil {
			cur.Count++
			cur.Params = encode(params)
			cur.Link = m.Link
			if s.db.Save(&cur).Error == nil {
				return cur, false
			}
		}
	}
	n := models.Notification{
		UserID: m.UserID, Kind: m.Kind, Verb: m.Verb, WorkspaceID: m.WorkspaceID, ActorID: m.ActorID,
		Params: encode(params), Link: m.Link, GroupKey: m.Group, Count: 1,
	}
	if err := s.db.Create(&n).Error; err != nil {
		s.log.Warn("could not store a notification", zap.Error(err))
		return models.Notification{}, false
	}
	return n, true
}

func encode(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// View is a notification as the API returns it.
type View struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Verb        string         `json:"verb"`
	WorkspaceID string         `json:"workspace_id"`
	ActorID     string         `json:"actor_id"`
	Params      map[string]any `json:"params"`
	Link        string         `json:"link"`
	Count       int            `json:"count"`
	Read        bool           `json:"read"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// ToView decodes a stored notification.
func ToView(n models.Notification) View {
	p := map[string]any{}
	_ = json.Unmarshal([]byte(n.Params), &p)
	return View{
		ID: n.ID, Kind: n.Kind, Verb: n.Verb, WorkspaceID: n.WorkspaceID, ActorID: n.ActorID, Params: p,
		Link: n.Link, Count: n.Count, Read: n.ReadAt != nil, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
	}
}

// publish shows it in the user's open tabs.
func (s *Service) publish(n models.Notification) {
	if s.hub == nil {
		return
	}
	b, _ := json.Marshal(map[string]any{"type": "notification", "notification": ToView(n)})
	s.hub.Publish(n.UserID, string(b))
}

// deliver sends it to the user's devices and channels, as they chose.
func (s *Service) deliver(n models.Notification) {
	var p models.UserPrefs
	s.db.Where("user_id = ?", n.UserID).First(&p)
	d := PrefsOf(p)[n.Kind]
	if !d.Push && !d.Channels {
		return
	}
	var u models.User
	if s.db.Select("id", "email", "disabled").First(&u, "id = ?", n.UserID).Error != nil || u.Disabled {
		return
	}
	v := ToView(n)
	title, body := Render(p.Locale, v)
	if d.Push && s.PushReady() {
		go s.push(n.UserID, pushPayload{Title: title, Body: body, URL: n.Link, Tag: tagOf(n), Kind: n.Kind})
	}
	if d.Channels && s.channels != nil {
		var chans []string
		_ = json.Unmarshal([]byte(p.NotifyChannels), &chans)
		personal, shared := SplitChannels(chans)
		// Email is the person's own: the whole text. The others are places
		// the owner set up, which others may read: a ping, unless the person
		// chose to send the text there too.
		if len(personal) > 0 {
			go s.channels.Send(personal, u.Email, "Orbit: "+title, title+"\n"+body)
		}
		if len(shared) > 0 {
			if p.ChannelDetails {
				go s.channels.Send(shared, u.Email, "Orbit: "+title, title+"\n"+body)
			} else {
				pt, pb := Ping(p.Locale, n.Kind, s.publicURL)
				go s.channels.Send(shared, u.Email, pt, pb)
			}
		}
	}
}

// SplitChannels separates the personal channels (email, to the person's own
// address) from the shared ones (one chat, topic or webhook for everyone).
func SplitChannels(chans []string) (personal, shared []string) {
	for _, c := range chans {
		if c == notify.ChEmail {
			personal = append(personal, c)
		} else {
			shared = append(shared, c)
		}
	}
	return personal, shared
}

// tagOf lets a newer notification of the same thing replace an older one on
// the device (a group, or the same task's reminder).
func tagOf(n models.Notification) string {
	if n.GroupKey != "" {
		return n.GroupKey
	}
	return n.ID
}

// Cleanup deletes notifications older than keepFor.
func (s *Service) Cleanup() {
	if s == nil {
		return
	}
	s.db.Where("created_at < ?", time.Now().Add(-keepFor)).Delete(&models.Notification{})
}

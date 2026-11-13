package inbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// ---- Quiet hours --------------------------------------------------------------

// Zone is where a user's times are read: their chosen zone, else the
// browser's last reported one, else UTC.
func Zone(p models.UserPrefs) *time.Location {
	for _, tz := range []string{p.Timezone, p.DeviceTZ} {
		if tz == "" || tz == "local" {
			continue
		}
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.UTC
}

func clockMinutes(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil || len(s) != 5 {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// ValidClock reports an "HH:MM".
func ValidClock(s string) bool {
	_, ok := clockMinutes(s)
	return ok
}

// Quiet reports whether now falls in the user's quiet hours (which may run
// past midnight: 22:00 – 07:00).
func Quiet(p models.UserPrefs, now time.Time) bool {
	from, ok1 := clockMinutes(p.QuietFrom)
	to, ok2 := clockMinutes(p.QuietTo)
	if !ok1 || !ok2 || from == to {
		return false
	}
	local := now.In(Zone(p))
	m := local.Hour()*60 + local.Minute()
	if from < to {
		return m >= from && m < to
	}
	return m >= from || m < to
}

// holds reports whether a notification waits instead of going out now.
func holds(p models.UserPrefs, kind string, now time.Time) bool {
	if kind == Reminder && p.QuietReminders {
		return false
	}
	return Quiet(p, now)
}

// ReleaseHeld sends, to each user whose quiet hours are over, one summary of
// what waited ("5 notifications while it was quiet"), to their devices and
// channels if any of it would have gone there.
func (s *Service) ReleaseHeld(now time.Time) {
	if s == nil {
		return
	}
	var users []string
	s.db.Model(&models.Notification{}).Where("held = ?", true).Distinct("user_id").Pluck("user_id", &users)
	for _, uid := range users {
		var p models.UserPrefs
		s.db.Where("user_id = ?", uid).First(&p)
		if Quiet(p, now) {
			continue
		}
		var held []models.Notification
		s.db.Where("user_id = ? AND held = ?", uid, true).Order("created_at").Find(&held)
		s.db.Model(&models.Notification{}).Where("user_id = ? AND held = ?", uid, true).Update("held", false)
		var waiting []models.Notification
		for _, n := range held {
			if n.ReadAt == nil { // read meanwhile: nothing to tell
				waiting = append(waiting, n)
			}
		}
		if len(waiting) == 0 {
			continue
		}
		if len(waiting) == 1 {
			s.send(waiting[0], p)
			continue
		}
		titles := make([]string, 0, 3)
		push, chans := false, false
		prefs := PrefsOf(p)
		for i, n := range waiting {
			if i < 3 {
				t, _ := Render(p.Locale, ToView(n))
				titles = append(titles, "· "+t)
			}
			push = push || prefs[n.Kind].Push
			chans = chans || prefs[n.Kind].Channels
		}
		summary := models.Notification{UserID: uid, Kind: Digest, Verb: "held", Count: len(waiting),
			Params: encode(map[string]any{"lines": joinLines(titles)}), Link: ""}
		s.sendAs(summary, p, Delivery{Push: push, Channels: chans})
	}
}

func joinLines(l []string) string {
	out := ""
	for i, s := range l {
		if i > 0 {
			out += "\n"
		}
		out += s
	}
	return out
}

// ---- Snooze -------------------------------------------------------------------

// ErrNotFound: no such notification of that user.
var ErrNotFound = errors.New("notification not found")

// Snooze puts a notification off until then: out of the list (read) until it
// comes back unread and is sent again (Resurface).
func (s *Service) Snooze(userID, id string, until time.Time) error {
	res := s.db.Model(&models.Notification{}).Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{"snoozed_until": until, "read_at": time.Now(), "held": false})
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	if s.hub != nil {
		s.hub.Publish(userID, `{"type":"notifications-read"}`)
	}
	return nil
}

// Resurface brings back the notifications whose snooze is over: unread, on
// top, and sent again to devices and channels.
func (s *Service) Resurface(now time.Time) {
	if s == nil {
		return
	}
	var due []models.Notification
	s.db.Where("snoozed_until IS NOT NULL AND snoozed_until <= ?", now).Find(&due)
	for _, n := range due {
		if s.db.Model(&models.Notification{}).Where("id = ?", n.ID).
			Updates(map[string]any{"snoozed_until": nil, "read_at": nil, "created_at": now, "updated_at": now}).Error != nil {
			continue
		}
		n.SnoozedUntil, n.ReadAt, n.CreatedAt, n.UpdatedAt = nil, nil, now, now
		s.publish(n)
		s.deliver(n)
	}
}

// actionKey signs the links a push notification's buttons call (the service
// worker has no session): from the VAPID private key, so it lasts.
func (s *Service) actionKey() []byte {
	h := sha256.Sum256([]byte("orbit-notification-actions:" + s.vapidPriv))
	return h[:]
}

func (s *Service) sign(id string) []byte {
	mac := hmac.New(sha256.New, s.actionKey())
	mac.Write([]byte(id))
	return mac.Sum(nil)
}

// ActionToken is the signature of an action link for one notification.
func (s *Service) ActionToken(id string) string { return hex.EncodeToString(s.sign(id)) }

// ValidAction checks an action link's signature.
func (s *Service) ValidAction(id, token string) bool {
	if s == nil || s.vapidPriv == "" {
		return false
	}
	got, err := hex.DecodeString(token)
	return err == nil && hmac.Equal(got, s.sign(id))
}

// SnoozeOwner snoozes by an action link (the owner from the notification).
func (s *Service) SnoozeOwner(id string, d time.Duration) error {
	var n models.Notification
	if s.db.Select("id", "user_id").First(&n, "id = ?", id).Error != nil {
		return ErrNotFound
	}
	return s.Snooze(n.UserID, id, time.Now().Add(d))
}

func (s *Service) snoozeURL(id string) string {
	if s.publicURL == "" {
		return fmt.Sprintf("/api/v1/notifications/%s/snooze?token=%s", id, s.ActionToken(id))
	}
	return fmt.Sprintf("%s/api/v1/notifications/%s/snooze?token=%s", s.publicURL, id, s.ActionToken(id))
}

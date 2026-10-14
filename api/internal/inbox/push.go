package inbox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	webpush "github.com/SherClockHolmes/webpush-go"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// The instance's VAPID key pair: the public key in clear, the private key
// sealed with the master key.
const (
	settingVAPIDPublic  = "vapid_public_key"
	settingVAPIDPrivate = "vapid_private_key_sealed"
)

func loadVAPID(db *gorm.DB, sealer *keystore.Sealer) (pub, priv string, err error) {
	var rows []models.Setting
	db.Where("key IN ?", []string{settingVAPIDPublic, settingVAPIDPrivate}).Find(&rows)
	m := map[string]string{}
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	if m[settingVAPIDPublic] != "" && m[settingVAPIDPrivate] != "" {
		blob, err := base64.StdEncoding.DecodeString(m[settingVAPIDPrivate])
		if err != nil {
			return "", "", err
		}
		p, err := sealer.Unseal(blob)
		if err != nil {
			return "", "", err
		}
		return m[settingVAPIDPublic], string(p), nil
	}
	priv, pub, err = webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	sealed, err := sealer.Seal([]byte(priv))
	if err != nil {
		return "", "", err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&models.Setting{Key: settingVAPIDPublic, Value: pub}).Error; err != nil {
			return err
		}
		return tx.Save(&models.Setting{Key: settingVAPIDPrivate, Value: base64.StdEncoding.EncodeToString(sealed)}).Error
	})
	return pub, priv, err
}

// pushPayload is what the service worker receives (web/public/sw.js).
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	Kind  string `json:"kind"`
}

// ValidEndpoint accepts only https push-service addresses (the dialer also
// refuses private and loopback addresses).
func ValidEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && len(raw) <= 1000
}

// push sends to every subscription of a user; ones the push service says are
// gone (404, 410) are deleted.
func (s *Service) push(userID string, p pushPayload) {
	var subs []models.PushSubscription
	s.db.Where("user_id = ?", userID).Find(&subs)
	msg, _ := json.Marshal(p)
	for _, sub := range subs {
		if err := s.sendOne(sub, msg); err != nil {
			s.log.Debug("push failed", zap.Error(err))
		}
	}
}

var errGone = errors.New("subscription gone")

func (s *Service) sendOne(sub models.PushSubscription, msg []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := webpush.SendNotificationWithContext(ctx, msg, &webpush.Subscription{
		Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		HTTPClient: s.client, Subscriber: s.subscriber, VAPIDPublicKey: s.vapidPub, VAPIDPrivateKey: s.vapidPriv,
		TTL: 24 * 3600, Urgency: webpush.UrgencyNormal,
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		s.db.Delete(&models.PushSubscription{}, "id = ?", sub.ID)
		return errGone
	case resp.StatusCode >= 300:
		return errors.New("push service answered " + resp.Status)
	}
	now := time.Now()
	s.db.Model(&models.PushSubscription{}).Where("id = ?", sub.ID).Update("last_used_at", &now)
	return nil
}

// TestPush sends a test notification to one user's devices. It reports how
// many devices took it.
func (s *Service) TestPush(userID, locale string) (int, error) {
	if !s.PushReady() {
		return 0, errors.New("push is not available on this instance")
	}
	var subs []models.PushSubscription
	s.db.Where("user_id = ?", userID).Find(&subs)
	title, body := Render(locale, View{Kind: "test", Verb: "test"})
	msg, _ := json.Marshal(pushPayload{Title: title, Body: body, URL: "/settings#notifications", Tag: "orbit-test", Kind: "test"})
	ok := 0
	var last error
	for _, sub := range subs {
		if err := s.sendOne(sub, msg); err != nil {
			last = err
			continue
		}
		ok++
	}
	if ok == 0 && last != nil {
		return 0, last
	}
	return ok, nil
}

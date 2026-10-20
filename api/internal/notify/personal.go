package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"
)

// Personal channels: a person's own Discord / Slack / Teams webhook, Telegram
// chat, ntfy topic or Gotify app, next to the owner's shared ones. They are
// theirs alone, so they get the full text, as email does. Any user types
// these addresses, so they go out through the public-only client (never the
// LAN or the host itself) and must be https.

// PersonalKinds are the kinds a person can add (email is already theirs).
var PersonalKinds = []string{ChDiscord, ChSlack, ChTeams, ChTelegram, ChNtfy, ChGotify}

// Personal is one personal channel's settings.
type Personal struct {
	Kind       string `json:"kind"`
	WebhookURL string `json:"webhook_url,omitempty"` // discord, slack, teams
	BotToken   string `json:"bot_token,omitempty"`   // telegram
	ChatID     string `json:"chat_id,omitempty"`     // telegram
	ServerURL  string `json:"server_url,omitempty"`  // ntfy, gotify
	Topic      string `json:"topic,omitempty"`       // ntfy
	Token      string `json:"token,omitempty"`       // ntfy (optional), gotify
}

var (
	botToken = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]{20,}$`)
	chatID   = regexp.MustCompile(`^-?\d{1,20}$|^@[A-Za-z0-9_]{4,32}$`)
)

func httpsURL(raw, what string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("%s must be an https:// address", what)
	}
	return nil
}

// Validate checks a personal channel's settings (not that it answers).
func (p Personal) Validate() error {
	switch p.Kind {
	case ChDiscord, ChSlack, ChTeams:
		return httpsURL(p.WebhookURL, "the webhook URL")
	case ChTelegram:
		if !botToken.MatchString(p.BotToken) || !chatID.MatchString(p.ChatID) {
			return errors.New("give the bot token (123456:ABC…) and the chat id")
		}
		return nil
	case ChNtfy:
		if strings.TrimSpace(p.Topic) == "" {
			return errors.New("give the topic")
		}
		return httpsURL(p.ServerURL, "the server URL")
	case ChGotify:
		if strings.TrimSpace(p.Token) == "" {
			return errors.New("give the app token")
		}
		return httpsURL(p.ServerURL, "the server URL")
	}
	return errors.New("unknown kind of channel")
}

// Hint is what the settings show of a channel instead of its secrets: the
// server's host, the ntfy topic, the Telegram chat.
func (p Personal) Hint() string {
	host := func(raw string) string {
		if u, err := url.Parse(raw); err == nil {
			return u.Hostname()
		}
		return ""
	}
	switch p.Kind {
	case ChTelegram:
		return "chat " + p.ChatID
	case ChNtfy:
		return host(p.ServerURL) + "/" + p.Topic
	case ChGotify:
		return host(p.ServerURL)
	}
	return host(p.WebhookURL)
}

// SealPersonal / OpenPersonal store a personal channel's settings encrypted.
func (s *Service) SealPersonal(p Personal) ([]byte, error) {
	b, err := json.Marshal(p)
	if err != nil || s.sealer == nil {
		return b, err
	}
	return s.sealer.Seal(b)
}

func (s *Service) OpenPersonal(blob []byte) (Personal, error) {
	var p Personal
	raw := blob
	if s.sealer != nil {
		var err error
		if raw, err = s.sealer.Unseal(blob); err != nil {
			return p, err
		}
	}
	err := json.Unmarshal(raw, &p)
	return p, err
}

// publicClient sends to addresses users typed: public addresses only.
var publicClient = func() *http.Client { return safedial.PublicClient(15 * time.Second) }

// UsePersonalClient replaces that client (tests, with a local server).
func UsePersonalClient(c *http.Client) { publicClient = func() *http.Client { return c } }

// SendPersonal sends to one personal channel.
func (s *Service) SendPersonal(ctx context.Context, p Personal, subject, body string) error {
	if err := p.Validate(); err != nil {
		return err
	}
	c := publicClient()
	switch p.Kind {
	case ChDiscord:
		return sendDiscord(ctx, c, p.WebhookURL, subject+"\n"+body)
	case ChSlack:
		return sendSlack(ctx, c, p.WebhookURL, subject, body)
	case ChTeams:
		return sendTeams(ctx, c, p.WebhookURL, subject, body)
	case ChTelegram:
		return sendTelegram(ctx, c, p.BotToken, p.ChatID, subject+"\n"+body)
	case ChNtfy:
		return sendNtfy(ctx, c, p.ServerURL, p.Topic, p.Token, subject, body)
	case ChGotify:
		return sendGotify(ctx, c, p.ServerURL, p.Token, subject, body)
	}
	return errors.New("unknown kind of channel")
}

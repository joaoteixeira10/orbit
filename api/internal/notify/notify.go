// Package notify delivers notifications through admin-configured channels:
// email (SMTP), Discord, Telegram, ntfy, Gotify, Microsoft Teams and Slack. Configuration is stored in
// the settings table (managed in the admin UI), replacing the old env-based
// single webhook. Users opt into the channels the admin has enabled.
package notify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// redactedSecret is the placeholder returned to the UI in place of a stored
// secret. On save, a field still equal to this means "leave unchanged".
const redactedSecret = "********"

// Channel keys.
const (
	ChEmail    = "email"
	ChDiscord  = "discord"
	ChTelegram = "telegram"
	ChNtfy     = "ntfy"
	ChGotify   = "gotify"
	ChTeams    = "teams"
	ChSlack    = "slack"
)

// AllChannels lists every channel key, in display order.
var AllChannels = []string{ChEmail, ChDiscord, ChTelegram, ChNtfy, ChGotify, ChTeams, ChSlack}

const settingKey = "notification_config"

// Per-provider configuration.
type EmailConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	StartTLS bool   `json:"start_tls"`
}
type DiscordConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}
type TelegramConfig struct {
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
}
type NtfyConfig struct {
	Enabled   bool   `json:"enabled"`
	ServerURL string `json:"server_url"`
	Topic     string `json:"topic"`
	Token     string `json:"token"`
}
type GotifyConfig struct {
	Enabled   bool   `json:"enabled"`
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
}

// TeamsConfig posts to a Microsoft Teams channel through a Workflows
// "When a Teams webhook request is received" URL (an Adaptive Card).
type TeamsConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

// SlackConfig posts to a Slack incoming webhook.
type SlackConfig struct {
	Enabled    bool   `json:"enabled"`
	WebhookURL string `json:"webhook_url"`
}

// Config aggregates all channel configs.
type Config struct {
	Email    EmailConfig    `json:"email"`
	Discord  DiscordConfig  `json:"discord"`
	Telegram TelegramConfig `json:"telegram"`
	Ntfy     NtfyConfig     `json:"ntfy"`
	Gotify   GotifyConfig   `json:"gotify"`
	Teams    TeamsConfig    `json:"teams"`
	Slack    SlackConfig    `json:"slack"`
	Webhook  WebhookConfig  `json:"webhook"`
}

// WebhookConfig is a generic outbound integration webhook: a signed JSON POST on
// Orbit events, distinct from the human notification channels.
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"` // HMAC-SHA256 key; payload signed in X-Orbit-Signature
}

// Service loads channel config and sends/tests notifications.
type Service struct {
	db     *gorm.DB
	log    *zap.Logger
	client *http.Client
	sealer *keystore.Sealer // seals the stored configuration (nil: plain JSON, tests)
}

// New constructs a notification Service. The HTTP client is SSRF-guarded: it
// blocks loopback/link-local targets at dial time and never follows redirects,
// so a public webhook URL can't 302 to the cloud-metadata endpoint
// (guardOutboundURL is a fast pre-check; this is the real defense).
func New(db *gorm.DB, log *zap.Logger) *Service {
	return &Service{db: db, log: log, client: safedial.GuardedClient(15 * time.Second)}
}

// sealedPrefix marks a configuration stored sealed with the master key.
const sealedPrefix = "sealed:"

// UseSealer makes the configuration (SMTP password, webhook URLs, bot tokens,
// the webhook signing secret) be stored sealed with ORBIT_MASTER_KEY.
func (s *Service) UseSealer(k *keystore.Sealer) *Service {
	s.sealer = k
	return s
}

// SealStored re-saves a configuration that is still in plain text (written
// before sealing, or without a sealer), so it is sealed at rest from now on.
func (s *Service) SealStored() error {
	var row models.Setting
	if s.sealer == nil || s.db.Where(&models.Setting{Key: settingKey}).First(&row).Error != nil ||
		row.Value == "" || strings.HasPrefix(row.Value, sealedPrefix) {
		return nil
	}
	return s.Save(s.Load())
}

// Load reads the stored channel configuration (zero-value if unset).
func (s *Service) Load() Config {
	var row models.Setting
	var cfg Config
	if err := s.db.Where(&models.Setting{Key: settingKey}).First(&row).Error; err != nil {
		return cfg
	}
	raw := []byte(row.Value)
	if v, ok := strings.CutPrefix(row.Value, sealedPrefix); ok {
		blob, err := base64.StdEncoding.DecodeString(v)
		if err == nil && s.sealer != nil {
			raw, err = s.sealer.Unseal(blob)
		}
		if err != nil || s.sealer == nil {
			s.log.Error("notification configuration can't be unsealed (was ORBIT_MASTER_KEY changed?)")
			return cfg
		}
	}
	_ = json.Unmarshal(raw, &cfg)
	return cfg
}

// Save persists the channel configuration, sealed when a sealer is set.
func (s *Service) Save(cfg Config) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	value := string(b)
	if s.sealer != nil {
		blob, err := s.sealer.Seal(b)
		if err != nil {
			return err
		}
		value = sealedPrefix + base64.StdEncoding.EncodeToString(blob)
	}
	return s.db.Save(&models.Setting{Key: settingKey, Value: value, UpdatedAt: time.Now()}).Error
}

// EnabledChannels returns the channel keys the admin has enabled.
func (s *Service) EnabledChannels() []string {
	cfg := s.Load()
	out := []string{} // non-nil → JSON [] (not null) when nothing is enabled
	if cfg.Email.Enabled {
		out = append(out, ChEmail)
	}
	if cfg.Discord.Enabled {
		out = append(out, ChDiscord)
	}
	if cfg.Telegram.Enabled {
		out = append(out, ChTelegram)
	}
	if cfg.Ntfy.Enabled {
		out = append(out, ChNtfy)
	}
	if cfg.Gotify.Enabled {
		out = append(out, ChGotify)
	}
	if cfg.Teams.Enabled {
		out = append(out, ChTeams)
	}
	if cfg.Slack.Enabled {
		out = append(out, ChSlack)
	}
	return out
}

// AnyEnabled reports whether at least one channel is configured.
func (s *Service) AnyEnabled() bool { return len(s.EnabledChannels()) > 0 }

// Broadcast sends an operational message to every enabled channel *except*
// email (which has no broadcast recipient). Best-effort and asynchronous, so
// callers in the request/issuance path never block on delivery.
func (s *Service) Broadcast(subject, body string) {
	var webhooks []string
	for _, ch := range s.EnabledChannels() {
		if ch != ChEmail {
			webhooks = append(webhooks, ch)
		}
	}
	if len(webhooks) == 0 {
		return
	}
	go s.Send(webhooks, "", subject, body)
}

// Test sends a test message via a single channel using the current config.
func (s *Service) Test(channel, recipientEmail string) error {
	cfg := s.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.sendOne(ctx, cfg, channel, recipientEmail, "Orbit test",
		"This is a test notification from Orbit.")
}

// Send delivers a message via each of the requested channels (best-effort).
func (s *Service) Send(channels []string, recipientEmail, subject, body string) {
	cfg := s.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, ch := range channels {
		if err := s.sendOne(ctx, cfg, ch, recipientEmail, subject, body); err != nil {
			s.log.Warn("notification send failed", zap.String("channel", ch), zap.Error(err))
		}
	}
}

// sendOne routes to the provider for a channel, enforcing it is enabled.
func (s *Service) sendOne(ctx context.Context, cfg Config, channel, recipientEmail, subject, body string) error {
	switch channel {
	case ChEmail:
		if !cfg.Email.Enabled {
			return errors.New("email channel is not enabled")
		}
		return sendEmail(cfg.Email, recipientEmail, subject, body)
	case ChDiscord:
		if !cfg.Discord.Enabled {
			return errors.New("discord channel is not enabled")
		}
		if err := guardOutboundURL(cfg.Discord.WebhookURL); err != nil {
			return err
		}
		return sendDiscord(ctx, s.client, cfg.Discord.WebhookURL, subject+"\n"+body)
	case ChTelegram:
		if !cfg.Telegram.Enabled {
			return errors.New("telegram channel is not enabled")
		}
		return sendTelegram(ctx, s.client, cfg.Telegram.BotToken, cfg.Telegram.ChatID, subject+"\n"+body)
	case ChNtfy:
		if !cfg.Ntfy.Enabled {
			return errors.New("ntfy channel is not enabled")
		}
		if err := guardOutboundURL(cfg.Ntfy.ServerURL); err != nil {
			return err
		}
		return sendNtfy(ctx, s.client, cfg.Ntfy.ServerURL, cfg.Ntfy.Topic, cfg.Ntfy.Token, subject, body)
	case ChGotify:
		if !cfg.Gotify.Enabled {
			return errors.New("gotify channel is not enabled")
		}
		if err := guardOutboundURL(cfg.Gotify.ServerURL); err != nil {
			return err
		}
		return sendGotify(ctx, s.client, cfg.Gotify.ServerURL, cfg.Gotify.Token, subject, body)
	case ChTeams:
		if !cfg.Teams.Enabled {
			return errors.New("teams channel is not enabled")
		}
		if err := guardOutboundURL(cfg.Teams.WebhookURL); err != nil {
			return err
		}
		return sendTeams(ctx, s.client, cfg.Teams.WebhookURL, subject, body)
	case ChSlack:
		if !cfg.Slack.Enabled {
			return errors.New("slack channel is not enabled")
		}
		if err := guardOutboundURL(cfg.Slack.WebhookURL); err != nil {
			return err
		}
		return sendSlack(ctx, s.client, cfg.Slack.WebhookURL, subject, body)
	default:
		return fmt.Errorf("unknown channel %q", channel)
	}
}

// guardOutboundURL rejects notification targets that point at loopback,
// link-local (incl. the cloud metadata 169.254.169.254), or unspecified
// addresses — a basic SSRF guard. Private LAN ranges are allowed on purpose:
// self-hosted ntfy/Gotify servers commonly live on the local network.
func guardOutboundURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("invalid notification URL")
	}
	host := u.Hostname()
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("could not resolve %q", host)
		}
		ips = resolved
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
			return errors.New("notification URL points at a blocked address")
		}
	}
	return nil
}

// Redact returns a copy of cfg with every stored secret replaced by a
// placeholder, for safe display in the admin UI.
func Redact(cfg Config) Config {
	mask := func(s string) string {
		if s != "" {
			return redactedSecret
		}
		return ""
	}
	cfg.Email.Password = mask(cfg.Email.Password)
	cfg.Discord.WebhookURL = mask(cfg.Discord.WebhookURL) // capability URL — treat as secret
	cfg.Telegram.BotToken = mask(cfg.Telegram.BotToken)
	cfg.Ntfy.Token = mask(cfg.Ntfy.Token)
	cfg.Gotify.Token = mask(cfg.Gotify.Token)
	cfg.Teams.WebhookURL = mask(cfg.Teams.WebhookURL) // capability URLs
	cfg.Slack.WebhookURL = mask(cfg.Slack.WebhookURL)
	cfg.Webhook.Secret = mask(cfg.Webhook.Secret)
	return cfg
}

// Merge applies an incoming (UI-edited) config over the current one, preserving
// any secret the UI left as the redaction placeholder (i.e. unchanged).
func Merge(current, incoming Config) Config {
	keep := func(in, cur string) string {
		if in == redactedSecret {
			return cur
		}
		return in
	}
	incoming.Email.Password = keep(incoming.Email.Password, current.Email.Password)
	incoming.Discord.WebhookURL = keep(incoming.Discord.WebhookURL, current.Discord.WebhookURL)
	incoming.Telegram.BotToken = keep(incoming.Telegram.BotToken, current.Telegram.BotToken)
	incoming.Ntfy.Token = keep(incoming.Ntfy.Token, current.Ntfy.Token)
	incoming.Gotify.Token = keep(incoming.Gotify.Token, current.Gotify.Token)
	incoming.Teams.WebhookURL = keep(incoming.Teams.WebhookURL, current.Teams.WebhookURL)
	incoming.Slack.WebhookURL = keep(incoming.Slack.WebhookURL, current.Slack.WebhookURL)
	incoming.Webhook.Secret = keep(incoming.Webhook.Secret, current.Webhook.Secret)
	return incoming
}

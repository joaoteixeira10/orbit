package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// webhookPayload is the JSON body POSTed to the integration webhook.
type webhookPayload struct {
	Event string `json:"event"` // e.g. "server.disabled"
	Time  string `json:"time"`  // RFC3339 (UTC)
	Data  any    `json:"data"`
}

// FireWebhook POSTs a signed lifecycle event to the configured integration
// webhook, if enabled. Best-effort and asynchronous so it never blocks or fails
// the action it describes. The body is signed with HMAC-SHA256 over the exact
// bytes sent, in the `X-Orbit-Signature: sha256=<hex>` header.
func (s *Service) FireWebhook(event string, data any) {
	cfg := s.Load().Webhook
	if !cfg.Enabled || cfg.URL == "" {
		return
	}
	if err := guardOutboundURL(cfg.URL); err != nil {
		s.log.Warn("webhook target rejected", zap.Error(err))
		return
	}
	body, err := json.Marshal(webhookPayload{
		Event: event,
		Time:  time.Now().UTC().Format(time.RFC3339),
		Data:  data,
	})
	if err != nil {
		return
	}
	go s.deliverWebhook(cfg, body, event)
}

func (s *Service) deliverWebhook(cfg WebhookConfig, body []byte, event string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Orbit-Event", event)
	if cfg.Secret != "" {
		mac := hmac.New(sha256.New, []byte(cfg.Secret))
		mac.Write(body)
		req.Header.Set("X-Orbit-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	if err := do(s.client, req); err != nil {
		s.log.Warn("webhook delivery failed", zap.Error(err))
	}
}

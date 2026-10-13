package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
)

// ---- Provider send functions ----

func sendEmail(cfg EmailConfig, to, subject, body string) error {
	if to == "" {
		return fmt.Errorf("notify: no recipient email")
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	msg := []byte(strings.Join([]string{
		"From: " + cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"", body,
	}, "\r\n"))
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return smtp.SendMail(addr, auth, cfg.From, []string{to}, msg)
}

func postJSON(ctx context.Context, client *http.Client, urlStr string, payload any) error {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return do(client, req)
}

func do(client *http.Client, req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify: %s returned %d", req.URL.Host, resp.StatusCode)
	}
	return nil
}

func sendDiscord(ctx context.Context, client *http.Client, webhook, content string) error {
	return postJSON(ctx, client, webhook, map[string]string{"content": content})
}

func sendTelegram(ctx context.Context, client *http.Client, token, chatID, text string) error {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	return postJSON(ctx, client, api, map[string]string{"chat_id": chatID, "text": text})
}

func sendNtfy(ctx context.Context, client *http.Client, server, topic, token, title, body string) error {
	endpoint := strings.TrimRight(server, "/") + "/" + url.PathEscape(topic)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(client, req)
}

func sendGotify(ctx context.Context, client *http.Client, server, token, title, body string) error {
	endpoint := strings.TrimRight(server, "/") + "/message?token=" + url.QueryEscape(token)
	return postJSON(ctx, client, endpoint, map[string]any{"title": title, "message": body, "priority": 5})
}

// sendTeams posts an Adaptive Card to a Teams Workflows webhook.
func sendTeams(ctx context.Context, client *http.Client, webhook, title, body string) error {
	card := map[string]any{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []any{
			map[string]any{"type": "TextBlock", "text": title, "weight": "Bolder", "size": "Medium", "wrap": true},
			map[string]any{"type": "TextBlock", "text": body, "wrap": true},
		},
	}
	return postJSON(ctx, client, webhook, map[string]any{
		"type":        "message",
		"attachments": []any{map[string]any{"contentType": "application/vnd.microsoft.card.adaptive", "content": card}},
	})
}

// sendSlack posts to a Slack incoming webhook.
func sendSlack(ctx context.Context, client *http.Client, webhook, title, body string) error {
	return postJSON(ctx, client, webhook, map[string]string{"text": "*" + title + "*\n" + body})
}

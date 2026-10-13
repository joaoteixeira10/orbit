package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTeamsAndSlackPayloads: Teams gets an Adaptive Card message (Workflows),
// Slack a text with the bold title.
func TestTeamsAndSlackPayloads(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	if err := sendTeams(context.Background(), srv.Client(), srv.URL, "[Orbit] DOWN: web01", "Failing its health checks"); err != nil {
		t.Fatal(err)
	}
	att := got["attachments"].([]any)[0].(map[string]any)
	card := att["content"].(map[string]any)
	if got["type"] != "message" || att["contentType"] != "application/vnd.microsoft.card.adaptive" || card["type"] != "AdaptiveCard" {
		t.Errorf("teams payload: %v", got)
	}
	if b, _ := json.Marshal(card["body"]); !strings.Contains(string(b), "DOWN: web01") || !strings.Contains(string(b), "Failing its health checks") {
		t.Errorf("teams card body: %s", b)
	}

	if err := sendSlack(context.Background(), srv.Client(), srv.URL, "Title", "Body"); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "*Title*\nBody" {
		t.Errorf("slack payload: %v", got)
	}
}

// TestTeamsSlackEnabledAndRedacted: the new channels count as enabled and
// their URLs are redacted, then kept on save.
func TestTeamsSlackEnabledAndRedacted(t *testing.T) {
	cfg := Config{Teams: TeamsConfig{Enabled: true, WebhookURL: "https://t.example.org/x"}, Slack: SlackConfig{Enabled: true, WebhookURL: "https://hooks.slack.com/y"}}
	r := Redact(cfg)
	if r.Teams.WebhookURL != redactedSecret || r.Slack.WebhookURL != redactedSecret {
		t.Errorf("redact: %+v", r)
	}
	m := Merge(cfg, r)
	if m.Teams.WebhookURL != cfg.Teams.WebhookURL || m.Slack.WebhookURL != cfg.Slack.WebhookURL {
		t.Errorf("merge: %+v", m)
	}
}

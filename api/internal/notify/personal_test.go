package notify

import "testing"

// TestPersonalValidate: https only, the fields each kind needs, a hint that
// shows no secret.
func TestPersonalValidate(t *testing.T) {
	ok := []Personal{
		{Kind: ChDiscord, WebhookURL: "https://discord.com/api/webhooks/1/abc"},
		{Kind: ChTelegram, BotToken: "123456:ABCdefGhIJKlmNoPQRsTUVwxyZ0123", ChatID: "-1001234"},
		{Kind: ChNtfy, ServerURL: "https://ntfy.sh", Topic: "jt-orbit"},
		{Kind: ChGotify, ServerURL: "https://push.example.org", Token: "Axyz"},
	}
	for _, p := range ok {
		if err := p.Validate(); err != nil {
			t.Errorf("%s refused: %v", p.Kind, err)
		}
	}
	bad := []Personal{
		{Kind: ChSlack, WebhookURL: "http://hooks.slack.com/x"},
		{Kind: ChTeams, WebhookURL: "https://user:pw@example.org/x"},
		{Kind: ChTelegram, BotToken: "x/../y", ChatID: "1"},
		{Kind: ChNtfy, ServerURL: "https://ntfy.sh"},
		{Kind: "email"},
	}
	for _, p := range bad {
		if p.Validate() == nil {
			t.Errorf("%+v accepted", p)
		}
	}
	if h := ok[0].Hint(); h != "discord.com" {
		t.Errorf("hint %q", h)
	}
	if h := ok[2].Hint(); h != "ntfy.sh/jt-orbit" {
		t.Errorf("hint %q", h)
	}
}

package notify

import "testing"

func TestGuardOutboundURL(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/x",
		"http://localhost/x",
		"http://169.254.169.254/latest/meta-data", // cloud metadata (link-local)
		"http://[::1]/x",
		"http://0.0.0.0/x",
		"ftp://example.com/x", // wrong scheme
		"not a url",
	}
	for _, u := range blocked {
		if err := guardOutboundURL(u); err == nil {
			t.Errorf("expected %q to be blocked", u)
		}
	}
	// Private LAN + public hosts are allowed (self-hosted ntfy/Gotify live on the LAN).
	for _, u := range []string{"https://discord.com/api/webhooks/1/abc", "http://192.168.1.50:8080/x", "http://10.0.0.5/x"} {
		if err := guardOutboundURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got %v", u, err)
		}
	}
}

func TestRedactAndMerge(t *testing.T) {
	cur := Config{}
	cur.Email.Password = "smtp-secret"
	cur.Gotify.Token = "gotify-secret"
	cur.Ntfy.ServerURL = "https://ntfy.lan" // not a secret

	red := Redact(cur)
	if red.Email.Password != redactedSecret || red.Gotify.Token != redactedSecret {
		t.Fatal("secrets should be redacted on read")
	}
	if red.Ntfy.ServerURL != "https://ntfy.lan" {
		t.Fatal("non-secret fields must not be redacted")
	}

	// UI re-saves the redacted form but changes the gotify token.
	incoming := red
	incoming.Gotify.Token = "new-gotify-secret"
	merged := Merge(cur, incoming)
	if merged.Email.Password != "smtp-secret" {
		t.Errorf("unchanged (redacted) secret should be preserved, got %q", merged.Email.Password)
	}
	if merged.Gotify.Token != "new-gotify-secret" {
		t.Errorf("changed secret should be applied, got %q", merged.Gotify.Token)
	}
}

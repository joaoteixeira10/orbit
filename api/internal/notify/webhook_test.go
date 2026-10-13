package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// TestWebhookDelivery checks the signed POST: the body is sent verbatim and the
// X-Orbit-Signature header is HMAC-SHA256 over it. (deliverWebhook is synchronous
// and skips the SSRF guard, which is applied by FireWebhook.)
func TestWebhookDelivery(t *testing.T) {
	type captured struct {
		event string
		sig   string
		body  []byte
	}
	ch := make(chan captured, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- captured{event: r.Header.Get("X-Orbit-Event"), sig: r.Header.Get("X-Orbit-Signature"), body: b}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := New(nil, zap.NewNop())
	// The production client is SSRF-guarded (blocks loopback); this test targets a
	// loopback httptest server to verify the signing/headers, so use a plain client.
	// The guard itself is covered by internal/safedial.
	s.client = srv.Client()
	payload := []byte(`{"event":"certificate.issued","data":{"id":1}}`)
	s.deliverWebhook(WebhookConfig{Enabled: true, URL: srv.URL, Secret: "shh"}, payload, "certificate.issued")

	c := <-ch
	if c.event != "certificate.issued" {
		t.Errorf("X-Orbit-Event: %q", c.event)
	}
	if string(c.body) != string(payload) {
		t.Errorf("body not sent verbatim: %q", c.body)
	}
	mac := hmac.New(sha256.New, []byte("shh"))
	mac.Write(payload)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if c.sig != want {
		t.Errorf("signature: got %q, want %q", c.sig, want)
	}
}

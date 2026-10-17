package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIPRateLimiterBurstAndDeny(t *testing.T) {
	l := newIPRateLimiter(3, 0) // burst 3, no refill
	now := time.Now()
	for i := 0; i < 3; i++ {
		if !l.allow("1.2.3.4", now) {
			t.Fatalf("request %d within burst should be allowed", i)
		}
	}
	if l.allow("1.2.3.4", now) {
		t.Fatal("4th request over burst should be denied")
	}
	if !l.allow("5.6.7.8", now) {
		t.Fatal("a different IP should have its own bucket")
	}
}

func TestIPRateLimiterRefill(t *testing.T) {
	l := newIPRateLimiter(1, 1) // burst 1, 1 token/sec
	now := time.Now()
	if !l.allow("ip", now) {
		t.Fatal("first request should pass")
	}
	if l.allow("ip", now) {
		t.Fatal("immediate second request should be denied")
	}
	if !l.allow("ip", now.Add(2*time.Second)) {
		t.Fatal("a token should have refilled after 2s")
	}
}

func TestSecurityHeadersSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(securityHeaders(func() string { return "https://pki-cert.example.org" }))
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/x", nil)
	r.ServeHTTP(w, req)

	// The extra connect-src origin must land in the CSP.
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "connect-src 'self' https://pki-cert.example.org") {
		t.Errorf("CSP connect-src missing extra origin: %q", csp)
	}
	// Without it the branded blob: manifest is blocked and the app can't be installed.
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "manifest-src 'self' blob:") {
		t.Errorf("CSP manifest-src missing: %q", csp)
	}

	for _, h := range []string{
		"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy",
	} {
		if w.Header().Get(h) == "" {
			t.Errorf("expected security header %q to be set", h)
		}
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestRateLimitMiddlewareReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(newIPRateLimiter(1, 0).middleware())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	do := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := do(); code != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", code)
	}
	if code := do(); code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want 429", code)
	}
}

// TestLogPathHidesFeedTokens: a calendar feed's address is a secret and
// doesn't reach the request log.
func TestLogPathHidesFeedTokens(t *testing.T) {
	if got := logPath("/ical/0123456789abcdef.ics"); got != "/ical/…" {
		t.Errorf("feed path logged as %q", got)
	}
	if got := logPath("/api/v1/invites/s3cr3t/accept"); got != "/api/v1/invites/…/accept" {
		t.Errorf("invite path logged as %q", got)
	}
	if got := logPath("/join/s3cr3t"); got != "/join/…" {
		t.Errorf("join path logged as %q", got)
	}
	if got := logPath("/api/v1/workspaces"); got != "/api/v1/workspaces" {
		t.Errorf("ordinary path changed: %q", got)
	}
}

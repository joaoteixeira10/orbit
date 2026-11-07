package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/config"

	"github.com/gin-gonic/gin"
)

// reqWithCookies issues a POST carrying the given cookies and (optionally) the
// CSRF header, returning the recorder.
func reqWithCookies(r *gin.Engine, path string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		if ck != nil {
			req.AddCookie(ck)
		}
	}
	if csrf != "" {
		req.Header.Set(csrfHeaderName, csrf)
	}
	r.ServeHTTP(w, req)
	return w
}

// loginCookies logs in the seeded admin and returns the refresh + csrf cookies.
func loginCookies(t *testing.T, r *gin.Engine) (refresh, csrf *http.Cookie, body string) {
	t.Helper()
	w := do(r, "POST", "/api/v1/auth/login", `{"username":"admin","password":"adminpass"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login: got %d body=%s", w.Code, w.Body.String())
	}
	for _, ck := range w.Result().Cookies() {
		switch ck.Name {
		case refreshCookieName:
			refresh = ck
		case csrfCookieName:
			csrf = ck
		}
	}
	return refresh, csrf, w.Body.String()
}

func TestLoginSetsAuthCookies(t *testing.T) {
	r := newTestRouter(t)
	refresh, csrf, body := loginCookies(t, r)

	if refresh == nil || refresh.Value == "" {
		t.Fatal("login did not set the refresh cookie")
	}
	if !refresh.HttpOnly {
		t.Error("refresh cookie must be HttpOnly")
	}
	if refresh.SameSite != http.SameSiteStrictMode {
		t.Error("refresh cookie must be SameSite=Strict")
	}
	if refresh.Path != refreshCookiePath {
		t.Errorf("refresh cookie path = %q, want %q", refresh.Path, refreshCookiePath)
	}
	if csrf == nil || csrf.Value == "" {
		t.Fatal("login did not set the csrf cookie")
	}
	if csrf.HttpOnly {
		t.Error("csrf cookie must be readable (not HttpOnly)")
	}
	// Phase 3: the refresh token must NOT appear in the response body.
	if strings.Contains(body, "refresh_token") {
		t.Error("login response must not expose refresh_token in the body")
	}
}

func TestRefreshRequiresCSRF(t *testing.T) {
	r := newTestRouter(t)
	refresh, csrf, _ := loginCookies(t, r)

	// Cookie present but no CSRF header → rejected.
	if w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{refresh, csrf}, ""); w.Code != http.StatusForbidden {
		t.Errorf("refresh without CSRF header: got %d, want 403", w.Code)
	}
	// Cookie + matching CSRF header → accepted, new access token, no body refresh.
	w := reqWithCookies(r, "/api/v1/auth/refresh", []*http.Cookie{refresh, csrf}, csrf.Value)
	if w.Code != http.StatusOK {
		t.Fatalf("refresh with CSRF: got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "access_token") {
		t.Error("refresh must return a new access token")
	}
	if strings.Contains(w.Body.String(), "refresh_token") {
		t.Error("refresh response must not expose refresh_token in the body")
	}
}

func TestRefreshWithoutCookieFails(t *testing.T) {
	r := newTestRouter(t)
	// No cookies at all → 401 (missing refresh token), not a 500.
	if w := reqWithCookies(r, "/api/v1/auth/refresh", nil, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("refresh with no cookie: got %d, want 401", w.Code)
	}
}

func TestCookieDomainScoping(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h := &Handlers{cfg: &config.Config{CookieDomain: "example.org"}}
	h.setAuthCookies(c, "tok")
	for _, ck := range w.Result().Cookies() {
		if ck.Domain != "example.org" {
			t.Errorf("cookie %q domain = %q, want example.org", ck.Name, ck.Domain)
		}
	}

	// Default (no COOKIE_DOMAIN) → host-only (empty Domain).
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	(&Handlers{cfg: &config.Config{}}).setAuthCookies(c2, "tok")
	for _, ck := range w2.Result().Cookies() {
		if ck.Domain != "" {
			t.Errorf("host-only cookie %q got domain %q, want empty", ck.Name, ck.Domain)
		}
	}
}

func TestLogoutClearsCookie(t *testing.T) {
	r := newTestRouter(t)
	refresh, csrf, _ := loginCookies(t, r)

	w := reqWithCookies(r, "/api/v1/auth/logout", []*http.Cookie{refresh, csrf}, csrf.Value)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout: got %d", w.Code)
	}
	cleared := false
	for _, ck := range w.Result().Cookies() {
		if ck.Name == refreshCookieName && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout must expire the refresh cookie")
	}
}

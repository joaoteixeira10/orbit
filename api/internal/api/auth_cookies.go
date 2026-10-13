package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"

	"github.com/gin-gonic/gin"
)

// Auth cookie names. The refresh token rides in an HttpOnly cookie (never
// readable by JS); the CSRF token is a readable companion for the double-submit
// check on the cookie-bearing endpoints (/auth/refresh, /auth/logout).
const (
	refreshCookieName = "orbit_refresh"
	csrfCookieName    = "orbit_csrf"
	csrfHeaderName    = "X-CSRF-Token"
	// refreshCookiePath scopes the refresh cookie to the auth endpoints only, so
	// it's never attached to ordinary API requests.
	refreshCookiePath = "/api/v1/auth"
)

// setAuthCookies issues the HttpOnly refresh cookie (Strict, path-scoped) plus a
// readable CSRF companion cookie. Secure is gated on config (TLS/release).
func (h *Handlers) setAuthCookies(c *gin.Context, refresh string) {
	secure := h.cfg.CookieSecure
	domain := h.cfg.CookieDomain // "" = host-only; set to share across subdomains
	maxAge := int(auth.RefreshTokenTTL.Seconds())
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, refresh, maxAge, refreshCookiePath, domain, secure, true) // HttpOnly
	c.SetCookie(csrfCookieName, randomCSRFToken(), maxAge, "/", domain, secure, false)       // readable by JS
}

// clearAuthCookies expires both auth cookies (logout).
func (h *Handlers) clearAuthCookies(c *gin.Context) {
	secure := h.cfg.CookieSecure
	domain := h.cfg.CookieDomain
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, domain, secure, true)
	c.SetCookie(csrfCookieName, "", -1, "/", domain, secure, false)
}

// refreshTokenFromRequest returns the refresh token, preferring an explicit body
// value (programmatic) and falling back to the HttpOnly cookie (browser). CSRF is
// enforced unconditionally by the callers, so the source no longer needs to be
// reported.
func refreshTokenFromRequest(c *gin.Context, body string) string {
	if body != "" {
		return body
	}
	if ck, err := c.Cookie(refreshCookieName); err == nil && ck != "" {
		return ck
	}
	return ""
}

// csrfOK verifies the double-submit token: the X-CSRF-Token header must match
// the readable CSRF cookie. Used only when the refresh token came from the
// cookie (the bearer/body paths carry no ambient credential, so no CSRF risk).
func csrfOK(c *gin.Context) bool {
	cookie, err := c.Cookie(csrfCookieName)
	if err != nil || cookie == "" {
		return false
	}
	return c.GetHeader(csrfHeaderName) == cookie
}

func randomCSRFToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

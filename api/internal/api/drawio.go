package api

import (
	"net/url"
	"strings"
	"sync/atomic"
)

// The draw.io editor's address (Diagrams load it in a frame): the admin's,
// else DRAWIO_URL, else draw.io's own. Kept here, so every response's
// Content-Security-Policy can let that one address be framed without
// reading the settings.

const (
	keyDrawioURL     = "drawio_url"
	defaultDrawioURL = "https://embed.diagrams.net"
)

var drawioNow atomic.Pointer[string]

// cleanDrawioURL checks an editor address: http(s), a host, no user or
// query; "" when it isn't one. The trailing slash goes.
func cleanDrawioURL(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, " ;'\"") {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/")
}

// refreshDrawio works the address out again (at start, after a change).
func (h *Handlers) refreshDrawio() string {
	v := cleanDrawioURL(h.loadSettings()[keyDrawioURL])
	if v == "" && h.cfg != nil {
		v = cleanDrawioURL(h.cfg.DrawioURL)
	}
	if v == "" {
		v = defaultDrawioURL
	}
	drawioNow.Store(&v)
	return v
}

// drawioOrigin is the editor's origin, for frame-src ("" before it's known).
func drawioOrigin() string {
	p := drawioNow.Load()
	if p == nil {
		return ""
	}
	u, err := url.Parse(*p)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

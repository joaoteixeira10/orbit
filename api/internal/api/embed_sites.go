package api

import (
	"net"
	"net/url"
	"strings"
	"sync/atomic"
)

// The sites the Apps module may show in a frame: the owner's list (Owner
// settings), else EMBED_SITES. Kept here, like the draw.io address, so every
// response's Content-Security-Policy (frame-src) names exactly them without
// reading the settings. Each is an origin — scheme, host, port — and a host
// may start with "*." for every subdomain (as CSP reads it).

const (
	keyEmbedSites = "embed_sites" // JSON []string; once saved it wins, even empty
	maxEmbedSites = 50
)

var embedSitesNow atomic.Pointer[[]string]

// cleanEmbedSite reads a site as an origin ("https://gitea.example.org",
// "http://grafana:3000", "https://*.example.org"); "" when it isn't one.
// A path, a query or a user name aren't a site's.
func cleanEmbedSite(s string) string {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	wild := false
	if i := strings.Index(s, "://*."); i > 0 {
		wild, s = true, s[:i+3]+s[i+5:]
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(u.Host, " ;,'\"*") {
		return ""
	}
	host := strings.ToLower(u.Host)
	if p := u.Port(); (u.Scheme == "https" && p == "443") || (u.Scheme == "http" && p == "80") {
		host = strings.ToLower(u.Hostname())
		if strings.Contains(host, ":") { // IPv6
			host = "[" + host + "]"
		}
	}
	if wild {
		if net.ParseIP(u.Hostname()) != nil {
			return ""
		}
		host = "*." + host
	}
	return u.Scheme + "://" + host
}

// cleanEmbedSites cleans a list; bad is the first entry that isn't a site.
func cleanEmbedSites(in []string) (out []string, bad string) {
	out = []string{}
	seen := map[string]bool{}
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		c := cleanEmbedSite(s)
		if c == "" {
			return nil, s
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, ""
}

// refreshEmbedSites works the list out again (at start, after a change).
func (h *Handlers) refreshEmbedSites() {
	var sites []string
	if v, ok := h.loadSettings()[keyEmbedSites]; ok {
		sites, _ = cleanEmbedSites(decodeList(v))
	} else if h.cfg != nil {
		sites, _ = cleanEmbedSites(h.cfg.EmbedSites)
	}
	if sites == nil {
		sites = []string{}
	}
	embedSitesNow.Store(&sites)
}

func embedSites() []string {
	if p := embedSitesNow.Load(); p != nil {
		return *p
	}
	return nil
}

// frameOrigins is what frame-src allows besides Orbit: the draw.io editor
// and the Apps module's sites.
func frameOrigins() string {
	out := drawioOrigin()
	for _, s := range embedSites() {
		out = strings.TrimSpace(out + " " + s)
	}
	return out
}

// siteMatches tells whether an address is on a site (an origin as
// cleanEmbedSite gives it, perhaps with "*.").
func siteMatches(site string, u *url.URL) bool {
	scheme, host, ok := strings.Cut(site, "://")
	if !ok || !strings.EqualFold(u.Scheme, scheme) {
		return false
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	sh, sp := host, ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		sh, sp = h, p
	}
	sh = strings.Trim(sh, "[]")
	if port != sp {
		return false
	}
	name := strings.ToLower(u.Hostname())
	if rest, wild := strings.CutPrefix(sh, "*."); wild {
		return strings.HasSuffix(name, "."+rest)
	}
	return name == sh
}

// embedAllowed tells whether an address is on one of the allowed sites.
func embedAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for _, s := range embedSites() {
		if siteMatches(s, u) {
			return true
		}
	}
	return false
}

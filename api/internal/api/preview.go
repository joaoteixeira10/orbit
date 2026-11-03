package api

import (
	"html"
	"strings"

	"github.com/gin-gonic/gin"
)

// Link previews: Slack, Teams, WhatsApp and the like fetch a pasted link
// without signing in and show what its page's Open Graph tags say. Every
// page of the app gets the instance's name and icon; only a guest link —
// public by its owner's choice — names what it opens (its page's title and
// first words). A private link (/w/…) says nothing of what's behind it.
func (h *Handlers) previewTags(c *gin.Context) string {
	app := h.loadSettings()[keyAppName]
	if app == "" {
		app = "Orbit"
	}
	base := h.cfg.PublicURL
	if base == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + c.Request.Host
	}
	title, desc := app, "Open it in "+app+" (signing in if asked)."
	p := c.Request.URL.Path
	if tok, ok := strings.CutPrefix(p, "/shared/"); ok {
		if g, ok := h.shareOf(strings.Trim(tok, "/")); ok {
			title = g.root.Title
			if title == "" {
				title = "Untitled"
			}
			desc = clip(strings.Join(strings.Fields(g.root.ContentText), " "), 200)
			if desc == "" {
				desc = "A page shared from " + app + "."
			}
			app = g.ws.Name + " · " + app
		}
	}
	tag := func(prop, val string) string {
		return `<meta property="` + prop + `" content="` + html.EscapeString(val) + `" />`
	}
	return strings.Join([]string{
		tag("og:site_name", app),
		tag("og:title", title),
		tag("og:description", desc),
		tag("og:type", "website"),
		tag("og:url", base+p),
		tag("og:image", base+"/favicon-512.png"),
		`<meta name="twitter:card" content="summary" />`,
	}, "\n    ")
}

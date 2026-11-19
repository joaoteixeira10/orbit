package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Apps: other sites — a Gitea, a Grafana, a wiki — pinned in a workspace and
// shown full-size in a frame, one tab each. Only on the sites Orbit's owner
// allows (embed_sites.go); the site keeps its own sign-in, and Orbit never
// hands it anything.

const maxWorkspaceApps = 50

type appResponse struct {
	models.WorkspaceApp
	// Allowed: its address is on an allowed site still (the owner may have
	// taken the site off the list since).
	Allowed bool `json:"allowed"`
}

func toApp(a models.WorkspaceApp) appResponse {
	return appResponse{WorkspaceApp: a, Allowed: embedAllowed(a.URL)}
}

func (h *Handlers) appAccess(c *gin.Context, need string) (models.WorkspaceApp, bool) {
	var a models.WorkspaceApp
	if err := h.db.First(&a, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
		return a, false
	}
	_, _, ok := h.wsAccess(c, a.WorkspaceID, need)
	return a, ok
}

// cleanAppURL checks an app's address: http(s), a host, no user name, on an
// allowed site. msg says what's wrong ("" when it's fine).
func cleanAppURL(s string) (string, string) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || len(s) > 2000 {
		return "", "the address must be an http(s) address"
	}
	if !embedAllowed(u.String()) {
		return "", "that site isn't allowed here; Orbit's owner can allow it (Owner → Apps)"
	}
	return u.String(), ""
}

type embedSitesResponse struct {
	Sites []string `json:"sites"`
}

// listEmbedSites returns the sites apps may be on (to anyone signed in: the
// add form names them).
//
//	@Summary	The sites the Apps module may show
//	@Tags		apps
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	embedSitesResponse
//	@Router		/embed-sites [get]
func (h *Handlers) listEmbedSites(c *gin.Context) {
	c.JSON(http.StatusOK, embedSitesResponse{Sites: append([]string{}, embedSites()...)})
}

// listApps lists a workspace's apps, in their tabs' order.
//
//	@Summary	A workspace's apps
//	@Tags		apps
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	appResponse
//	@Router		/workspaces/{ws}/apps [get]
func (h *Handlers) listApps(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.WorkspaceApp
	h.db.Where("workspace_id = ?", ws.ID).Order("position, created_at").Find(&list)
	out := make([]appResponse, len(list))
	for i, a := range list {
		out[i] = toApp(a)
	}
	c.JSON(http.StatusOK, out)
}

type appRequest struct {
	Name *string `json:"name" binding:"omitempty,max=80"`
	URL  *string `json:"url"`
}

// createApp pins a site in a workspace (editors), as its last tab.
//
//	@Summary	Add an app
//	@Tags		apps
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		appRequest	true	"Its name and address"
//	@Success	201		{object}	appResponse
//	@Router		/workspaces/{ws}/apps [post]
func (h *Handlers) createApp(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req appRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.URL == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}
	u, msg := cleanAppURL(*req.URL)
	if msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var n int64
	h.db.Model(&models.WorkspaceApp{}).Where("workspace_id = ?", ws.ID).Count(&n)
	if n >= maxWorkspaceApps {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a workspace keeps up to 50 apps"})
		return
	}
	a := models.WorkspaceApp{WorkspaceID: ws.ID, URL: u, Position: int(n), CreatedBy: auth.UserID(c)}
	if req.Name != nil {
		a.Name = strings.TrimSpace(*req.Name)
	}
	if a.Name == "" {
		pu, _ := url.Parse(u)
		a.Name = pu.Hostname()
	}
	if err := h.db.Create(&a).Error; err != nil {
		h.internalError(c, err, "could not add the app")
		return
	}
	h.publishWS(c, ws.ID, "apps")
	c.JSON(http.StatusCreated, toApp(a))
}

// updateApp renames an app or changes its address (editors).
//
//	@Summary	Change an app
//	@Tags		apps
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"App id"
//	@Param		body	body		appRequest	true	"Its name and/or address"
//	@Success	200		{object}	appResponse
//	@Router		/apps/{id} [put]
func (h *Handlers) updateApp(c *gin.Context) {
	a, ok := h.appAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req appRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.URL != nil && strings.TrimSpace(*req.URL) != a.URL {
		u, msg := cleanAppURL(*req.URL)
		if msg != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": msg})
			return
		}
		a.URL = u
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		a.Name = strings.TrimSpace(*req.Name)
	}
	if err := h.db.Model(&models.WorkspaceApp{}).Where("id = ?", a.ID).Updates(map[string]any{"name": a.Name, "url": a.URL}).Error; err != nil {
		h.internalError(c, err, "could not save the app")
		return
	}
	h.publishWS(c, a.WorkspaceID, "apps")
	c.JSON(http.StatusOK, toApp(a))
}

// deleteApp unpins an app (editors). The site isn't touched.
//
//	@Summary	Remove an app
//	@Tags		apps
//	@Security	BearerAuth
//	@Param		id	path	string	true	"App id"
//	@Success	204
//	@Router		/apps/{id} [delete]
func (h *Handlers) deleteApp(c *gin.Context) {
	a, ok := h.appAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if err := h.db.Delete(&models.WorkspaceApp{}, "id = ?", a.ID).Error; err != nil {
		h.internalError(c, err, "could not remove the app")
		return
	}
	h.publishWS(c, a.WorkspaceID, "apps")
	c.Status(http.StatusNoContent)
}

// orderApps sets the order of a workspace's tabs (editors).
//
//	@Summary	Reorder a workspace's apps
//	@Tags		apps
//	@Accept		json
//	@Security	BearerAuth
//	@Param		ws		path	string			true	"Workspace id"
//	@Param		body	body	orderRequest	true	"App ids in order"
//	@Success	204
//	@Router		/workspaces/{ws}/apps/order [put]
func (h *Handlers) orderApps(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req orderRequest
	if !bindJSON(c, &req) {
		return
	}
	for i, id := range req.IDs {
		h.db.Model(&models.WorkspaceApp{}).Where("id = ? AND workspace_id = ?", id, ws.ID).Update("position", i)
	}
	h.publishWS(c, ws.ID, "apps")
	c.Status(http.StatusNoContent)
}

type appCheckResponse struct {
	// Frameable: yes (it may be shown here), no (it refuses: its headers, or
	// it sends you to a site that isn't allowed) or unknown (Orbit couldn't
	// reach it — the browser may still).
	Frameable string `json:"frameable"`
	Why       string `json:"why,omitempty"` // no: headers | elsewhere
	Site      string `json:"site,omitempty"`
}

// checkApp asks the site whether it lets other sites show it (its
// X-Frame-Options and CSP frame-ancestors), so a refusal is said in words
// rather than an empty frame. Only addresses on the owner's allowed sites
// are fetched, redirects only while they stay on them; nothing is sent but
// a plain GET.
//
//	@Summary	Can an app be shown in a frame?
//	@Tags		apps
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"App id"
//	@Success	200	{object}	appCheckResponse
//	@Router		/apps/{id}/check [get]
func (h *Handlers) checkApp(c *gin.Context) {
	a, ok := h.appAccess(c, models.WSViewer)
	if !ok {
		return
	}
	if !embedAllowed(a.URL) {
		c.JSON(http.StatusOK, appCheckResponse{Frameable: "no", Why: "elsewhere"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
	defer cancel()
	c.JSON(http.StatusOK, probeFrame(ctx, a.URL, h.orbitOrigin(c)))
}

// errElsewhere stops a redirect off the allowed sites.
var errElsewhere = errors.New("redirected off the allowed sites")

var frameProbe = &http.Client{
	// The owner's own sites: often on Orbit's network (that's why they're
	// listed), so the public-only dialer doesn't apply; the list is the guard.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if !embedAllowed(req.URL.String()) {
			return errElsewhere
		}
		return nil
	},
}

func probeFrame(ctx context.Context, raw, orbit string) appCheckResponse {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return appCheckResponse{Frameable: "unknown"}
	}
	req.Header.Set("User-Agent", "Orbit (frame check)")
	res, err := frameProbe.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) && errors.Is(ue.Err, errElsewhere) {
			site := ""
			if u, perr := url.Parse(ue.URL); perr == nil {
				site = u.Scheme + "://" + u.Host
			}
			return appCheckResponse{Frameable: "no", Why: "elsewhere", Site: site}
		}
		return appCheckResponse{Frameable: "unknown"}
	}
	defer func() { _ = res.Body.Close() }()
	if framingAllowed(res.Header, res.Request.URL, orbit) {
		return appCheckResponse{Frameable: "yes"}
	}
	return appCheckResponse{Frameable: "no", Why: "headers"}
}

// framingAllowed reads a response's headers as a browser would for a frame
// on orbit's origin: CSP frame-ancestors when there is one (it wins), else
// X-Frame-Options.
func framingAllowed(hd http.Header, at *url.URL, orbit string) bool {
	seen := false
	for _, policy := range hd.Values("Content-Security-Policy") {
		for _, d := range strings.Split(policy, ";") {
			f := strings.Fields(d)
			if len(f) == 0 || !strings.EqualFold(f[0], "frame-ancestors") {
				continue
			}
			seen = true
			if !ancestorsAllow(f[1:], at, orbit) {
				return false
			}
		}
	}
	if seen {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(hd.Get("X-Frame-Options"))) {
	case "DENY":
		return false
	case "SAMEORIGIN":
		return sameOrigin(at, orbit)
	}
	return true
}

// ancestorsAllow tells whether orbit's origin is among frame-ancestors' sources.
func ancestorsAllow(sources []string, at *url.URL, orbit string) bool {
	o, err := url.Parse(orbit)
	if err != nil || o.Host == "" {
		return false
	}
	for _, s := range sources {
		ls := strings.ToLower(s)
		switch {
		case ls == "'none'":
			return false
		case ls == "*":
			return true
		case ls == "'self'":
			if sameOrigin(at, orbit) {
				return true
			}
		case strings.HasSuffix(ls, ":") && !strings.Contains(ls, "/"): // a scheme: "https:"
			if strings.TrimSuffix(ls, ":") == o.Scheme {
				return true
			}
		default:
			site := ls
			if !strings.Contains(site, "://") {
				site = o.Scheme + "://" + site
			}
			if i := strings.Index(site[strings.Index(site, "://")+3:], "/"); i >= 0 { // a path doesn't matter here
				site = site[:strings.Index(site, "://")+3+i]
			}
			if c := cleanEmbedSite(site); c != "" && siteMatches(c, o) {
				return true
			}
		}
	}
	return false
}

func sameOrigin(at *url.URL, orbit string) bool {
	o, err := url.Parse(orbit)
	return err == nil && at != nil && strings.EqualFold(at.Scheme, o.Scheme) && strings.EqualFold(at.Host, o.Host)
}

// orbitOrigin is the address people open Orbit at: PUBLIC_URL, else this
// request's.
func (h *Handlers) orbitOrigin(c *gin.Context) string {
	if h.cfg != nil && h.cfg.PublicURL != "" {
		if u, err := url.Parse(h.cfg.PublicURL); err == nil && u.Host != "" {
			return u.Scheme + "://" + u.Host
		}
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

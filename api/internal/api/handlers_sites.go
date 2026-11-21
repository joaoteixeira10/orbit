package api

import (
	"encoding/xml"
	"html"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/unicode/norm"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// A workspace published as a website: some of its pages (the ones its
// owners pick, and what's under them), read-only, open to anyone, at
// /site/<name> and, if set, at a domain of its own (the reverse proxy sends
// that domain to Orbit and holds its certificate). On its own domain only
// the site is served: no sign-in, no other part of Orbit. Search engines are
// asked not to list it unless its owners say they may (then a sitemap too).

var (
	siteSlugRe   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)
	siteDomainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,62}$`)
)

const maxSitePages = 50

// siteDomains maps a site's own domain to its name (cached; refreshed when a
// site changes, and every half minute for other replicas).
type siteDomains struct {
	mu sync.RWMutex
	m  map[string]string
	at time.Time
}

func (h *Handlers) siteOfHost(host string) string {
	host = strings.ToLower(host)
	if hp, _, err := net.SplitHostPort(host); err == nil {
		host = hp
	}
	h.sites.mu.RLock()
	fresh := h.sites.m != nil && time.Since(h.sites.at) < 30*time.Second
	slug := h.sites.m[host]
	h.sites.mu.RUnlock()
	if fresh {
		return slug
	}
	h.refreshSiteDomains()
	h.sites.mu.RLock()
	defer h.sites.mu.RUnlock()
	return h.sites.m[host]
}

func (h *Handlers) refreshSiteDomains() {
	var rows []models.Site
	h.db.Select("slug", "domain").Where("domain <> '' AND published = ?", true).Find(&rows)
	m := make(map[string]string, len(rows))
	for _, s := range rows {
		m[s.Domain] = s.Slug
	}
	h.sites.mu.Lock()
	h.sites.m, h.sites.at = m, time.Now()
	h.sites.mu.Unlock()
}

// siteHost, on a site's own domain, serves only that site: its pages (the
// app, in site mode), its API, the branding, robots.txt and its sitemap.
func (h *Handlers) siteHost(c *gin.Context) {
	slug := h.siteOfHost(c.Request.Host)
	if slug == "" {
		c.Next()
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	p := c.Request.URL.Path
	switch {
	case p == "/robots.txt":
		h.siteRobots(c, slug)
		c.Abort()
	case p == "/sitemap.xml":
		h.siteSitemapFor(c, slug, "https://"+strings.ToLower(hostOnly(c.Request.Host)))
		c.Abort()
	case p == "/api/v1/branding", p == "/api/v1/site/"+slug, strings.HasPrefix(p, "/api/v1/site/"+slug+"/"):
		c.Next()
	case c.FullPath() != "":
		// Any other part of Orbit isn't on this domain.
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		c.Set("site_host", slug) // the app's page, in site mode
		c.Next()
	}
}

func hostOnly(host string) string {
	if hp, _, err := net.SplitHostPort(host); err == nil {
		return hp
	}
	return host
}

// ---- The site, for its workspace's owners ----

type siteResponse struct {
	models.Site
	PageIDs []string `json:"page_ids"`
	Path    string   `json:"path"` // /site/<slug>
}

func toSiteResponse(s models.Site) siteResponse {
	return siteResponse{Site: s, PageIDs: decodeList(s.PageIDs), Path: "/site/" + s.Slug}
}

// getSite is the workspace's website, or null (owners).
//
//	@Summary	The workspace's website
//	@Tags		sites
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	siteResponse
//	@Router		/workspaces/{ws}/site [get]
func (h *Handlers) getSite(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var s models.Site
	if h.db.First(&s, "workspace_id = ?", ws.ID).Error != nil {
		c.JSON(http.StatusOK, gin.H{"site": nil, "suggested_slug": h.freeSlug(slugify(ws.Name), "")})
		return
	}
	c.JSON(http.StatusOK, gin.H{"site": toSiteResponse(s)})
}

type siteRequest struct {
	Published *bool     `json:"published"`
	Slug      *string   `json:"slug" binding:"omitempty,max=40"`
	Title     *string   `json:"title" binding:"omitempty,max=200"`
	Domain    *string   `json:"domain" binding:"omitempty,max=253"`
	PageIDs   *[]string `json:"page_ids"`
	Indexed   *bool     `json:"indexed"`
}

// putSite makes or changes the workspace's website (owners).
//
//	@Summary	Publish the workspace as a website, or change it
//	@Tags		sites
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		siteRequest	true	"What changes"
//	@Success	200		{object}	siteResponse
//	@Router		/workspaces/{ws}/site [put]
func (h *Handlers) putSite(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	if !feature.Enabled(h.db, feature.PublicSites) {
		c.JSON(http.StatusForbidden, gin.H{"error": "publishing websites is turned off on this instance"})
		return
	}
	var req siteRequest
	if !bindJSON(c, &req) {
		return
	}
	var s models.Site
	isNew := h.db.First(&s, "workspace_id = ?", ws.ID).Error != nil
	if isNew {
		s = models.Site{WorkspaceID: ws.ID, Slug: h.freeSlug(slugify(ws.Name), ""), CreatedBy: auth.UserID(c)}
	}
	bad := func(msg string) { c.JSON(http.StatusBadRequest, gin.H{"error": msg}) }
	if req.Slug != nil {
		slug := strings.ToLower(strings.TrimSpace(*req.Slug))
		if !siteSlugRe.MatchString(slug) {
			bad("the website's name: lower-case letters, digits and dashes (not at either end), up to 40")
			return
		}
		var n int64
		h.db.Model(&models.Site{}).Where("slug = ? AND workspace_id <> ?", slug, ws.ID).Count(&n)
		if n > 0 {
			bad("another website has that name")
			return
		}
		s.Slug = slug
	}
	if req.Title != nil {
		s.Title = strings.TrimSpace(*req.Title)
	}
	if req.Domain != nil {
		d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(*req.Domain)), ".")
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.TrimSuffix(d, "/")
		if d != "" {
			if !siteDomainRe.MatchString(d) || len(d) > 253 {
				bad("the domain: a name like docs.example.org (no https://, no path)")
				return
			}
			if d == h.ownHost(c) {
				bad("that's Orbit's own address: the website is there already, at /site/" + s.Slug)
				return
			}
			var n int64
			h.db.Model(&models.Site{}).Where("domain = ? AND workspace_id <> ?", d, ws.ID).Count(&n)
			if n > 0 {
				bad("another website has that domain")
				return
			}
		}
		s.Domain = d
	}
	if req.PageIDs != nil {
		ids := make([]string, 0, len(*req.PageIDs))
		seen := map[string]bool{}
		for _, id := range *req.PageIDs {
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) > maxSitePages {
			bad("up to " + strconv.Itoa(maxSitePages) + " pages (what's under each comes with it)")
			return
		}
		if len(ids) > 0 {
			var n int64
			h.db.Model(&models.Page{}).Where("id IN ? AND workspace_id = ? AND trashed_at IS NULL", ids, ws.ID).Count(&n)
			if int(n) != len(ids) {
				bad("a page isn't in this workspace (or is in the trash)")
				return
			}
		}
		s.PageIDs = encodeList(ids)
	}
	if req.Indexed != nil {
		s.Indexed = *req.Indexed
	}
	if req.Published != nil {
		s.Published = *req.Published
	}
	var err error
	if isNew {
		err = h.db.Create(&s).Error
	} else {
		err = h.db.Save(&s).Error
	}
	if err != nil {
		h.internalError(c, err, "could not save the website")
		return
	}
	h.refreshSiteDomains()
	h.recordCtx(c, "workspace.site", "workspace", ws.ID, gin.H{"slug": s.Slug, "domain": s.Domain, "published": s.Published, "indexed": s.Indexed})
	c.JSON(http.StatusOK, toSiteResponse(s))
}

// deleteSite takes the website down for good (owners).
//
//	@Summary	Take the workspace's website down
//	@Tags		sites
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	204
//	@Router		/workspaces/{ws}/site [delete]
func (h *Handlers) deleteSite(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	h.db.Where("workspace_id = ?", ws.ID).Delete(&models.Site{})
	h.refreshSiteDomains()
	h.recordCtx(c, "workspace.site_delete", "workspace", ws.ID, nil)
	c.Status(http.StatusNoContent)
}

// ownHost is the host Orbit itself answers on.
func (h *Handlers) ownHost(c *gin.Context) string {
	base := strings.TrimPrefix(strings.TrimPrefix(h.baseURL(c), "https://"), "http://")
	return strings.ToLower(hostOnly(strings.SplitN(base, "/", 2)[0]))
}

// freeSlug is base, or base-2, base-3… if taken (by another workspace).
func (h *Handlers) freeSlug(base, wsID string) string {
	if base == "" {
		base = "site"
	}
	for i := 1; i < 1000; i++ {
		s := base
		if i > 1 {
			suffix := "-" + strconv.Itoa(i)
			s = strings.TrimRight(base[:min(len(base), 40-len(suffix))], "-") + suffix
		}
		var n int64
		h.db.Model(&models.Site{}).Where("slug = ? AND workspace_id <> ?", s, wsID).Count(&n)
		if n == 0 {
			return s
		}
	}
	return base + "-" + strconv.FormatInt(time.Now().Unix(), 36)
}

// slugify turns a name into a website's name: "Équipe Ops" → "equipe-ops".
func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(name)) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue // accents
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 40 {
		cut := s[:40]
		if s[40] != '-' {
			if i := strings.LastIndexByte(cut, '-'); i > 0 {
				cut = cut[:i] // not in the middle of a word
			}
		}
		s = strings.Trim(cut, "-")
	}
	return s
}

// ---- The site, for anyone ----

type publicSite struct {
	site  models.Site
	ws    models.Workspace
	roots []string
	order []models.Page // every page it shows, its tree in order
	pages map[string]bool
}

// siteBySlug is a published site and the pages it shows (false: none).
func (h *Handlers) siteBySlug(slug string) (publicSite, bool) {
	var ps publicSite
	if slug == "" || !feature.Enabled(h.db, feature.PublicSites) {
		return ps, false
	}
	if h.db.First(&ps.site, "slug = ? AND published = ?", slug, true).Error != nil {
		return ps, false
	}
	if h.db.First(&ps.ws, "id = ?", ps.site.WorkspaceID).Error != nil || ps.ws.ArchivedAt != nil {
		return ps, false
	}
	var all []models.Page
	h.db.Select("id", "parent_id", "title", "icon", "rank", "restricted", "sealed", "template", "updated_at", "trashed_at").
		Where("workspace_id = ? AND trashed_at IS NULL", ps.ws.ID).Order("rank").Find(&all)
	byID := make(map[string]models.Page, len(all))
	kids := map[string][]models.Page{}
	for _, p := range all {
		byID[p.ID] = p
		if p.ParentID != nil {
			kids[*p.ParentID] = append(kids[*p.ParentID], p)
		}
	}
	// Never shown, nor what's under it: restricted, locked, a template.
	hidden := func(p models.Page) bool { return p.Restricted || p.Sealed || p.Template }
	ps.pages = map[string]bool{}
	var walk func(p models.Page)
	walk = func(p models.Page) {
		if hidden(p) || ps.pages[p.ID] || len(ps.order) >= 5000 {
			return
		}
		ps.pages[p.ID] = true
		ps.order = append(ps.order, p)
		for _, k := range kids[p.ID] {
			walk(k)
		}
	}
	for _, id := range decodeList(ps.site.PageIDs) {
		p, ok := byID[id]
		if !ok || hidden(p) || ps.pages[id] || h.underHidden(p, byID, hidden) {
			continue
		}
		ps.roots = append(ps.roots, id)
		walk(p)
	}
	return ps, true
}

// underHidden: is a page inside a restricted or locked one?
func (h *Handlers) underHidden(p models.Page, byID map[string]models.Page, hidden func(models.Page) bool) bool {
	for i := 0; p.ParentID != nil && i < 100; i++ {
		parent, ok := byID[*p.ParentID]
		if !ok {
			return false
		}
		if hidden(parent) {
			return true
		}
		p = parent
	}
	return false
}

// siteAccess finds the site in the address (404 if none) and says whether
// search engines may list what it answers.
func (h *Handlers) siteAccess(c *gin.Context) (publicSite, bool) {
	ps, ok := h.siteBySlug(c.Param("slug"))
	if !ok {
		c.Header("X-Robots-Tag", "noindex, nofollow")
		c.JSON(http.StatusNotFound, gin.H{"error": "there's no website here"})
		return ps, false
	}
	if !ps.site.Indexed {
		c.Header("X-Robots-Tag", "noindex, nofollow")
	}
	c.Header("Cache-Control", "no-cache")
	return ps, true
}

func (ps publicSite) title() string {
	if ps.site.Title != "" {
		return ps.site.Title
	}
	return ps.ws.Name
}

// siteOpen is a published site: its name, its first page and its pages.
//
//	@Summary	A published website (no account)
//	@Tags		sites
//	@Produce	json
//	@Param		slug	path		string	true	"The website's name"
//	@Success	200		{object}	map[string]any
//	@Router		/site/{slug} [get]
func (h *Handlers) siteOpen(c *gin.Context) {
	ps, ok := h.siteAccess(c)
	if !ok {
		return
	}
	pages := make([]guestPage, 0, len(ps.order))
	for _, p := range ps.order {
		pages = append(pages, toGuestPage(p, false))
	}
	home := ""
	if len(ps.roots) > 0 {
		home = ps.roots[0]
	}
	c.JSON(http.StatusOK, gin.H{"slug": ps.site.Slug, "title": ps.title(), "home": home, "roots": ps.roots, "pages": pages, "indexed": ps.site.Indexed})
}

// sitePage is one of a site's pages.
//
//	@Summary	A page of a published website
//	@Tags		sites
//	@Produce	json
//	@Param		slug	path		string	true	"The website's name"
//	@Param		id		path		string	true	"Page id"
//	@Success	200		{object}	guestPage
//	@Router		/site/{slug}/pages/{id} [get]
func (h *Handlers) sitePage(c *gin.Context) {
	ps, ok := h.siteAccess(c)
	if !ok {
		return
	}
	var p models.Page
	if !ps.pages[c.Param("id")] || h.db.First(&p, "id = ?", c.Param("id")).Error != nil || p.Sealed {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}
	c.JSON(http.StatusOK, toGuestPage(p, true))
}

type siteHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Icon    string `json:"icon"`
	Snippet string `json:"snippet"`
}

// siteSearch finds a site's pages by their title and words.
//
//	@Summary	Search a published website
//	@Tags		sites
//	@Produce	json
//	@Param		slug	path		string	true	"The website's name"
//	@Param		q		query		string	true	"Words to find"
//	@Success	200		{array}		siteHit
//	@Router		/site/{slug}/search [get]
func (h *Handlers) siteSearch(c *gin.Context) {
	ps, ok := h.siteAccess(c)
	if !ok {
		return
	}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	out := []siteHit{}
	if len([]rune(q)) < 2 || len(ps.pages) == 0 {
		c.JSON(http.StatusOK, out)
		return
	}
	if len(q) > 100 {
		q = q[:100]
	}
	ids := make([]string, 0, len(ps.pages))
	for id := range ps.pages {
		ids = append(ids, id)
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
	var found []models.Page
	h.db.Select("id", "title", "icon", "content_text").
		Where("id IN ? AND (LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(content_text) LIKE ? ESCAPE '\\')", ids, like, like).
		Limit(200).Find(&found)
	// Title matches first, then the others; each in the site's order.
	rank := make(map[string]int, len(ps.order))
	for i, p := range ps.order {
		rank[p.ID] = i
	}
	sort.SliceStable(found, func(i, j int) bool {
		ti, tj := strings.Contains(strings.ToLower(found[i].Title), q), strings.Contains(strings.ToLower(found[j].Title), q)
		if ti != tj {
			return ti
		}
		return rank[found[i].ID] < rank[found[j].ID]
	})
	for _, p := range found {
		if len(out) == 20 {
			break
		}
		out = append(out, siteHit{ID: p.ID, Title: p.Title, Icon: p.Icon, Snippet: snippetAround(p.ContentText, q)})
	}
	c.JSON(http.StatusOK, out)
}

// snippetAround is a few words of text around the first q in it.
func snippetAround(text, q string) string {
	flat := strings.Join(strings.Fields(text), " ")
	i := strings.Index(strings.ToLower(flat), q)
	if i < 0 {
		return clip(flat, 160)
	}
	from := max(0, i-60)
	for from > 0 && flat[from-1] != ' ' {
		from-- // from the start of a word
	}
	s := flat[from:]
	if from > 0 {
		s = "…" + s
	}
	return clip(s, 160)
}

// siteEmbed is a sheet, diagram or whiteboard shown in one of a site's pages.
//
//	@Summary	What a website's page shows
//	@Tags		sites
//	@Produce	json
//	@Param		slug	path		string	true	"The website's name"
//	@Param		kind	path		string	true	"sheet, diagram or board"
//	@Param		id		path		string	true	"Its id"
//	@Success	200		{object}	map[string]any
//	@Router		/site/{slug}/embed/{kind}/{id} [get]
func (h *Handlers) siteEmbed(c *gin.Context) {
	ps, ok := h.siteAccess(c)
	if !ok {
		return
	}
	h.serveEmbed(c, guestShare{ws: ps.ws, pages: ps.pages})
}

// siteUpload is a picture or file of one of a site's pages.
//
//	@Summary	A website's file
//	@Tags		sites
//	@Param		slug	path	string	true	"The website's name"
//	@Param		id		path	string	true	"Upload id"
//	@Success	200
//	@Router		/site/{slug}/uploads/{id}/{name} [get]
func (h *Handlers) siteUpload(c *gin.Context) {
	ps, ok := h.siteAccess(c)
	if !ok {
		return
	}
	h.serveShared(c, guestShare{ws: ps.ws, pages: ps.pages}, "public, max-age=3600")
}

// ---- For search engines ----

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// siteSitemap is a listed site's sitemap at Orbit's address.
func (h *Handlers) siteSitemap(c *gin.Context) {
	h.siteSitemapFor(c, c.Param("slug"), strings.TrimRight(h.baseURL(c), "/")+"/site/"+c.Param("slug"))
}

// siteSitemapFor lists a site's pages under base (only when it may be listed).
func (h *Handlers) siteSitemapFor(c *gin.Context, slug, base string) {
	ps, ok := h.siteBySlug(slug)
	if !ok || !ps.site.Indexed {
		c.String(http.StatusNotFound, "not found")
		return
	}
	set := sitemapSet{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for i, p := range ps.order {
		loc := base + "/p/" + p.ID
		if i == 0 {
			loc = base + "/"
		}
		set.URLs = append(set.URLs, sitemapURL{Loc: loc, LastMod: p.UpdatedAt.UTC().Format("2006-01-02")})
	}
	b, _ := xml.MarshalIndent(set, "", "  ")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Data(http.StatusOK, "application/xml; charset=utf-8", append([]byte(xml.Header), b...))
}

// siteRobots is robots.txt on a site's own domain.
func (h *Handlers) siteRobots(c *gin.Context, slug string) {
	ps, ok := h.siteBySlug(slug)
	c.Header("Cache-Control", "public, max-age=3600")
	if !ok || !ps.site.Indexed {
		c.String(http.StatusOK, "User-agent: *\nDisallow: /\n")
		return
	}
	c.String(http.StatusOK, "User-agent: *\nAllow: /\nSitemap: https://"+strings.ToLower(hostOnly(c.Request.Host))+"/sitemap.xml\n")
}

// siteTags are the page's head tags for a site's page: its title and words
// for link previews, and whether search engines may list it.
func (h *Handlers) siteTags(slug, pageID string) (title, desc string, indexed, ok bool) {
	ps, found := h.siteBySlug(slug)
	if !found {
		return "", "", false, false
	}
	if pageID == "" && len(ps.roots) > 0 {
		pageID = ps.roots[0]
	}
	title = ps.title()
	if ps.pages[pageID] {
		var p models.Page
		if h.db.Select("title", "content_text").First(&p, "id = ?", pageID).Error == nil {
			if p.Title != "" && p.Title != title {
				title = p.Title + " · " + title
			}
			desc = clip(strings.Join(strings.Fields(p.ContentText), " "), 200)
		}
	}
	return title, desc, ps.site.Indexed, true
}

// siteHead is what goes in the app's page head on a site: its name (for the
// app, on its own domain), title, description, and robots.
func (h *Handlers) siteHead(c *gin.Context) string {
	p := c.Request.URL.Path
	slug, onDomain := c.Get("site_host")
	page := ""
	var s string
	if onDomain {
		s, _ = slug.(string)
		page = strings.TrimPrefix(p, "/p/")
		if page == p {
			page = ""
		}
	} else if rest, ok := strings.CutPrefix(p, "/site/"); ok {
		parts := strings.Split(strings.Trim(rest, "/"), "/")
		s = parts[0]
		if len(parts) >= 3 && parts[1] == "p" {
			page = parts[2]
		}
	} else {
		return ""
	}
	title, desc, indexed, ok := h.siteTags(s, page)
	if !ok {
		return `<meta name="robots" content="noindex, nofollow" />`
	}
	tags := []string{`<title>` + html.EscapeString(title) + `</title>`}
	if desc != "" {
		tags = append(tags, `<meta name="description" content="`+html.EscapeString(desc)+`" />`)
	}
	if !indexed {
		tags = append(tags, `<meta name="robots" content="noindex, nofollow" />`)
	}
	if onDomain {
		tags = append(tags, `<meta name="orbit-site" content="`+html.EscapeString(s)+`" />`)
	}
	return strings.Join(tags, "\n    ")
}

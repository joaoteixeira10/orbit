package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"mime"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Guest links: a page (and, if chosen, the pages under it) open to anyone
// with the link — no account — to read, or also to edit. The workspace's
// editors make and take back links (Share in a page's menu); the owner can
// turn guest links off for the whole instance. A guest sees the pages, the
// sheets, diagrams and whiteboards shown in them, and their pictures and
// files; nothing else of the workspace.

// newLinkSecret makes a link's secret: what goes in the address, its hash
// (to find the link) and the secret sealed (to show the link again).
func (h *Handlers) newLinkSecret() (plain, hash, sealed string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hash = linkHash(plain)
	s, err := h.sealer.Seal([]byte(plain))
	if err != nil {
		return
	}
	return plain, hash, base64.StdEncoding.EncodeToString(s), nil
}

func linkHash(secret string) string {
	s := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(s[:])
}

// unsealLink is a link's secret again ("" if it can't be read).
func (h *Handlers) unsealLink(sealed string) string {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return ""
	}
	p, err := h.sealer.Unseal(raw)
	if err != nil {
		return ""
	}
	return string(p)
}

type shareLinkResponse struct {
	models.ShareLink
	URL string `json:"url"`
}

func (h *Handlers) shareOut(c *gin.Context, l models.ShareLink) shareLinkResponse {
	return shareLinkResponse{ShareLink: l, URL: h.baseURL(c) + "/shared/" + h.unsealLink(l.TokenSealed)}
}

// listShareLinks lists a page's guest links (its members).
//
//	@Summary	A page's guest links
//	@Tags		sharing
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Page id"
//	@Success	200	{array}	shareLinkResponse
//	@Router		/pages/{id}/links [get]
func (h *Handlers) listShareLinks(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var ls []models.ShareLink
	h.db.Where("page_id = ?", p.ID).Order("created_at").Find(&ls)
	out := make([]shareLinkResponse, 0, len(ls))
	for _, l := range ls {
		out = append(out, h.shareOut(c, l))
	}
	c.JSON(http.StatusOK, out)
}

type shareLinkRequest struct {
	Mode          *string `json:"mode" binding:"omitempty,oneof=read edit"`
	Subpages      *bool   `json:"subpages"`
	ExpiresInDays *int    `json:"expires_in_days" binding:"omitempty,min=0,max=3650"` // 0: never
}

// createShareLink makes a guest link to a page (editors).
//
//	@Summary	New guest link
//	@Tags		sharing
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Page id"
//	@Param		body	body		shareLinkRequest	true	"What it opens"
//	@Success	201		{object}	shareLinkResponse
//	@Router		/pages/{id}/links [post]
func (h *Handlers) createShareLink(c *gin.Context) {
	if !feature.Enabled(h.db, feature.GuestLinks) {
		c.JSON(http.StatusForbidden, gin.H{"error": "guest links are off on this Orbit"})
		return
	}
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req shareLinkRequest
	if !bindJSON(c, &req) {
		return
	}
	plain, hash, sealed, err := h.newLinkSecret()
	if err != nil || plain == "" {
		h.internalError(c, err, "could not make the link")
		return
	}
	l := models.ShareLink{WorkspaceID: p.WorkspaceID, PageID: p.ID, Mode: "read", Subpages: true, TokenHash: hash, TokenSealed: sealed, CreatedBy: auth.UserID(c)}
	applyShare(&l, req)
	if err := h.db.Create(&l).Error; err != nil {
		h.internalError(c, err, "could not make the link")
		return
	}
	h.recordCtx(c, "page.link_create", "page", p.ID, gin.H{"mode": l.Mode, "subpages": l.Subpages})
	c.JSON(http.StatusCreated, h.shareOut(c, l))
}

func applyShare(l *models.ShareLink, req shareLinkRequest) {
	if req.Mode != nil {
		l.Mode = *req.Mode
	}
	if req.Subpages != nil {
		l.Subpages = *req.Subpages
	}
	if req.ExpiresInDays != nil {
		if *req.ExpiresInDays == 0 {
			l.ExpiresAt = nil
		} else {
			at := time.Now().AddDate(0, 0, *req.ExpiresInDays)
			l.ExpiresAt = &at
		}
	}
}

func (h *Handlers) linkAccess(c *gin.Context) (models.ShareLink, bool) {
	var l models.ShareLink
	if h.db.First(&l, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return l, false
	}
	_, _, ok := h.wsAccess(c, l.WorkspaceID, models.WSEditor)
	return l, ok
}

// updateShareLink changes what a guest link opens (editors).
//
//	@Summary	Change a guest link
//	@Tags		sharing
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Link id"
//	@Param		body	body		shareLinkRequest	true	"What changes"
//	@Success	200		{object}	shareLinkResponse
//	@Router		/links/{id} [put]
func (h *Handlers) updateShareLink(c *gin.Context) {
	l, ok := h.linkAccess(c)
	if !ok {
		return
	}
	var req shareLinkRequest
	if !bindJSON(c, &req) {
		return
	}
	applyShare(&l, req)
	if err := h.db.Model(&models.ShareLink{}).Where("id = ?", l.ID).Updates(map[string]any{"mode": l.Mode, "subpages": l.Subpages, "expires_at": l.ExpiresAt}).Error; err != nil {
		h.internalError(c, err, "could not change the link")
		return
	}
	c.JSON(http.StatusOK, h.shareOut(c, l))
}

// deleteShareLink takes a guest link back (editors): it stops working.
//
//	@Summary	Delete a guest link
//	@Tags		sharing
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Link id"
//	@Success	204
//	@Router		/links/{id} [delete]
func (h *Handlers) deleteShareLink(c *gin.Context) {
	l, ok := h.linkAccess(c)
	if !ok {
		return
	}
	h.db.Delete(&l)
	h.recordCtx(c, "page.link_delete", "page", l.PageID, nil)
	c.Status(http.StatusNoContent)
}

// ---- The guest's side (no account) ----

type guestShare struct {
	link  models.ShareLink
	root  models.Page
	ws    models.Workspace
	pages map[string]bool // the pages it opens
}

// guestAccess finds the link in the address and what it opens; a link
// that's gone, expired or off is "not found".
// shareOf finds what a guest link opens — its link, page and workspace — or
// false when it opens nothing (guest links off, taken back, expired, its page
// gone). It changes nothing (a link preview uses it too).
func (h *Handlers) shareOf(tok string) (guestShare, bool) {
	var g guestShare
	if tok == "" || !feature.Enabled(h.db, feature.GuestLinks) {
		return g, false
	}
	if h.db.First(&g.link, "token_hash = ?", linkHash(tok)).Error != nil {
		return g, false
	}
	if g.link.ExpiresAt != nil && time.Now().After(*g.link.ExpiresAt) {
		return g, false
	}
	if h.db.First(&g.root, "id = ? AND trashed_at IS NULL", g.link.PageID).Error != nil || h.db.First(&g.ws, "id = ?", g.link.WorkspaceID).Error != nil {
		return g, false
	}
	return g, true
}

func (h *Handlers) guestAccess(c *gin.Context) (guestShare, bool) {
	c.Header("X-Robots-Tag", "noindex, nofollow")
	c.Header("Cache-Control", "no-store")
	var g guestShare
	gone := func() (guestShare, bool) {
		c.JSON(http.StatusNotFound, gin.H{"error": "this link doesn't open anything (taken back, expired, or mistyped)"})
		return g, false
	}
	g, ok := h.shareOf(c.Param("token"))
	if !ok {
		return gone()
	}
	g.pages = map[string]bool{g.root.ID: true}
	if g.link.Subpages {
		var ids []string
		under := descendants(h.db, g.root.ID)
		if len(under) > 0 {
			h.db.Model(&models.Page{}).Where("id IN ? AND trashed_at IS NULL", under).Pluck("id", &ids)
		}
		for _, id := range ids {
			g.pages[id] = true
		}
	}
	now := time.Now()
	if g.link.LastUsedAt == nil || now.Sub(*g.link.LastUsedAt) > time.Minute {
		h.db.Model(&models.ShareLink{}).Where("id = ?", g.link.ID).Update("last_used_at", &now)
	}
	return g, true
}

func (g guestShare) canEdit() bool { return g.link.Mode == "edit" && g.ws.ArchivedAt == nil }

type guestPage struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parent_id"`
	Title     string  `json:"title"`
	Icon      string  `json:"icon"`
	Cover     string  `json:"cover,omitempty"`
	Content   string  `json:"content,omitempty"`
	Version   int     `json:"version,omitempty"`
	UpdatedAt string  `json:"updated_at"`
}

func toGuestPage(p models.Page, full bool) guestPage {
	g := guestPage{ID: p.ID, ParentID: p.ParentID, Title: p.Title, Icon: p.Icon, Cover: p.Cover, UpdatedAt: stamp(p.UpdatedAt)}
	if full {
		g.Content, g.Version = p.Content, p.Version
	}
	return g
}

// guestShared opens a guest link: what it is, its first page, and the
// pages under it it opens.
//
//	@Summary	A guest link (no account)
//	@Tags		sharing
//	@Produce	json
//	@Param		token	path		string	true	"The link's secret"
//	@Success	200		{object}	map[string]any
//	@Router		/shared/{token} [get]
func (h *Handlers) guestShared(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	tree := []guestPage{}
	if len(g.pages) > 1 {
		ids := make([]string, 0, len(g.pages))
		for id := range g.pages {
			ids = append(ids, id)
		}
		var ps []models.Page
		h.db.Select("id", "parent_id", "title", "icon", "rank", "updated_at").Where("id IN ?", ids).Order("rank").Find(&ps)
		for _, p := range ps {
			tree = append(tree, toGuestPage(p, false))
		}
	}
	c.JSON(http.StatusOK, gin.H{"mode": g.link.Mode, "can_edit": g.canEdit(), "workspace": g.ws.Name, "root": g.root.ID, "page": toGuestPage(g.root, true), "pages": tree})
}

// guestPageOf is one of the pages a guest link opens.
//
//	@Summary	A page through a guest link
//	@Tags		sharing
//	@Produce	json
//	@Param		token	path		string	true	"The link's secret"
//	@Param		id		path		string	true	"Page id"
//	@Success	200		{object}	guestPage
//	@Router		/shared/{token}/pages/{id} [get]
func (h *Handlers) guestPageOf(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	var p models.Page
	if !g.pages[c.Param("id")] || h.db.First(&p, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}
	c.JSON(http.StatusOK, toGuestPage(p, true))
}

type guestEditRequest struct {
	Title   *string `json:"title" binding:"omitempty,max=300"`
	Content *string `json:"content"`
	Version int     `json:"version"`
}

// guestEdit saves a guest's change to a page their link lets them edit
// (with the version they started from: someone else's newer save wins).
//
//	@Summary	Edit a page through a guest link
//	@Tags		sharing
//	@Accept		json
//	@Produce	json
//	@Param		token	path		string				true	"The link's secret"
//	@Param		id		path		string				true	"Page id"
//	@Param		body	body		guestEditRequest	true	"What changes"
//	@Success	200		{object}	guestPage
//	@Router		/shared/{token}/pages/{id} [put]
func (h *Handlers) guestEdit(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	if !g.canEdit() {
		c.JSON(http.StatusForbidden, gin.H{"error": "this link is to read only"})
		return
	}
	var p models.Page
	if !g.pages[c.Param("id")] || h.db.First(&p, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}
	var req guestEditRequest
	if !bindJSON(c, &req) {
		return
	}
	prev := p
	up := map[string]any{"updated_by": ""}
	if req.Title != nil {
		up["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Content != nil {
		md := planner.PageMarkdown(*req.Content)
		if err := planner.CheckDoc(md); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		up["content"], up["content_text"], up["version"] = md, planner.DocText(md), gorm.Expr("version + 1")
	}
	q := h.db.Model(&models.Page{}).Where("id = ?", p.ID)
	if req.Content != nil {
		q = q.Where("version = ?", req.Version)
	}
	res := q.Updates(up)
	if res.Error != nil {
		h.internalError(c, res.Error, "could not save the page")
		return
	}
	p = reloadPage(h, p.ID)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "someone else saved this page since you opened it", "page": toGuestPage(p, true)})
		return
	}
	recordVersion(h.db, p, "", &prev, 0)
	h.collabSaved(c, "page:"+p.ID, p.Version)
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusOK, toGuestPage(p, true))
}

// guestReferenced: is the thing (by its address part and id) shown in one
// of the pages the link opens?
func (h *Handlers) guestReferenced(g guestShare, seg, id string) bool {
	ids := make([]string, 0, len(g.pages))
	for x := range g.pages {
		ids = append(ids, x)
	}
	var n int64
	h.db.Model(&models.Page{}).Where("id IN ? AND content LIKE ?", ids, "%/w/"+g.ws.ID+"/"+seg+"/"+id+"%").Count(&n)
	return n > 0
}

// guestEmbed is a sheet, a diagram or a whiteboard shown in a page the link
// opens (read only: its picture, or a sheet's cells).
//
//	@Summary	What a shared page shows (guest link)
//	@Tags		sharing
//	@Produce	json
//	@Param		token	path		string	true	"The link's secret"
//	@Param		kind	path		string	true	"sheet, diagram or board"
//	@Param		id		path		string	true	"Its id"
//	@Success	200		{object}	map[string]any
//	@Router		/shared/{token}/embed/{kind}/{id} [get]
func (h *Handlers) guestEmbed(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	id := c.Param("id")
	notFound := func() { c.JSON(http.StatusNotFound, gin.H{"error": "not found"}) }
	switch c.Param("kind") {
	case "sheet":
		var s models.Sheet
		if !h.guestReferenced(g, "s", id) || h.db.First(&s, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			notFound()
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": s.ID, "title": s.Title, "data": rawJSON(s.Data)})
	case "diagram":
		var d models.Diagram
		if !h.guestReferenced(g, "d", id) || h.db.First(&d, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			notFound()
			return
		}
		out := gin.H{"id": d.ID, "title": d.Title, "kind": d.Kind, "preview": d.Preview}
		if d.Kind == models.DiagramMermaid {
			out["data"] = d.Data // drawn in the browser when there's no picture yet
		}
		c.JSON(http.StatusOK, out)
	case "board":
		var d models.Drawing
		if !h.guestReferenced(g, "whiteboards", id) || h.db.First(&d, "id = ? AND workspace_id = ?", id, g.ws.ID).Error != nil {
			notFound()
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": d.ID, "title": d.Title, "preview": d.Preview})
	default:
		notFound()
	}
}

// guestUpload is a picture or a file of a page the link opens.
//
//	@Summary	A shared page's file (guest link)
//	@Tags		sharing
//	@Param		token	path	string	true	"The link's secret"
//	@Param		id		path	string	true	"Upload id"
//	@Success	200
//	@Router		/shared/{token}/uploads/{id}/{name} [get]
func (h *Handlers) guestUpload(c *gin.Context) {
	g, ok := h.guestAccess(c)
	if !ok {
		return
	}
	var u models.Upload
	if h.db.First(&u, "id = ?", c.Param("id")).Error != nil || u.Target != uploads.Page || !g.pages[u.TargetID] {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	data, err := uploads.Read(h.db, u)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	disp, typ := "attachment", "application/octet-stream"
	if uploads.Inline(u.Type) {
		disp, typ = "inline", u.Type
	}
	head := c.Writer.Header()
	head.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": u.Name}))
	head.Set("X-Content-Type-Options", "nosniff")
	head.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	head.Set("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, typ, data)
}

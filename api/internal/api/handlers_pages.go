package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// pageSummary is a page without its document, for the sidebar tree.
type pageSummary struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	ParentID    *string    `json:"parent_id"`
	Title       string     `json:"title"`
	Icon        string     `json:"icon"`
	Rank        float64    `json:"rank"`
	UpdatedAt   time.Time  `json:"updated_at"`
	TrashedAt   *time.Time `json:"trashed_at,omitempty"`
	Favorite    bool       `json:"favorite"`
}

// pageResponse is a full page plus what the caller may do with it.
type pageResponse struct {
	models.Page
	Favorite bool `json:"favorite"`
	CanEdit  bool `json:"can_edit"`
}

func (h *Handlers) favoriteSet(uid string) map[string]bool {
	var ids []string
	h.db.Model(&models.PageFavorite{}).Where("user_id = ?", uid).Pluck("page_id", &ids)
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func summarize(p models.Page, fav map[string]bool) pageSummary {
	return pageSummary{ID: p.ID, WorkspaceID: p.WorkspaceID, ParentID: p.ParentID, Title: p.Title, Icon: p.Icon, Rank: p.Rank, UpdatedAt: p.UpdatedAt, TrashedAt: p.TrashedAt, Favorite: fav[p.ID]}
}

// pageAccess loads a page and checks the caller's role in its workspace.
func (h *Handlers) pageAccess(c *gin.Context, need string) (models.Page, models.WorkspaceMember, bool) {
	var p models.Page
	if err := h.db.First(&p, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return p, models.WorkspaceMember{}, false
	}
	_, m, ok := h.wsAccess(c, p.WorkspaceID, need)
	return p, m, ok
}

// listPages returns a workspace's page tree (no documents).
//
//	@Summary	Pages of a workspace
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	pageSummary
//	@Router		/workspaces/{ws}/pages [get]
func (h *Handlers) listPages(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var pages []models.Page
	h.db.Omit("content", "content_text").Where("workspace_id = ? AND trashed_at IS NULL", ws.ID).Order("rank").Find(&pages)
	fav := h.favoriteSet(auth.UserID(c))
	out := make([]pageSummary, 0, len(pages))
	for _, p := range pages {
		out = append(out, summarize(p, fav))
	}
	c.JSON(http.StatusOK, out)
}

// listTrash returns a workspace's trashed pages.
//
//	@Summary	Trashed pages
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	pageSummary
//	@Router		/workspaces/{ws}/trash [get]
func (h *Handlers) listTrash(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var pages []models.Page
	h.db.Omit("content", "content_text").Where("workspace_id = ? AND trashed_at IS NOT NULL", ws.ID).Order("trashed_at DESC").Find(&pages)
	out := make([]pageSummary, 0, len(pages))
	for _, p := range pages {
		out = append(out, summarize(p, nil))
	}
	c.JSON(http.StatusOK, out)
}

type createPageRequest struct {
	ParentID *string `json:"parent_id"`
	Title    string  `json:"title" binding:"max=300"`
	Icon     string  `json:"icon" binding:"max=64"`
	Content  string  `json:"content"`
	// AfterID puts the page right after this sibling ("" = at the end).
	AfterID string `json:"after_id"`
}

// siblingRank returns a rank among the children of parent in ws: right after
// afterID, or after the last one.
func (h *Handlers) siblingRank(tx *gorm.DB, wsID string, parent *string, afterID, exclude string) float64 {
	q := tx.Model(&models.Page{}).Where("workspace_id = ? AND trashed_at IS NULL AND id <> ?", wsID, exclude)
	if parent == nil {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", *parent)
	}
	var sibs []struct {
		ID   string
		Rank float64
	}
	q.Select("id", "rank").Order("rank").Find(&sibs)
	ranks := make([]float64, len(sibs))
	ids := make([]string, len(sibs))
	for i, s := range sibs {
		ranks[i], ids[i] = s.Rank, s.ID
	}
	if afterID == "" {
		if len(ranks) == 0 {
			return planner.Rank(nil, nil)
		}
		return planner.Rank(&ranks[len(ranks)-1], nil)
	}
	if afterID == "^" { // the first place
		if len(ranks) == 0 {
			return planner.Rank(nil, nil)
		}
		return planner.Rank(nil, &ranks[0])
	}
	for i, id := range ids {
		if id == afterID {
			if i+1 < len(ranks) {
				return planner.Rank(&ranks[i], &ranks[i+1])
			}
			return planner.Rank(&ranks[i], nil)
		}
	}
	if len(ranks) == 0 {
		return planner.Rank(nil, nil)
	}
	return planner.Rank(&ranks[len(ranks)-1], nil)
}

// checkParent verifies a parent page is a live page of the same workspace.
func (h *Handlers) checkParent(c *gin.Context, wsID string, parent *string) bool {
	if parent == nil {
		return true
	}
	var n int64
	h.db.Model(&models.Page{}).Where("id = ? AND workspace_id = ? AND trashed_at IS NULL", *parent, wsID).Count(&n)
	if n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the parent page isn't in this workspace"})
		return false
	}
	return true
}

// createPage adds a page (editors).
//
//	@Summary	Create a page
//	@Tags		pages
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		createPageRequest	true	"Page"
//	@Success	201		{object}	pageResponse
//	@Router		/workspaces/{ws}/pages [post]
func (h *Handlers) createPage(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req createPageRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.ParentID != nil && *req.ParentID == "" {
		req.ParentID = nil
	}
	if !h.checkParent(c, ws.ID, req.ParentID) {
		return
	}
	if err := planner.CheckDoc(req.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	uid := auth.UserID(c)
	p := models.Page{
		WorkspaceID: ws.ID, ParentID: req.ParentID, Title: strings.TrimSpace(req.Title), Icon: req.Icon,
		Content: req.Content, ContentText: planner.DocText(req.Content),
		Rank: h.siblingRank(h.db, ws.ID, req.ParentID, req.AfterID, ""), CreatedBy: uid, UpdatedBy: uid,
	}
	if err := h.db.Create(&p).Error; err != nil {
		h.internalError(c, err, "could not create the page")
		return
	}
	h.publishWS(c, ws.ID, "pages")
	c.JSON(http.StatusCreated, pageResponse{Page: p, CanEdit: true})
}

// getPage returns a page with its document.
//
//	@Summary	Get a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Page id"
//	@Success	200	{object}	pageResponse
//	@Router		/pages/{id} [get]
func (h *Handlers) getPage(c *gin.Context) {
	p, m, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var n int64
	h.db.Model(&models.PageFavorite{}).Where("user_id = ? AND page_id = ?", auth.UserID(c), p.ID).Count(&n)
	canEdit := roleRank(m.Role) >= roleRank(models.WSEditor) && !auth.Effective(c).ReadOnly && p.TrashedAt == nil
	c.JSON(http.StatusOK, pageResponse{Page: p, Favorite: n > 0, CanEdit: canEdit})
}

type updatePageRequest struct {
	Title   *string `json:"title" binding:"omitempty,max=300"`
	Icon    *string `json:"icon" binding:"omitempty,max=64"`
	Cover   *string `json:"cover" binding:"omitempty,max=64"`
	Content *string `json:"content"`
	// Version is the version the editor started from; required with content.
	// A newer save by someone else makes this a 409 with the current page.
	Version int `json:"version"`
}

// updatePage saves a page (editors). Saving the document needs the version
// it was edited from, so a stale editor never overwrites someone's newer save.
//
//	@Summary	Save a page
//	@Tags		pages
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Page id"
//	@Param		body	body		updatePageRequest	true	"Fields to change"
//	@Success	200		{object}	pageResponse
//	@Failure	409		{object}	map[string]any
//	@Router		/pages/{id} [put]
func (h *Handlers) updatePage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if p.TrashedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the page is in the trash; restore it first"})
		return
	}
	var req updatePageRequest
	if !bindJSON(c, &req) {
		return
	}
	up := map[string]any{"updated_by": auth.UserID(c)}
	if req.Title != nil {
		up["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Icon != nil {
		up["icon"] = *req.Icon
	}
	if req.Cover != nil {
		up["cover"] = *req.Cover
	}
	if req.Content != nil {
		if err := planner.CheckDoc(*req.Content); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		up["content"] = *req.Content
		up["content_text"] = planner.DocText(*req.Content)
		up["version"] = gorm.Expr("version + 1")
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
		c.JSON(http.StatusConflict, gin.H{"error": "someone else saved this page since you opened it", "page": pageResponse{Page: p, CanEdit: true}})
		return
	}
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusOK, pageResponse{Page: p, CanEdit: true})
}

type movePageRequest struct {
	ParentID *string `json:"parent_id"` // null = top level
	AfterID  string  `json:"after_id"`  // sibling to follow; "^" = first; "" = last
}

// movePage moves a page in the tree (editors). A page can't move under itself.
//
//	@Summary	Move a page
//	@Tags		pages
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Page id"
//	@Param		body	body		movePageRequest	true	"New place"
//	@Success	200		{object}	pageSummary
//	@Router		/pages/{id}/move [post]
func (h *Handlers) movePage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req movePageRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.ParentID != nil && *req.ParentID == "" {
		req.ParentID = nil
	}
	if !h.checkParent(c, p.WorkspaceID, req.ParentID) {
		return
	}
	// Walk up from the new parent: meeting the page itself would make a loop.
	for cur := req.ParentID; cur != nil; {
		if *cur == p.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a page can't go inside itself"})
			return
		}
		var up models.Page
		if h.db.Select("id", "parent_id").First(&up, "id = ?", *cur).Error != nil {
			break
		}
		cur = up.ParentID
	}
	rank := h.siblingRank(h.db, p.WorkspaceID, req.ParentID, req.AfterID, p.ID)
	h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(map[string]any{"parent_id": req.ParentID, "rank": rank})
	p = reloadPage(h, p.ID)
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusOK, summarize(p, nil))
}

// descendants returns the ids of a page's sub-pages, at every depth.
func descendants(db *gorm.DB, id string) []string {
	var out []string
	frontier := []string{id}
	for len(frontier) > 0 && len(out) < 10000 {
		var next []string
		db.Model(&models.Page{}).Where("parent_id IN ?", frontier).Pluck("id", &next)
		out = append(out, next...)
		frontier = next
	}
	return out
}

// trashPage moves a page and its sub-pages to the trash (editors), or, with
// ?permanent=1 on a page already there, deletes them for good.
//
//	@Summary	Trash (or delete) a page
//	@Tags		pages
//	@Security	BearerAuth
//	@Param		id			path	string	true	"Page id"
//	@Param		permanent	query	bool	false	"Delete a trashed page for good"
//	@Success	204
//	@Router		/pages/{id} [delete]
func (h *Handlers) trashPage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	ids := append(descendants(h.db, p.ID), p.ID)
	if c.Query("permanent") == "1" {
		if p.TrashedAt == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "move the page to the trash first"})
			return
		}
		h.db.Where("page_id IN ?", ids).Delete(&models.PageFavorite{})
		h.db.Where("id IN ?", ids).Delete(&models.Page{})
	} else {
		now := time.Now()
		h.db.Model(&models.Page{}).Where("id IN ? AND trashed_at IS NULL", ids).Update("trashed_at", &now)
	}
	h.publishWS(c, p.WorkspaceID, "pages")
	c.Status(http.StatusNoContent)
}

// restorePage takes a page and what was trashed with it out of the trash. A
// page whose parent is still in the trash comes back at the top level.
//
//	@Summary	Restore a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Page id"
//	@Success	200	{object}	pageSummary
//	@Router		/pages/{id}/restore [post]
func (h *Handlers) restorePage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if p.TrashedAt == nil {
		c.JSON(http.StatusOK, summarize(p, nil))
		return
	}
	ids := descendants(h.db, p.ID)
	h.db.Model(&models.Page{}).Where("id IN ? AND trashed_at = ?", ids, p.TrashedAt).Update("trashed_at", nil)
	up := map[string]any{"trashed_at": nil}
	if p.ParentID != nil {
		var parent models.Page
		if h.db.Select("id", "trashed_at").First(&parent, "id = ?", *p.ParentID).Error != nil || parent.TrashedAt != nil {
			up["parent_id"] = nil
		}
	}
	h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(up)
	p = reloadPage(h, p.ID)
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusOK, summarize(p, nil))
}

// duplicatePage copies a page (not its sub-pages) right after itself.
//
//	@Summary	Duplicate a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Page id"
//	@Success	201	{object}	pageResponse
//	@Router		/pages/{id}/duplicate [post]
func (h *Handlers) duplicatePage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSEditor)
	if !ok {
		return
	}
	uid := auth.UserID(c)
	cp := models.Page{
		WorkspaceID: p.WorkspaceID, ParentID: p.ParentID, Title: p.Title + " (copy)", Icon: p.Icon, Cover: p.Cover,
		Content: p.Content, ContentText: p.ContentText, CreatedBy: uid, UpdatedBy: uid,
		Rank: h.siblingRank(h.db, p.WorkspaceID, p.ParentID, p.ID, ""),
	}
	if err := h.db.Create(&cp).Error; err != nil {
		h.internalError(c, err, "could not duplicate the page")
		return
	}
	h.publishWS(c, p.WorkspaceID, "pages")
	c.JSON(http.StatusCreated, pageResponse{Page: cp, CanEdit: true})
}

// favoritePage adds (POST) or removes (DELETE) a page from my Favorites.
//
//	@Summary	Favourite a page
//	@Tags		pages
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Page id"
//	@Success	204
//	@Router		/pages/{id}/favorite [post]
func (h *Handlers) favoritePage(c *gin.Context) {
	p, _, ok := h.pageAccess(c, models.WSViewer)
	if !ok {
		return
	}
	uid := auth.UserID(c)
	if c.Request.Method == http.MethodDelete {
		h.db.Where("user_id = ? AND page_id = ?", uid, p.ID).Delete(&models.PageFavorite{})
	} else {
		var n int64
		h.db.Model(&models.PageFavorite{}).Where("user_id = ? AND page_id = ?", uid, p.ID).Count(&n)
		if n == 0 {
			var last struct{ Max *float64 }
			h.db.Model(&models.PageFavorite{}).Select("MAX(rank) AS max").Where("user_id = ?", uid).Scan(&last)
			h.db.Create(&models.PageFavorite{UserID: uid, PageID: p.ID, Rank: planner.Rank(last.Max, nil)})
		}
	}
	h.events.Publish(uid, liveEvent("favorites"))
	c.Status(http.StatusNoContent)
}

// listFavorites returns my favourite pages across workspaces.
//
//	@Summary	My favourite pages
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	pageSummary
//	@Router		/me/favorites [get]
func (h *Handlers) listFavorites(c *gin.Context) {
	uid := auth.UserID(c)
	var pages []models.Page
	h.db.Omit("content", "content_text").
		Joins("JOIN page_favorites ON page_favorites.page_id = pages.id").
		Where("page_favorites.user_id = ? AND pages.trashed_at IS NULL AND pages.workspace_id IN ?", uid, append(h.myWorkspaceIDs(c, true), "")).
		Order("page_favorites.rank").Find(&pages)
	out := make([]pageSummary, 0, len(pages))
	for _, p := range pages {
		s := summarize(p, nil)
		s.Favorite = true
		out = append(out, s)
	}
	c.JSON(http.StatusOK, out)
}

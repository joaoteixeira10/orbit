package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/linkpreview"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Bookmarks: links worth keeping, with what the page says about itself.

type bookmarkResponse struct {
	models.Bookmark
	Tags []string `json:"tags"`
}

func toBookmark(b models.Bookmark) bookmarkResponse {
	return bookmarkResponse{Bookmark: b, Tags: decodeList(b.Tags)}
}

func (h *Handlers) bookmarkAccess(c *gin.Context, need string) (models.Bookmark, bool) {
	var b models.Bookmark
	if err := h.db.First(&b, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
		return b, false
	}
	_, _, ok := h.wsAccess(c, b.WorkspaceID, need)
	return b, ok
}

// listBookmarks lists a workspace's bookmarks, newest first.
//
//	@Summary	A workspace's bookmarks
//	@Tags		bookmarks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	bookmarkResponse
//	@Router		/workspaces/{ws}/bookmarks [get]
func (h *Handlers) listBookmarks(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.Bookmark
	h.db.Where("workspace_id = ?", ws.ID).Order("created_at DESC").Find(&list)
	out := make([]bookmarkResponse, len(list))
	for i, b := range list {
		out[i] = toBookmark(b)
	}
	c.JSON(http.StatusOK, out)
}

type bookmarkRequest struct {
	URL         *string   `json:"url"`
	Title       *string   `json:"title" binding:"omitempty,max=300"`
	Description *string   `json:"description" binding:"omitempty,max=700"`
	Tags        *[]string `json:"tags"`
	Notes       *string   `json:"notes" binding:"omitempty,max=20000"`
	ReadLater   *bool     `json:"read_later"`
	Read        *bool     `json:"read"` // opened: true sets ReadAt, false clears it
}

func (req bookmarkRequest) apply(b *models.Bookmark) {
	if req.Title != nil {
		b.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		b.Description = strings.TrimSpace(*req.Description)
	}
	if req.Tags != nil {
		b.Tags = encodeList(cleanTags(*req.Tags))
	}
	if req.Notes != nil {
		b.Notes = *req.Notes
	}
	if req.ReadLater != nil {
		b.ReadLater = *req.ReadLater
		if b.ReadLater {
			b.ReadAt = nil
		}
	}
	if req.Read != nil {
		if *req.Read {
			now := time.Now()
			b.ReadAt = &now
		} else {
			b.ReadAt = nil
		}
	}
}

// preview fills in what the link says about itself and keeps its picture
// (a failure leaves the link as typed; it's still a bookmark).
func (h *Handlers) preview(ctx context.Context, b *models.Bookmark) {
	f := h.links
	if f == nil {
		f = linkpreview.New()
	}
	p, err := f.Fetch(ctx, b.URL)
	if err != nil {
		h.log.Debug("bookmark preview failed")
		return
	}
	if p.URL != "" {
		b.URL = p.URL
	}
	if b.Title == "" {
		b.Title = p.Title
	}
	if b.Description == "" {
		b.Description = p.Description
	}
	b.SiteName = p.SiteName
	if p.ImageURL != "" {
		if img, err := f.FetchImage(ctx, p.ImageURL); err == nil {
			h.db.Save(&models.BookmarkImage{ID: b.ID, Type: img.Type, Data: img.Data})
			b.HasImage = true
		}
	}
}

// createBookmark keeps a link (editors): its preview is read at once.
//
//	@Summary	Add a bookmark
//	@Tags		bookmarks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		bookmarkRequest	true	"The link (and tags, read later)"
//	@Success	201		{object}	bookmarkResponse
//	@Router		/workspaces/{ws}/bookmarks [post]
func (h *Handlers) createBookmark(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req bookmarkRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.URL == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the link"})
		return
	}
	link, err := linkpreview.Normalize(*req.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	b := models.Bookmark{WorkspaceID: ws.ID, URL: link, Tags: "[]", CreatedBy: auth.UserID(c)}
	req.apply(&b)
	if err := h.db.Create(&b).Error; err != nil {
		h.internalError(c, err, "could not keep the link")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	h.preview(ctx, &b)
	h.db.Save(&b)
	h.publishWS(c, ws.ID, "bookmarks")
	c.JSON(http.StatusCreated, toBookmark(b))
}

// updateBookmark changes a bookmark (editors; anyone who can see it may mark
// it read).
//
//	@Summary	Change a bookmark
//	@Tags		bookmarks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Bookmark id"
//	@Param		body	body		bookmarkRequest	true	"What changes"
//	@Success	200		{object}	bookmarkResponse
//	@Router		/bookmarks/{id} [put]
func (h *Handlers) updateBookmark(c *gin.Context) {
	b, ok := h.bookmarkAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req bookmarkRequest
	if !bindJSON(c, &req) {
		return
	}
	req.apply(&b)
	if err := h.db.Save(&b).Error; err != nil {
		h.internalError(c, err, "could not save the bookmark")
		return
	}
	h.publishWS(c, b.WorkspaceID, "bookmarks")
	c.JSON(http.StatusOK, toBookmark(b))
}

// refreshBookmark reads the link's preview again.
//
//	@Summary	Read a bookmark's preview again
//	@Tags		bookmarks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Bookmark id"
//	@Success	200	{object}	bookmarkResponse
//	@Router		/bookmarks/{id}/refresh [post]
func (h *Handlers) refreshBookmark(c *gin.Context) {
	b, ok := h.bookmarkAccess(c, models.WSEditor)
	if !ok {
		return
	}
	b.Title, b.Description = "", ""
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	h.preview(ctx, &b)
	h.db.Save(&b)
	h.publishWS(c, b.WorkspaceID, "bookmarks")
	c.JSON(http.StatusOK, toBookmark(b))
}

// deleteBookmark removes a bookmark (editors).
//
//	@Summary	Delete a bookmark
//	@Tags		bookmarks
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Bookmark id"
//	@Success	204
//	@Router		/bookmarks/{id} [delete]
func (h *Handlers) deleteBookmark(c *gin.Context) {
	b, ok := h.bookmarkAccess(c, models.WSEditor)
	if !ok {
		return
	}
	_ = deleteBookmarks(h.db, "id = ?", b.ID)
	h.publishWS(c, b.WorkspaceID, "bookmarks")
	c.Status(http.StatusNoContent)
}

// bookmarkImage serves a bookmark's picture (members): from here, so the
// site never sees who looks.
//
//	@Summary	A bookmark's picture
//	@Tags		bookmarks
//	@Produce	image/png
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Bookmark id"
//	@Success	200	{file}	binary
//	@Router		/bookmarks/{id}/image [get]
func (h *Handlers) bookmarkImage(c *gin.Context) {
	b, ok := h.bookmarkAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var img models.BookmarkImage
	if h.db.First(&img, "id = ?", b.ID).Error != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(http.StatusOK, img.Type, img.Data)
}

// deleteBookmarks removes bookmarks (a query on models.Bookmark) with their
// pictures.
func deleteBookmarks(tx *gorm.DB, where string, args ...any) error {
	var ids []string
	tx.Model(&models.Bookmark{}).Where(where, args...).Pluck("id", &ids)
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("id IN ?", ids).Delete(&models.BookmarkImage{}).Error; err != nil {
		return err
	}
	return tx.Where("id IN ?", ids).Delete(&models.Bookmark{}).Error
}

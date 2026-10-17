package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
)

// Uploads: images and files on pages, tasks and notes. A file belongs to the
// workspace of what it's on, and whoever can see that can see the file (a
// personal Quick Note's: only its owner). See internal/uploads.

type uploadResponse struct {
	models.Upload
	URL   string `json:"url"`
	Image bool   `json:"image"` // shown in the page (a raster image)
}

func toUploadResponse(u models.Upload) uploadResponse {
	return uploadResponse{Upload: u, URL: uploads.URL(u), Image: uploads.Inline(u.Type)}
}

// uploadTarget checks the caller may see (need = viewer) or change (editor)
// the page, task or note a file is on, answering when not. It returns the
// workspace ("" for a personal note).
func (h *Handlers) uploadTarget(c *gin.Context, target, id, need string) (string, bool) {
	notFound := func() (string, bool) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return "", false
	}
	var wsID string
	switch target {
	case uploads.Page:
		var p models.Page
		if h.db.Select("id", "workspace_id").First(&p, "id = ?", id).Error != nil {
			return notFound()
		}
		wsID = p.WorkspaceID
	case uploads.Task:
		var t models.Task
		if h.db.Select("id", "workspace_id").First(&t, "id = ?", id).Error != nil {
			return notFound()
		}
		wsID = t.WorkspaceID
	case uploads.Note:
		var n models.Note
		if h.db.Select("id", "workspace_id", "owner_id").First(&n, "id = ?", id).Error != nil {
			return notFound()
		}
		if n.WorkspaceID == "" {
			if n.OwnerID != auth.UserID(c) {
				return notFound()
			}
			if need != models.WSViewer && auth.Effective(c).ReadOnly {
				c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
				return "", false
			}
			return "", true
		}
		wsID = n.WorkspaceID
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "target must be page, task or note"})
		return "", false
	}
	if _, _, ok := h.wsAccess(c, wsID, need); !ok {
		return "", false
	}
	return wsID, true
}

// uploadFile adds a file (multipart field "file") to a page, task or note
// (editors; a personal note's owner).
//
//	@Summary	Upload a file
//	@Tags		uploads
//	@Accept		mpfd
//	@Produce	json
//	@Security	BearerAuth
//	@Param		target		query		string	true	"page, task or note"
//	@Param		target_id	query		string	true	"Its id"
//	@Param		file		formData	file	true	"The file"
//	@Success	201			{object}	uploadResponse
//	@Router		/uploads [post]
func (h *Handlers) uploadFile(c *gin.Context) {
	target, targetID := c.Query("target"), c.Query("target_id")
	wsID, ok := h.uploadTarget(c, target, targetID, models.WSEditor)
	if !ok {
		return
	}
	max := h.cfg.UploadMaxBytes
	if max <= 0 {
		max = 25 << 20
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max+1<<20) // + the form around it
	fh, err := c.FormFile("file")
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large (at most " + strconv.FormatInt(max>>20, 10) + " MB)"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "send the file in the form field \"file\""})
		return
	}
	if fh.Size > max {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large (at most " + strconv.FormatInt(max>>20, 10) + " MB)"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "could not read the file"})
		return
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(data)) > max {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "the file is too large"})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the file is empty"})
		return
	}
	u := models.Upload{WorkspaceID: wsID, OwnerID: auth.UserID(c), Target: target, TargetID: targetID, Name: fh.Filename}
	if err := uploads.Create(h.db, &u, data, h.cfg.UploadQuotaBytes); err != nil {
		if errors.Is(err, uploads.ErrQuota) {
			c.JSON(http.StatusInsufficientStorage, gin.H{"error": err.Error()})
			return
		}
		h.internalError(c, err, "could not store the file")
		return
	}
	if wsID != "" {
		h.publishWS(c, wsID, "uploads")
	}
	c.JSON(http.StatusCreated, toUploadResponse(u))
}

// listUploads lists the files on a page, task or note.
//
//	@Summary	Files on a page, task or note
//	@Tags		uploads
//	@Produce	json
//	@Security	BearerAuth
//	@Param		target		query	string	true	"page, task or note"
//	@Param		target_id	query	string	true	"Its id"
//	@Success	200			{array}	uploadResponse
//	@Router		/uploads [get]
func (h *Handlers) listUploads(c *gin.Context) {
	target, targetID := c.Query("target"), c.Query("target_id")
	if _, ok := h.uploadTarget(c, target, targetID, models.WSViewer); !ok {
		return
	}
	var rows []models.Upload
	h.db.Where("target = ? AND target_id = ?", target, targetID).Order("created_at").Find(&rows)
	out := make([]uploadResponse, 0, len(rows))
	for _, u := range rows {
		out = append(out, toUploadResponse(u))
	}
	c.JSON(http.StatusOK, out)
}

// uploadAccess loads :id and checks the caller may see (or change) it.
func (h *Handlers) uploadAccess(c *gin.Context, need string) (models.Upload, bool) {
	var u models.Upload
	if h.db.First(&u, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return u, false
	}
	if need == models.WSEditor {
		_, ok := h.uploadTarget(c, u.Target, u.TargetID, need)
		return u, ok
	}
	if u.WorkspaceID == "" {
		if u.OwnerID != auth.UserID(c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return u, false
		}
		return u, true
	}
	_, _, ok := h.wsAccess(c, u.WorkspaceID, models.WSViewer)
	return u, ok
}

// serveUpload sends a file: raster images inline, everything else as a
// download; never sniffed, never run (a sandboxing CSP of its own).
//
//	@Summary	Download a file
//	@Tags		uploads
//	@Produce	octet-stream
//	@Security	BearerAuth
//	@Param		id		path	string	true	"Upload id"
//	@Param		name	path	string	true	"File name (ignored)"
//	@Success	200
//	@Router		/uploads/{id}/{name} [get]
func (h *Handlers) serveUpload(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSViewer)
	if !ok {
		return
	}
	var blob models.UploadBlob
	if h.db.First(&blob, "id = ?", u.ID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	disp := "attachment"
	typ := "application/octet-stream"
	if uploads.Inline(u.Type) {
		disp, typ = "inline", u.Type
	}
	head := c.Writer.Header()
	head.Set("Content-Disposition", mime.FormatMediaType(disp, map[string]string{"filename": u.Name}))
	head.Set("X-Content-Type-Options", "nosniff")
	head.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	head.Set("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, typ, blob.Data)
}

// deleteUpload removes a file (whoever may change what it's on).
//
//	@Summary	Delete a file
//	@Tags		uploads
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Upload id"
//	@Success	204
//	@Router		/uploads/{id} [delete]
func (h *Handlers) deleteUpload(c *gin.Context) {
	u, ok := h.uploadAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if err := uploads.Delete(h.db, "id = ?", u.ID); err != nil {
		h.internalError(c, err, "could not delete the file")
		return
	}
	if u.WorkspaceID != "" {
		h.publishWS(c, u.WorkspaceID, "uploads")
	}
	c.Status(http.StatusNoContent)
}

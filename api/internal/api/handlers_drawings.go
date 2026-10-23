package api

import (
	"errors"
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Drawings (Excalidraw): the whiteboards of a workspace, and the drawing on a
// note. The scene is the browser's JSON, kept as it comes (capped); a
// preview SVG is shown as an image only.

const (
	maxScene   = 16 << 20 // embedded images included
	maxPreview = 1 << 20
	maxText    = 64 << 10
)

// drawingSummary is a whiteboard in a list: no scene.
type drawingSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Preview   string `json:"preview"`
	Version   int    `json:"version"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
}

// drawingAccess loads the :id drawing: a note's follows its note, a
// whiteboard its workspace's roles.
func (h *Handlers) drawingAccess(c *gin.Context, need string) (models.Drawing, bool) {
	var d models.Drawing
	if err := h.db.First(&d, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "drawing not found"})
		return d, false
	}
	if d.NoteID != "" {
		_, ok := h.noteAccessID(c, d.NoteID, need)
		return d, ok
	}
	_, _, ok := h.wsAccess(c, d.WorkspaceID, need)
	return d, ok
}

func (h *Handlers) publishDrawing(c *gin.Context, d models.Drawing) {
	switch {
	case d.NoteID != "":
		h.publishNotes(c, models.Note{Base: models.Base{ID: d.NoteID}, WorkspaceID: d.WorkspaceID, OwnerID: d.OwnerID})
	default:
		h.publishWS(c, d.WorkspaceID, "drawings")
	}
}

// listDrawings lists a workspace's whiteboards (newest change first).
//
//	@Summary	A workspace's whiteboards
//	@Tags		drawings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	drawingSummary
//	@Router		/workspaces/{ws}/drawings [get]
func (h *Handlers) listDrawings(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var ds []models.Drawing
	h.db.Select("id", "title", "preview", "version", "updated_by", "updated_at").
		Where("workspace_id = ? AND note_id = ''", ws.ID).Order("updated_at DESC").Find(&ds)
	out := make([]drawingSummary, 0, len(ds))
	for _, d := range ds {
		out = append(out, drawingSummary{ID: d.ID, Title: d.Title, Preview: d.Preview, Version: d.Version, UpdatedBy: d.UpdatedBy, UpdatedAt: d.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z")})
	}
	c.JSON(http.StatusOK, out)
}

type drawingRequest struct {
	Title   *string `json:"title" binding:"omitempty,max=300"`
	Scene   *string `json:"scene"`
	Preview *string `json:"preview"`
	Text    *string `json:"text"`
	Version int     `json:"version"` // the version saved over (0: don't check)
}

// apply copies a request's fields, checked.
func (req drawingRequest) apply(d *models.Drawing) error {
	if req.Title != nil {
		d.Title = strings.TrimSpace(*req.Title)
	}
	if req.Scene != nil {
		if len(*req.Scene) > maxScene {
			return errors.New("the drawing is too large (16 MB at most, images included)")
		}
		d.Scene = *req.Scene
	}
	if req.Preview != nil {
		p := *req.Preview
		if len(p) > maxPreview || (p != "" && !strings.HasPrefix(strings.TrimSpace(p), "<svg")) {
			p = "" // no preview rather than a wrong one
		}
		d.Preview = p
	}
	if req.Text != nil {
		t := *req.Text
		if len(t) > maxText {
			t = t[:maxText]
		}
		d.Text = t
	}
	return nil
}

// createDrawing makes a whiteboard (editors).
//
//	@Summary	New whiteboard
//	@Tags		drawings
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		drawingRequest	true	"Title (and scene)"
//	@Success	201		{object}	models.Drawing
//	@Router		/workspaces/{ws}/drawings [post]
func (h *Handlers) createDrawing(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req drawingRequest
	if !h.bindDrawing(c, &req) {
		return
	}
	uid := auth.UserID(c)
	d := models.Drawing{WorkspaceID: ws.ID, CreatedBy: uid, UpdatedBy: uid, Version: 1}
	if err := req.apply(&d); err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Create(&d).Error; err != nil {
		h.internalError(c, err, "could not create the whiteboard")
		return
	}
	h.publishDrawing(c, d)
	c.JSON(http.StatusCreated, d)
}

// bindDrawing reads a drawing request with its own size cap (scenes carry
// their images).
func (h *Handlers) bindDrawing(c *gin.Context, req *drawingRequest) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxScene+maxPreview+maxText+64<<10)
	return bindJSON(c, req)
}

// getDrawing returns a drawing with its scene.
//
//	@Summary	A drawing
//	@Tags		drawings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Drawing id"
//	@Success	200	{object}	models.Drawing
//	@Router		/drawings/{id} [get]
func (h *Handlers) getDrawing(c *gin.Context) {
	d, ok := h.drawingAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, d)
}

// updateDrawing saves a drawing (editors). With version, a save over a newer
// one is refused (409, the newer one in the answer).
//
//	@Summary	Save a drawing
//	@Tags		drawings
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Drawing id"
//	@Param		body	body		drawingRequest	true	"What changed"
//	@Success	200		{object}	models.Drawing
//	@Failure	409		{object}	models.Drawing
//	@Router		/drawings/{id} [put]
func (h *Handlers) updateDrawing(c *gin.Context) {
	d, ok := h.drawingAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req drawingRequest
	if !h.bindDrawing(c, &req) {
		return
	}
	if req.Version != 0 && req.Version != d.Version {
		c.JSON(http.StatusConflict, d)
		return
	}
	if err := req.apply(&d); err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		return
	}
	d.Version++
	d.UpdatedBy = auth.UserID(c)
	// Saved only if still that version (two saves at once: one wins).
	res := h.db.Model(&models.Drawing{}).Where("id = ? AND version = ?", d.ID, d.Version-1).
		Updates(map[string]any{"title": d.Title, "scene": d.Scene, "preview": d.Preview, "text": d.Text, "version": d.Version, "updated_by": d.UpdatedBy})
	if res.Error != nil {
		h.internalError(c, res.Error, "could not save the drawing")
		return
	}
	if res.RowsAffected == 0 {
		h.db.First(&d, "id = ?", d.ID)
		c.JSON(http.StatusConflict, d)
		return
	}
	h.db.First(&d, "id = ?", d.ID)
	h.publishDrawing(c, d)
	c.JSON(http.StatusOK, d)
}

// deleteDrawing deletes a drawing (editors).
//
//	@Summary	Delete a drawing
//	@Tags		drawings
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Drawing id"
//	@Success	204
//	@Router		/drawings/{id} [delete]
func (h *Handlers) deleteDrawing(c *gin.Context) {
	d, ok := h.drawingAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Delete(&d)
	h.publishDrawing(c, d)
	c.Status(http.StatusNoContent)
}

// noteDrawingOf returns a note's drawing, making an empty one the first time
// (editors make it; viewers get 404 until then).
//
//	@Summary	A note's drawing
//	@Tags		drawings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Note id"
//	@Success	200	{object}	models.Drawing
//	@Router		/notes/{id}/drawing [post]
func (h *Handlers) noteDrawingOf(c *gin.Context) {
	n, ok := h.noteAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var d models.Drawing
	if h.db.Where("note_id = ?", n.ID).First(&d).Error == nil {
		c.JSON(http.StatusOK, d)
		return
	}
	uid := auth.UserID(c)
	d = models.Drawing{WorkspaceID: n.WorkspaceID, OwnerID: n.OwnerID, NoteID: n.ID, CreatedBy: uid, UpdatedBy: uid, Version: 1}
	if err := h.db.Create(&d).Error; err != nil {
		h.internalError(c, err, "could not start the drawing")
		return
	}
	h.publishNotes(c, n)
	c.JSON(http.StatusOK, d)
}

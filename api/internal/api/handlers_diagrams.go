package api

import (
	"net/http"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// Diagrams: a workspace's draw.io and Mermaid diagrams. The app edits them
// (draw.io's in the draw.io editor at the address the instance sets,
// Mermaid's as text) and hands over the file (draw.io's XML, or Mermaid's
// text), a picture of it (SVG) and its labels for search; the server stores
// them as they come, capped, and guards saves by version.

const (
	maxDiagramData    = 8 << 20
	maxDiagramPreview = 4 << 20
	maxDiagramText    = 256 << 10
)

type diagramSummary struct {
	ID        string `json:"id"`
	FolderID  string `json:"folder_id"`
	Kind      string `json:"kind"` // drawio | mermaid
	Title     string `json:"title"`
	Preview   string `json:"preview,omitempty"`
	Version   int    `json:"version"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	Size      int    `json:"size"` // its file's bytes
}

func (h *Handlers) diagramAccess(c *gin.Context, need string) (models.Diagram, bool) {
	var d models.Diagram
	if err := h.db.First(&d, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "diagram not found"})
		return d, false
	}
	_, _, ok := h.wsAccess(c, d.WorkspaceID, need)
	return d, ok
}

// listDiagrams lists a workspace's diagrams with their pictures (newest
// change first).
//
//	@Summary	A workspace's diagrams
//	@Tags		diagrams
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	diagramSummary
//	@Router		/workspaces/{ws}/diagrams [get]
func (h *Handlers) listDiagrams(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.Diagram
	h.db.Select("id", "folder_id", "kind", "title", "preview", "version", "updated_by", "updated_at", "created_by", "created_at", "length(data) AS data").Where("workspace_id = ?", ws.ID).Order("updated_at DESC").Find(&list)
	out := make([]diagramSummary, 0, len(list))
	for _, d := range list {
		out = append(out, diagramSummary{ID: d.ID, FolderID: d.FolderID, Kind: d.Kind, Title: d.Title, Preview: d.Preview, Version: d.Version, UpdatedBy: d.UpdatedBy, UpdatedAt: stamp(d.UpdatedAt), CreatedBy: d.CreatedBy, CreatedAt: stamp(d.CreatedAt), Size: atoiSafe(d.Data)})
	}
	c.JSON(http.StatusOK, out)
}

type diagramRequest struct {
	FolderID string  `json:"folder_id"`                                     // where a new one goes ("" the top)
	Kind     string  `json:"kind" binding:"omitempty,oneof=drawio mermaid"` // a new one's (drawio)
	Title    *string `json:"title" binding:"omitempty,max=300"`
	Data     *string `json:"data"`    // draw.io's XML, or Mermaid's text
	Preview  *string `json:"preview"` // an SVG picture of it
	Text     *string `json:"text"`
	Version  int     `json:"version"` // the version saved over (0: don't check)
}

// apply copies a request's fields, checked: a draw.io file is XML, the
// picture an SVG.
func (req diagramRequest) apply(d *models.Diagram) (int, string) {
	if req.Title != nil {
		d.Title = strings.TrimSpace(*req.Title)
	}
	if req.Data != nil {
		x := strings.TrimSpace(*req.Data)
		if len(x) > maxDiagramData {
			return http.StatusRequestEntityTooLarge, "the diagram is too large (8 MB at most)"
		}
		if x != "" && d.Kind != models.DiagramMermaid && !strings.HasPrefix(x, "<") {
			return http.StatusBadRequest, "the diagram must be draw.io's XML"
		}
		d.Data = x
	}
	if req.Preview != nil {
		x := strings.TrimSpace(*req.Preview)
		if len(x) > maxDiagramPreview {
			x = "" // too big to show in a list: none
		}
		if x != "" && !strings.HasPrefix(x, "<svg") && !strings.HasPrefix(x, "<?xml") {
			return http.StatusBadRequest, "the picture must be an SVG"
		}
		d.Preview = x
	}
	if req.Text != nil {
		t := *req.Text
		if len(t) > maxDiagramText {
			t = t[:maxDiagramText]
		}
		d.Text = t
	}
	return 0, ""
}

func (h *Handlers) bindDiagram(c *gin.Context, req *diagramRequest) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDiagramData+maxDiagramPreview+maxDiagramText+64<<10)
	return bindJSON(c, req)
}

// createDiagram makes a diagram (editors).
//
//	@Summary	New diagram
//	@Tags		diagrams
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		diagramRequest	true	"Title (and file)"
//	@Success	201		{object}	models.Diagram
//	@Router		/workspaces/{ws}/diagrams [post]
func (h *Handlers) createDiagram(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req diagramRequest
	if !h.bindDiagram(c, &req) {
		return
	}
	if !h.folderIn(ws.ID, req.FolderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder here"})
		return
	}
	uid := auth.UserID(c)
	d := models.Diagram{WorkspaceID: ws.ID, FolderID: req.FolderID, Kind: req.Kind, CreatedBy: uid, UpdatedBy: uid, Version: 1}
	if d.Kind == "" {
		d.Kind = models.DiagramDrawio
	}
	if code, msg := req.apply(&d); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.Diagram{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	d.Rank = planner.Rank(last.Max, nil)
	if err := h.db.Create(&d).Error; err != nil {
		h.internalError(c, err, "could not create the diagram")
		return
	}
	h.publishWS(c, ws.ID, "diagrams")
	c.JSON(http.StatusCreated, d)
}

// getDiagram returns a diagram with its file.
//
//	@Summary	A diagram
//	@Tags		diagrams
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Diagram id"
//	@Success	200	{object}	models.Diagram
//	@Router		/diagrams/{id} [get]
func (h *Handlers) getDiagram(c *gin.Context) {
	d, ok := h.diagramAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, d)
}

// updateDiagram saves a diagram (editors). With version, a save over a newer
// one is refused (409, the newer one in the answer).
//
//	@Summary	Save a diagram
//	@Tags		diagrams
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Diagram id"
//	@Param		body	body		diagramRequest	true	"What changed"
//	@Success	200		{object}	models.Diagram
//	@Failure	409		{object}	models.Diagram
//	@Router		/diagrams/{id} [put]
func (h *Handlers) updateDiagram(c *gin.Context) {
	d, ok := h.diagramAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req diagramRequest
	if !h.bindDiagram(c, &req) {
		return
	}
	if req.Version != 0 && req.Version != d.Version {
		c.JSON(http.StatusConflict, d)
		return
	}
	if code, msg := req.apply(&d); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	d.Version++
	d.UpdatedBy = auth.UserID(c)
	res := h.db.Model(&models.Diagram{}).Where("id = ? AND version = ?", d.ID, d.Version-1).
		Updates(map[string]any{"title": d.Title, "data": d.Data, "preview": d.Preview, "text": d.Text, "version": d.Version, "updated_by": d.UpdatedBy})
	if res.Error != nil {
		h.internalError(c, res.Error, "could not save the diagram")
		return
	}
	h.db.First(&d, "id = ?", d.ID)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, d)
		return
	}
	h.publishWS(c, d.WorkspaceID, "diagrams")
	c.JSON(http.StatusOK, d)
}

// deleteDiagram deletes a diagram (editors).
//
//	@Summary	Delete a diagram
//	@Tags		diagrams
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Diagram id"
//	@Success	204
//	@Router		/diagrams/{id} [delete]
func (h *Handlers) deleteDiagram(c *gin.Context) {
	d, ok := h.diagramAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Delete(&d)
	h.publishWS(c, d.WorkspaceID, "diagrams")
	c.Status(http.StatusNoContent)
}

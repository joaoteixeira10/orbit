package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// Sheets: a workspace's spreadsheets. The app keeps the cells (what was
// typed: values and "=" formulas, and their look) as JSON and works the
// formulas out itself; the server stores it as it comes (an object, capped)
// with the sheet's words for search, and guards saves by version.

const (
	maxSheetData = 8 << 20
	maxSheetText = 256 << 10
)

type sheetSummary struct {
	ID        string `json:"id"`
	FolderID  string `json:"folder_id"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
	UpdatedBy string `json:"updated_by"`
	UpdatedAt string `json:"updated_at"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	Size      int    `json:"size"` // its data's bytes
}

func (h *Handlers) sheetAccess(c *gin.Context, need string) (models.Sheet, bool) {
	var s models.Sheet
	if err := h.db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sheet not found"})
		return s, false
	}
	_, _, ok := h.wsAccess(c, s.WorkspaceID, need)
	return s, ok
}

// listSheets lists a workspace's sheets (newest change first).
//
//	@Summary	A workspace's sheets
//	@Tags		sheets
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	sheetSummary
//	@Router		/workspaces/{ws}/sheets [get]
func (h *Handlers) listSheets(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.Sheet
	h.db.Select("id", "folder_id", "title", "version", "updated_by", "updated_at", "created_by", "created_at", "length(data) AS data").Where("workspace_id = ?", ws.ID).Order("updated_at DESC").Find(&list)
	out := make([]sheetSummary, 0, len(list))
	for _, s := range list {
		out = append(out, sheetSummary{ID: s.ID, FolderID: s.FolderID, Title: s.Title, Version: s.Version, UpdatedBy: s.UpdatedBy, UpdatedAt: stamp(s.UpdatedAt), CreatedBy: s.CreatedBy, CreatedAt: stamp(s.CreatedAt), Size: atoiSafe(s.Data)})
	}
	c.JSON(http.StatusOK, out)
}

type sheetRequest struct {
	FolderID string           `json:"folder_id"` // where a new one goes ("" the top)
	Title    *string          `json:"title" binding:"omitempty,max=300"`
	Data     *json.RawMessage `json:"data" swaggertype:"object"`
	Text     *string          `json:"text"`
	Version  int              `json:"version"` // the version saved over (0: don't check)
}

// apply copies a request's fields, checked: the data is a JSON object.
func (req sheetRequest) apply(s *models.Sheet) (int, string) {
	if req.Title != nil {
		s.Title = strings.TrimSpace(*req.Title)
	}
	if req.Data != nil {
		raw := strings.TrimSpace(string(*req.Data))
		if len(raw) > maxSheetData {
			return http.StatusRequestEntityTooLarge, "the sheet is too large (8 MB at most)"
		}
		var obj map[string]json.RawMessage
		if raw != "" && raw != "null" && json.Unmarshal([]byte(raw), &obj) != nil {
			return http.StatusBadRequest, "the sheet's data must be an object"
		}
		if raw == "null" {
			raw = ""
		}
		s.Data = raw
	}
	if req.Text != nil {
		t := *req.Text
		if len(t) > maxSheetText {
			t = t[:maxSheetText]
		}
		s.Text = t
	}
	return 0, ""
}

func (h *Handlers) bindSheet(c *gin.Context, req *sheetRequest) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSheetData+maxSheetText+64<<10)
	return bindJSON(c, req)
}

// sheetOut is a sheet with its data as an object.
type sheetOut struct {
	models.Sheet
	Data json.RawMessage `json:"data,omitempty" swaggertype:"object"`
}

func sheetJSON(s models.Sheet) sheetOut { return sheetOut{Sheet: s, Data: rawJSON(s.Data)} }

// createSheet makes a sheet (editors).
//
//	@Summary	New sheet
//	@Tags		sheets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		sheetRequest	true	"Title (and data)"
//	@Success	201		{object}	sheetOut
//	@Router		/workspaces/{ws}/sheets [post]
func (h *Handlers) createSheet(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req sheetRequest
	if !h.bindSheet(c, &req) {
		return
	}
	if !h.folderIn(ws.ID, req.FolderID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no such folder here"})
		return
	}
	uid := auth.UserID(c)
	s := models.Sheet{WorkspaceID: ws.ID, FolderID: req.FolderID, CreatedBy: uid, UpdatedBy: uid, Version: 1}
	if code, msg := req.apply(&s); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.Sheet{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	s.Rank = planner.Rank(last.Max, nil)
	if err := h.db.Create(&s).Error; err != nil {
		h.internalError(c, err, "could not create the sheet")
		return
	}
	h.publishWS(c, ws.ID, "sheets")
	c.JSON(http.StatusCreated, sheetJSON(s))
}

// getSheet returns a sheet with its cells.
//
//	@Summary	A sheet
//	@Tags		sheets
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Sheet id"
//	@Success	200	{object}	sheetOut
//	@Router		/sheets/{id} [get]
func (h *Handlers) getSheet(c *gin.Context) {
	s, ok := h.sheetAccess(c, models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, sheetJSON(s))
}

// updateSheet saves a sheet (editors). With version, a save over a newer one
// is refused (409, the newer one in the answer).
//
//	@Summary	Save a sheet
//	@Tags		sheets
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Sheet id"
//	@Param		body	body		sheetRequest	true	"What changed"
//	@Success	200		{object}	sheetOut
//	@Failure	409		{object}	sheetOut
//	@Router		/sheets/{id} [put]
func (h *Handlers) updateSheet(c *gin.Context) {
	s, ok := h.sheetAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req sheetRequest
	if !h.bindSheet(c, &req) {
		return
	}
	if req.Version != 0 && req.Version != s.Version {
		c.JSON(http.StatusConflict, sheetJSON(s))
		return
	}
	if code, msg := req.apply(&s); msg != "" {
		c.JSON(code, gin.H{"error": msg})
		return
	}
	s.Version++
	s.UpdatedBy = auth.UserID(c)
	res := h.db.Model(&models.Sheet{}).Where("id = ? AND version = ?", s.ID, s.Version-1).
		Updates(map[string]any{"title": s.Title, "data": s.Data, "text": s.Text, "version": s.Version, "updated_by": s.UpdatedBy})
	if res.Error != nil {
		h.internalError(c, res.Error, "could not save the sheet")
		return
	}
	h.db.First(&s, "id = ?", s.ID)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, sheetJSON(s))
		return
	}
	h.publishWS(c, s.WorkspaceID, "sheets")
	c.JSON(http.StatusOK, sheetJSON(s))
}

// deleteSheet deletes a sheet (editors).
//
//	@Summary	Delete a sheet
//	@Tags		sheets
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Sheet id"
//	@Success	204
//	@Router		/sheets/{id} [delete]
func (h *Handlers) deleteSheet(c *gin.Context) {
	s, ok := h.sheetAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Delete(&s)
	h.publishWS(c, s.WorkspaceID, "sheets")
	c.Status(http.StatusNoContent)
}

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Quick notes: sticky notes on a workspace's wall. A note is a short text or
// a checklist; switching between the two keeps the lines.

const (
	maxNoteItems    = 200
	maxNoteItemText = 500
)

// noteColors are the colour keys a note can wear ("" = plain). The SPA maps
// them to light and dark tints.
var noteColors = map[string]bool{
	"": true, "yellow": true, "orange": true, "red": true, "pink": true, "purple": true,
	"blue": true, "teal": true, "green": true, "brown": true, "gray": true,
}

// noteItem is one line of a checklist note.
type noteItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type noteResponse struct {
	models.Note
	Items []noteItem `json:"items"`
}

func decodeItems(s string) []noteItem {
	out := []noteItem{}
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

func encodeItems(v []noteItem) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func toNoteResponse(n models.Note) noteResponse {
	return noteResponse{Note: n, Items: decodeItems(n.Items)}
}

// cleanItems trims the lines, drops empty ones and gives new ones an id.
func cleanItems(in []noteItem) ([]noteItem, string) {
	if len(in) > maxNoteItems {
		return nil, "a checklist holds up to 200 items"
	}
	out := make([]noteItem, 0, len(in))
	seen := map[string]bool{}
	for _, it := range in {
		it.Text = strings.TrimSpace(it.Text)
		if it.Text == "" {
			continue
		}
		if len(it.Text) > maxNoteItemText {
			return nil, "a checklist item is up to 500 characters"
		}
		if it.ID == "" || len(it.ID) > 36 || seen[it.ID] {
			it.ID = uuid.NewString()
		}
		seen[it.ID] = true
		out = append(out, it)
	}
	return out, ""
}

// linesToItems / itemsToText convert between a text note and a checklist.
func linesToItems(body string) []noteItem {
	out := []noteItem{}
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-•*"))
		if l != "" && len(out) < maxNoteItems {
			if len(l) > maxNoteItemText {
				l = l[:maxNoteItemText]
			}
			out = append(out, noteItem{ID: uuid.NewString(), Text: l})
		}
	}
	return out
}

func itemsToText(items []noteItem) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = it.Text
	}
	return strings.Join(lines, "\n")
}

func (h *Handlers) noteAccess(c *gin.Context, need string) (models.Note, bool) {
	var n models.Note
	if err := h.db.First(&n, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
		return n, false
	}
	_, _, ok := h.wsAccess(c, n.WorkspaceID, need)
	return n, ok
}

// listNotes returns a workspace's notes, pinned first, then in their order.
//
//	@Summary	Quick notes of a workspace
//	@Tags		notes
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	noteResponse
//	@Router		/workspaces/{ws}/notes [get]
func (h *Handlers) listNotes(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var notes []models.Note
	h.db.Where("workspace_id = ?", ws.ID).Order("pinned DESC, rank").Find(&notes)
	out := make([]noteResponse, 0, len(notes))
	for _, n := range notes {
		out = append(out, toNoteResponse(n))
	}
	c.JSON(http.StatusOK, out)
}

type noteRequest struct {
	Title     *string     `json:"title" binding:"omitempty,max=300"`
	Body      *string     `json:"body" binding:"omitempty,max=20000"`
	Checklist *bool       `json:"checklist"`
	Items     *[]noteItem `json:"items"`
	Color     *string     `json:"color"`
	Pinned    *bool       `json:"pinned"`
}

// applyNoteFields validates and copies a request onto n. Turning a text note
// into a checklist makes each line an item (and back).
func applyNoteFields(req noteRequest, n *models.Note) string {
	if req.Title != nil {
		n.Title = strings.TrimSpace(*req.Title)
	}
	if req.Body != nil {
		n.Body = strings.TrimRight(*req.Body, " \n\t")
	}
	if req.Color != nil {
		if !noteColors[*req.Color] {
			return "unknown note colour"
		}
		n.Color = *req.Color
	}
	if req.Pinned != nil {
		n.Pinned = *req.Pinned
	}
	items := decodeItems(n.Items)
	if req.Items != nil {
		var msg string
		if items, msg = cleanItems(*req.Items); msg != "" {
			return msg
		}
	}
	if req.Checklist != nil && *req.Checklist != n.Checklist {
		n.Checklist = *req.Checklist
		if n.Checklist && req.Items == nil {
			items = linesToItems(n.Body)
		}
		if !n.Checklist && req.Body == nil {
			n.Body = itemsToText(items)
		}
	}
	if n.Checklist {
		n.Body = ""
	} else {
		items = []noteItem{}
	}
	n.Items = encodeItems(items)
	n.PlainText = n.Body
	if n.Checklist {
		n.PlainText = itemsToText(items)
	}
	return ""
}

func noteEmpty(n models.Note) bool {
	return n.Title == "" && strings.TrimSpace(n.Body) == "" && len(decodeItems(n.Items)) == 0
}

// createNote adds a note at the top of the wall (editors).
//
//	@Summary	Create a quick note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		noteRequest	true	"Note"
//	@Success	201		{object}	noteResponse
//	@Router		/workspaces/{ws}/notes [post]
func (h *Handlers) createNote(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req noteRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	n := models.Note{WorkspaceID: ws.ID, Items: "[]", CreatedBy: uid, UpdatedBy: uid}
	if req.Checklist != nil && *req.Checklist && req.Items != nil {
		// A new checklist: no text to convert.
		n.Checklist = true
	}
	if msg := applyNoteFields(req, &n); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if noteEmpty(n) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the note is empty"})
		return
	}
	var first struct{ Min *float64 }
	h.db.Model(&models.Note{}).Select("MIN(rank) AS min").Where("workspace_id = ?", ws.ID).Scan(&first)
	n.Rank = 1024
	if first.Min != nil {
		n.Rank = *first.Min - 1024
	}
	if err := h.db.Create(&n).Error; err != nil {
		h.internalError(c, err, "could not create the note")
		return
	}
	h.publishWS(c, ws.ID, "notes")
	c.JSON(http.StatusCreated, toNoteResponse(n))
}

// updateNote changes a note (editors).
//
//	@Summary	Update a quick note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"Note id"
//	@Param		body	body		noteRequest	true	"Fields to change"
//	@Success	200		{object}	noteResponse
//	@Router		/notes/{id} [put]
func (h *Handlers) updateNote(c *gin.Context) {
	n, ok := h.noteAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req noteRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := applyNoteFields(req, &n); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	n.UpdatedBy = auth.UserID(c)
	if err := h.db.Save(&n).Error; err != nil {
		h.internalError(c, err, "could not save the note")
		return
	}
	h.publishWS(c, n.WorkspaceID, "notes")
	c.JSON(http.StatusOK, toNoteResponse(n))
}

// deleteNote deletes a note (editors).
//
//	@Summary	Delete a quick note
//	@Tags		notes
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Note id"
//	@Success	204
//	@Router		/notes/{id} [delete]
func (h *Handlers) deleteNote(c *gin.Context) {
	n, ok := h.noteAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if err := h.db.Delete(&models.Note{}, "id = ?", n.ID).Error; err != nil {
		h.internalError(c, err, "could not delete the note")
		return
	}
	h.publishWS(c, n.WorkspaceID, "notes")
	c.Status(http.StatusNoContent)
}

// orderNotes sets the order of a workspace's notes (editors); the SPA sends
// every note's id. Pinned notes still come first.
//
//	@Summary	Reorder quick notes
//	@Tags		notes
//	@Accept		json
//	@Security	BearerAuth
//	@Param		ws		path	string			true	"Workspace id"
//	@Param		body	body	orderRequest	true	"Note ids in order"
//	@Success	204
//	@Router		/workspaces/{ws}/notes/order [put]
func (h *Handlers) orderNotes(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req orderRequest
	if !bindJSON(c, &req) {
		return
	}
	if len(req.IDs) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many notes"})
		return
	}
	for i, id := range req.IDs {
		h.db.Model(&models.Note{}).Where("id = ? AND workspace_id = ?", id, ws.ID).Update("rank", float64(i+1)*1024)
	}
	h.publishWS(c, ws.ID, "notes")
	c.Status(http.StatusNoContent)
}

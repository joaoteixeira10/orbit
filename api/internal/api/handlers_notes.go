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

// noteAccess loads the :id note: a workspace's note follows its roles; a
// personal note is its owner's alone (anyone else gets 404).
func (h *Handlers) noteAccess(c *gin.Context, need string) (models.Note, bool) {
	var n models.Note
	if err := h.db.First(&n, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
		return n, false
	}
	if n.WorkspaceID == "" {
		if n.OwnerID != auth.UserID(c) {
			c.JSON(http.StatusNotFound, gin.H{"error": "note not found"})
			return n, false
		}
		if need != models.WSViewer && auth.Effective(c).ReadOnly {
			c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
			return n, false
		}
		return n, true
	}
	_, _, ok := h.wsAccess(c, n.WorkspaceID, need)
	return n, ok
}

// publishNotes tells the open tabs a wall of notes changed: a workspace's
// members, or (personal notes, ws "me") the owner.
func (h *Handlers) publishNotes(c *gin.Context, n models.Note) {
	if n.WorkspaceID != "" {
		h.publishWS(c, n.WorkspaceID, "notes")
		return
	}
	b, _ := json.Marshal(gin.H{"type": "ws", "ws": "me", "what": "notes", "origin": c.GetHeader("X-Orbit-Client")})
	h.events.Publish(n.OwnerID, string(b))
}

// topRank is a rank above every note of a wall (new notes go first).
func (h *Handlers) topRank(wsID, ownerID string) float64 {
	var first struct{ Min *float64 }
	q := h.db.Model(&models.Note{}).Select("MIN(rank) AS min")
	if wsID != "" {
		q = q.Where("workspace_id = ?", wsID)
	} else {
		q = q.Where("workspace_id = '' AND owner_id = ?", ownerID)
	}
	q.Scan(&first)
	if first.Min == nil {
		return 1024
	}
	return *first.Min - 1024
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

// createNote adds a note at the top of a workspace's wall (editors).
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
	if n, ok := h.addNote(c, models.Note{WorkspaceID: ws.ID}); ok {
		h.notifyActivity(c, ws, "note_added", noteTitle(n), noteLink(n))
	}
}

// listMyNotes returns the caller's personal notes (their scratchpad),
// pinned first.
//
//	@Summary	My quick notes
//	@Tags		notes
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	noteResponse
//	@Router		/me/notes [get]
func (h *Handlers) listMyNotes(c *gin.Context) {
	var notes []models.Note
	h.db.Where("workspace_id = '' AND owner_id = ?", auth.UserID(c)).Order("pinned DESC, rank").Find(&notes)
	out := make([]noteResponse, 0, len(notes))
	for _, n := range notes {
		out = append(out, toNoteResponse(n))
	}
	c.JSON(http.StatusOK, out)
}

// createMyNote jots a personal note, in no workspace.
//
//	@Summary	Create a personal quick note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		noteRequest	true	"Note"
//	@Success	201		{object}	noteResponse
//	@Router		/me/notes [post]
func (h *Handlers) createMyNote(c *gin.Context) {
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return
	}
	h.addNote(c, models.Note{OwnerID: auth.UserID(c)})
}

// addNote creates n (in a workspace, or personal) from the request and
// writes the response.
func (h *Handlers) addNote(c *gin.Context, n models.Note) (models.Note, bool) {
	var req noteRequest
	if !bindJSON(c, &req) {
		return n, false
	}
	uid := auth.UserID(c)
	n.Items, n.CreatedBy, n.UpdatedBy = "[]", uid, uid
	if req.Checklist != nil && *req.Checklist && req.Items != nil {
		// A new checklist: no text to convert.
		n.Checklist = true
	}
	if msg := applyNoteFields(req, &n); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return n, false
	}
	if noteEmpty(n) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the note is empty"})
		return n, false
	}
	n.Rank = h.topRank(n.WorkspaceID, n.OwnerID)
	if err := h.db.Create(&n).Error; err != nil {
		h.internalError(c, err, "could not create the note")
		return n, false
	}
	h.publishNotes(c, n)
	c.JSON(http.StatusCreated, toNoteResponse(n))
	return n, true
}

// noteLink opens a note in the app.
func noteLink(n models.Note) string {
	if n.WorkspaceID == "" {
		return "/notes?note=" + n.ID
	}
	return "/w/" + n.WorkspaceID + "/notes?note=" + n.ID
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
	h.publishNotes(c, n)
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
	h.publishNotes(c, n)
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

// orderMyNotes sets the order of the caller's personal notes.
//
//	@Summary	Reorder my quick notes
//	@Tags		notes
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	orderRequest	true	"Note ids in order"
//	@Success	204
//	@Router		/me/notes/order [put]
func (h *Handlers) orderMyNotes(c *gin.Context) {
	var req orderRequest
	if !bindJSON(c, &req) {
		return
	}
	if len(req.IDs) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many notes"})
		return
	}
	uid := auth.UserID(c)
	for i, id := range req.IDs {
		h.db.Model(&models.Note{}).Where("id = ? AND workspace_id = '' AND owner_id = ?", id, uid).Update("rank", float64(i+1)*1024)
	}
	h.publishNotes(c, models.Note{OwnerID: uid})
	c.Status(http.StatusNoContent)
}

type moveNoteRequest struct {
	WorkspaceID string `json:"workspace_id"` // "" = my own notes
}

// moveNote puts a note somewhere else: from the caller's own notes into a
// workspace they can edit, between workspaces, or back to their own notes.
// It goes to the top of its new wall.
//
//	@Summary	Move a quick note
//	@Tags		notes
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Note id"
//	@Param		body	body		moveNoteRequest	true	"Where to"
//	@Success	200		{object}	noteResponse
//	@Router		/notes/{id}/move [post]
func (h *Handlers) moveNote(c *gin.Context) {
	n, ok := h.noteAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req moveNoteRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.WorkspaceID == n.WorkspaceID {
		c.JSON(http.StatusOK, toNoteResponse(n))
		return
	}
	uid := auth.UserID(c)
	from := n
	var target models.Workspace
	if req.WorkspaceID != "" {
		ws, _, ok := h.wsAccess(c, req.WorkspaceID, models.WSEditor)
		if !ok {
			return
		}
		target = ws
		n.WorkspaceID, n.OwnerID = ws.ID, ""
	} else {
		n.WorkspaceID, n.OwnerID = "", uid
	}
	n.Rank, n.UpdatedBy = h.topRank(n.WorkspaceID, n.OwnerID), uid
	if err := h.db.Save(&n).Error; err != nil {
		h.internalError(c, err, "could not move the note")
		return
	}
	h.publishNotes(c, from)
	h.publishNotes(c, n)
	if target.ID != "" {
		h.notifyActivity(c, target, "note_added", noteTitle(n), noteLink(n))
	}
	c.JSON(http.StatusOK, toNoteResponse(n))
}

// noteTitle names a note in a notification: its title, else its first line.
func noteTitle(n models.Note) string {
	if n.Title != "" {
		return n.Title
	}
	line, _, _ := strings.Cut(strings.TrimSpace(n.PlainText), "\n")
	if r := []rune(line); len(r) > 80 {
		line = string(r[:80]) + "…"
	}
	return line
}

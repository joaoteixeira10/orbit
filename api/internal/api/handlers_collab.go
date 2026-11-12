package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/collab"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Presence and editing together. Presence: who's on a page, a sheet, a
// diagram or a whiteboard right now (each tab says so every few seconds).
// Editing together: a page's or a whiteboard's changes go through a log
// per document — numbered, kept until the next compaction — that the
// server relays to whoever is there and doesn't read (Yjs updates for a
// page, changed elements for a whiteboard). A tab that missed one (the
// event stream may drop) asks for what's after the last it has.
//
// What's saved stays the page's Markdown (or the whiteboard's scene): one
// tab saves it for everyone, marked X-Orbit-Collab. Saved any other way
// (another editor, the API, a restored version), the log starts over.

const maxCollabUpdate = 2 << 20 // one change, at most

type collabDoc struct {
	key     string // "page:<id>"
	ws      string // "" for one's own (a Quick Draw sketch)
	owner   string
	version int // what's saved now
	kind    string
}

// collabTarget loads :kind/:id and checks the caller may see (or change) it.
func (h *Handlers) collabTarget(c *gin.Context, need string) (collabDoc, bool) {
	kind, id := c.Param("kind"), c.Param("id")
	d := collabDoc{key: kind + ":" + id, kind: kind}
	switch kind {
	case "page":
		p, _, ok := h.pageAccess(c, need)
		if !ok {
			return d, false
		}
		// A locked page isn't edited together: its text would pass readable.
		if p.Sealed {
			c.JSON(http.StatusConflict, gin.H{"error": "a locked page isn't edited together"})
			return d, false
		}
		d.ws, d.version = p.WorkspaceID, p.Version
	case "board":
		b, ok := h.drawingAccess(c, need)
		if !ok {
			return d, false
		}
		d.ws, d.owner, d.version = b.WorkspaceID, b.OwnerID, b.Version
	case "sheet", "diagram": // presence only
		var ws string
		var err error
		if kind == "sheet" {
			var s models.Sheet
			err = h.db.Select("id", "workspace_id").First(&s, "id = ?", id).Error
			ws = s.WorkspaceID
		} else {
			var x models.Diagram
			err = h.db.Select("id", "workspace_id").First(&x, "id = ?", id).Error
			ws = x.WorkspaceID
		}
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return d, false
		}
		if _, _, ok := h.wsAccess(c, ws, models.WSViewer); !ok {
			return d, false
		}
		d.ws = ws
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind: page, board, sheet or diagram"})
		return d, false
	}
	return d, true
}

// tell sends a message to everyone on a document now (and to more).
func (h *Handlers) tell(doc string, msg any, more ...string) {
	b, _ := json.Marshal(msg)
	seen := map[string]bool{}
	for _, uid := range append(h.room.Users(doc), more...) {
		if !seen[uid] {
			seen[uid] = true
			h.events.Publish(uid, string(b))
		}
	}
}

// ---- Presence ----

type presenceRequest struct {
	Client  string `json:"client" binding:"required,max=40"`
	Editing bool   `json:"editing"`
	Leave   bool   `json:"leave"`
}

// presence says the caller's tab is on a document (editing it, or looking);
// with leave, that it left. It answers who's there now, and tells them.
//
//	@Summary	I'm here (presence)
//	@Tags		collab
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		kind	path	string			true	"page, board, sheet or diagram"
//	@Param		id		path	string			true	"Its id"
//	@Param		body	body	presenceRequest	true	"This tab"
//	@Success	200		{array}	collab.Person
//	@Router		/presence/{kind}/{id} [post]
func (h *Handlers) presence(c *gin.Context) {
	d, ok := h.collabTarget(c, models.WSViewer)
	if !ok {
		return
	}
	var req presenceRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	var people []collab.Person
	if req.Leave {
		people = h.room.Leave(d.key, req.Client)
	} else {
		var u models.User
		h.db.Select("id", "username", "display_name").First(&u, "id = ?", uid)
		name := u.DisplayName
		if name == "" {
			name = u.Username
		}
		var changed bool
		people, changed = h.room.Here(d.key, collab.Person{Client: req.Client, UserID: uid, Name: name, Editing: req.Editing})
		if !changed { // a heartbeat: nothing new to tell
			c.JSON(http.StatusOK, people)
			return
		}
	}
	h.tell(d.key, gin.H{"type": "presence", "doc": d.key, "people": people}, uid)
	c.JSON(http.StatusOK, people)
}

// ---- The change log ----

type collabUpdateOut struct {
	Seq    int    `json:"seq"`
	Data   []byte `json:"data"` // base64
	Client string `json:"client"`
}

type collabLog struct {
	Seq     int               `json:"seq"`     // the last change's number (0: none yet)
	Fresh   bool              `json:"fresh"`   // no log yet: start it from what's saved
	Version int               `json:"version"` // what's saved
	Updates []collabUpdateOut `json:"updates"`
}

// collabSince answers a document's changes after ?since= (all of them from
// 0). (A save made another way started the log over already: collabSaved.)
//
//	@Summary	A document's changes (editing together)
//	@Tags		collab
//	@Produce	json
//	@Security	BearerAuth
//	@Param		kind	path		string	true	"page or board"
//	@Param		id		path		string	true	"Its id"
//	@Param		since	query		int		false	"The last change the tab has"
//	@Success	200		{object}	collabLog
//	@Router		/collab/{kind}/{id} [get]
func (h *Handlers) collabSince(c *gin.Context) {
	d, ok := h.collabLogTarget(c, models.WSViewer)
	if !ok {
		return
	}
	since, _ := strconv.Atoi(c.Query("since"))
	var rows []models.CollabUpdate
	h.db.Where("doc_key = ? AND seq > ?", d.key, since).Order("seq").Find(&rows)
	out := collabLog{Version: d.version, Updates: make([]collabUpdateOut, 0, len(rows))}
	for _, r := range rows {
		out.Updates = append(out.Updates, collabUpdateOut{Seq: r.Seq, Data: r.Data, Client: r.Client})
	}
	h.db.Model(&models.CollabUpdate{}).Select("COALESCE(MAX(seq), 0)").Where("doc_key = ?", d.key).Scan(&out.Seq)
	out.Fresh = out.Seq == 0
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) collabLogTarget(c *gin.Context, need string) (collabDoc, bool) {
	if k := c.Param("kind"); k != "page" && k != "board" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind: page or board"})
		return collabDoc{}, false
	}
	return h.collabTarget(c, need)
}

type collabPost struct {
	Client string `json:"client" binding:"required,max=40"`
	Data   []byte `json:"data"` // base64
}

// collabAdd keeps a change to a document and sends it to whoever is there.
//
//	@Summary	A change (editing together)
//	@Tags		collab
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		kind	path		string		true	"page or board"
//	@Param		id		path		string		true	"Its id"
//	@Param		body	body		collabPost	true	"The change (base64)"
//	@Success	200		{object}	map[string]int
//	@Router		/collab/{kind}/{id} [post]
func (h *Handlers) collabAdd(c *gin.Context) {
	d, ok := h.collabLogTarget(c, models.WSEditor)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCollabUpdate*2)
	var req collabPost
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Data) == 0 || len(req.Data) > maxCollabUpdate {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a change between 1 byte and 2 MB"})
		return
	}
	var seq int
	err := h.db.Transaction(func(tx *gorm.DB) error {
		tx.Model(&models.CollabUpdate{}).Select("COALESCE(MAX(seq), 0)").Where("doc_key = ?", d.key).Scan(&seq)
		seq++
		if seq == 1 { // a new log: from what's saved now
			if err := tx.Save(&models.CollabDoc{DocKey: d.key, Version: d.version}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.CollabUpdate{DocKey: d.key, Seq: seq, Data: req.Data, Client: req.Client}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "try again"})
			return
		}
		h.internalError(c, err, "could not keep the change")
		return
	}
	h.room.Seeded(d.key)
	msg := gin.H{"type": "collab", "doc": d.key, "seq": seq, "client": req.Client}
	if len(req.Data) <= 12_000 { // small: in the event itself (else, fetched)
		msg["data"] = base64.StdEncoding.EncodeToString(req.Data)
	}
	h.tell(d.key, msg)
	c.JSON(http.StatusOK, gin.H{"seq": seq})
}

// collabSeed lets one tab start an empty document's log from what's saved
// (the others wait for its first change).
//
//	@Summary	May I start it? (editing together)
//	@Tags		collab
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		kind	path		string			true	"page or board"
//	@Param		id		path		string			true	"Its id"
//	@Param		body	body		presenceRequest	true	"This tab"
//	@Success	200		{object}	map[string]bool
//	@Router		/collab/{kind}/{id}/seed [post]
func (h *Handlers) collabSeed(c *gin.Context) {
	d, ok := h.collabLogTarget(c, models.WSEditor)
	if !ok {
		return
	}
	var req presenceRequest
	if !bindJSON(c, &req) {
		return
	}
	var n int64
	h.db.Model(&models.CollabUpdate{}).Where("doc_key = ?", d.key).Count(&n)
	c.JSON(http.StatusOK, gin.H{"go": n == 0 && h.room.Seed(d.key, req.Client)})
}

// collabAwareness sends a tab's cursor and selection to whoever is there
// (not kept).
//
//	@Summary	My cursor (editing together)
//	@Tags		collab
//	@Accept		json
//	@Security	BearerAuth
//	@Param		kind	path	string		true	"page or board"
//	@Param		id		path	string		true	"Its id"
//	@Param		body	body	collabPost	true	"The awareness update (base64)"
//	@Success	204
//	@Router		/collab/{kind}/{id}/awareness [post]
func (h *Handlers) collabAwareness(c *gin.Context) {
	d, ok := h.collabLogTarget(c, models.WSViewer)
	if !ok {
		return
	}
	var req collabPost
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Data) > 64<<10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too large"})
		return
	}
	h.tell(d.key, gin.H{"type": "awareness", "doc": d.key, "client": req.Client, "data": base64.StdEncoding.EncodeToString(req.Data)})
	c.Status(http.StatusNoContent)
}

type collabCompact struct {
	Client string `json:"client" binding:"required,max=40"`
	Upto   int    `json:"upto" binding:"min=1"`
	Data   []byte `json:"data"` // the sum of the changes up to Upto
}

// collabCompactLog replaces a document's first changes (up to upto) with
// their sum, so a tab that joins reads one, not hundreds.
//
//	@Summary	Fold the log (editing together)
//	@Tags		collab
//	@Accept		json
//	@Security	BearerAuth
//	@Param		kind	path	string			true	"page or board"
//	@Param		id		path	string			true	"Its id"
//	@Param		body	body	collabCompact	true	"Up to which change, and their sum"
//	@Success	204
//	@Router		/collab/{kind}/{id}/compact [post]
func (h *Handlers) collabCompactLog(c *gin.Context) {
	d, ok := h.collabLogTarget(c, models.WSEditor)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCollabUpdate*4)
	var req collabCompact
	if !bindJSON(c, &req) {
		return
	}
	if len(req.Data) == 0 || len(req.Data) > maxCollabUpdate*2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the sum is empty or too large"})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var have int64
		tx.Model(&models.CollabUpdate{}).Where("doc_key = ? AND seq = ?", d.key, req.Upto).Count(&have)
		if have == 0 {
			return errors.New("no such change")
		}
		if err := tx.Where("doc_key = ? AND seq <= ?", d.key, req.Upto).Delete(&models.CollabUpdate{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.CollabUpdate{DocKey: d.key, Seq: req.Upto, Data: req.Data, Client: req.Client}).Error
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// collabReset starts a document's log over (saved another way).
func (h *Handlers) collabReset(key string) {
	h.db.Where("doc_key = ?", key).Delete(&models.CollabUpdate{})
	h.db.Where("doc_key = ?", key).Delete(&models.CollabDoc{})
}

// collabSaved follows a save of a page or a whiteboard: by the tab saving
// for everyone (X-Orbit-Collab), the log is in step with it; any other way,
// the log starts over and the tabs there begin again from what's saved.
func (h *Handlers) collabSaved(c *gin.Context, key string, version int) {
	if c.GetHeader("X-Orbit-Collab") == "1" {
		h.db.Model(&models.CollabDoc{}).Where("doc_key = ?", key).Update("version", version)
		return
	}
	var n int64
	h.db.Model(&models.CollabUpdate{}).Where("doc_key = ?", key).Count(&n)
	if n == 0 {
		return
	}
	h.collabReset(key)
	h.tell(key, gin.H{"type": "collab", "doc": key, "reset": true})
}

// collabKeys are the log keys of some documents of a kind.
func collabKeys(kind string, ids []string) []string {
	out := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		out = append(out, kind+":"+id)
	}
	return append(out, "-")
}

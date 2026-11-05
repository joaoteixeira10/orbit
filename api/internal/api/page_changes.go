package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Undo for page structure: each person's own recent changes to pages —
// made, moved, renamed, copied, put in the trash — kept for two weeks, each
// with what it was before, so it can be undone ("My recent changes"). An
// undo of something that changed since asks first (force).

const changeKeep = 14 * 24 * time.Hour

// pageState is a page as a change found or left it.
type pageState struct {
	ParentID *string `json:"parent_id,omitempty"`
	Rank     float64 `json:"rank,omitempty"`
	Title    string  `json:"title,omitempty"`
	Version  int     `json:"version,omitempty"`
}

func encState(s pageState) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func decState(s string) pageState {
	var x pageState
	_ = json.Unmarshal([]byte(s), &x)
	return x
}

// recordChange keeps one of uid's changes to a page. A rename right after
// another of the same page (within two minutes) is the same change.
func (h *Handlers) recordChange(uid string, p models.Page, kind string, before, after pageState) {
	if kind == "rename" {
		var last models.PageChange
		if h.db.Where("user_id = ? AND page_id = ? AND kind = ? AND undone_at IS NULL", uid, p.ID, "rename").Order("created_at DESC").Limit(1).Find(&last).RowsAffected == 1 &&
			time.Since(last.CreatedAt) < 2*time.Minute {
			h.db.Model(&models.PageChange{}).Where("id = ?", last.ID).Update("after", encState(after))
			return
		}
	}
	h.db.Create(&models.PageChange{WorkspaceID: p.WorkspaceID, UserID: uid, PageID: p.ID, Kind: kind, Before: encState(before), After: encState(after)})
	h.db.Where("created_at < ?", time.Now().Add(-changeKeep)).Delete(&models.PageChange{})
}

type pageChangeResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	PageID      string  `json:"page_id"`
	Kind        string  `json:"kind"` // create | move | rename | copy | trash
	Title       string  `json:"title"`
	Icon        string  `json:"icon,omitempty"`
	FromTitle   string  `json:"from_title,omitempty"` // a rename's old title
	FromParent  *string `json:"from_parent,omitempty"`
	ToParent    *string `json:"to_parent,omitempty"`
	ToTitle     string  `json:"to_title,omitempty"` // a move's new parent's title
	At          string  `json:"at"`
	Undone      bool    `json:"undone"`
	Changed     bool    `json:"changed"` // changed since: undoing asks first
	Gone        bool    `json:"gone"`    // the page is deleted for good
}

// changeState judges a change now: whether the page changed since, and
// whether it's gone for good.
func (h *Handlers) changeState(ch models.PageChange) (models.Page, bool, bool) {
	var p models.Page
	if h.db.First(&p, "id = ?", ch.PageID).Error != nil {
		return p, false, true
	}
	after := decState(ch.After)
	changed := false
	switch ch.Kind {
	case "create", "copy":
		changed = p.TrashedAt != nil || p.Version > max(after.Version, 1) || p.Title != after.Title
	case "move":
		changed = !sameParent(p.ParentID, after.ParentID)
	case "rename":
		changed = p.Title != after.Title
	case "trash":
		changed = p.TrashedAt == nil
	}
	return p, changed, false
}

func sameParent(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (h *Handlers) changeOut(ch models.PageChange) pageChangeResponse {
	p, changed, gone := h.changeState(ch)
	before, after := decState(ch.Before), decState(ch.After)
	out := pageChangeResponse{ID: ch.ID, WorkspaceID: ch.WorkspaceID, PageID: ch.PageID, Kind: ch.Kind, Title: p.Title, Icon: p.Icon, At: stamp(ch.CreatedAt), Undone: ch.UndoneAt != nil, Changed: changed, Gone: gone}
	if gone {
		out.Title = after.Title
	}
	if ch.Kind == "rename" {
		out.FromTitle, out.Title = before.Title, after.Title
	}
	if ch.Kind == "move" {
		out.FromParent, out.ToParent = before.ParentID, after.ParentID
		if after.ParentID != nil {
			var parent models.Page
			if h.db.Select("title").First(&parent, "id = ?", *after.ParentID).Error == nil {
				out.ToTitle = parent.Title
			}
		}
	}
	return out
}

// listPageChanges lists the caller's recent changes to pages (newest first;
// one workspace's with ?ws=).
//
//	@Summary	My recent changes to pages
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	query	string	false	"Only this workspace"
//	@Success	200	{array}	pageChangeResponse
//	@Router		/me/page-changes [get]
func (h *Handlers) listPageChanges(c *gin.Context) {
	uid := auth.UserID(c)
	q := h.db.Where("user_id = ? AND created_at > ?", uid, time.Now().Add(-changeKeep))
	if ws := c.Query("ws"); ws != "" {
		q = q.Where("workspace_id = ?", ws)
	} else {
		q = q.Where("workspace_id IN ?", append(h.myWorkspaceIDs(c, true), ""))
	}
	var chs []models.PageChange
	q.Order("created_at DESC").Limit(40).Find(&chs)
	out := make([]pageChangeResponse, 0, len(chs))
	for _, ch := range chs {
		out = append(out, h.changeOut(ch))
	}
	c.JSON(http.StatusOK, out)
}

// undoPageChange undoes one of the caller's changes: a page made or copied
// goes to the trash, a move goes back, a rename too, a page put in the
// trash comes out. Something that changed since is refused (409, changed)
// unless ?force=1.
//
//	@Summary	Undo a change to a page
//	@Tags		pages
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Change id"
//	@Param		force	query		bool	false	"Even if it changed since"
//	@Success	200		{object}	pageChangeResponse
//	@Router		/page-changes/{id}/undo [post]
func (h *Handlers) undoPageChange(c *gin.Context) {
	uid := auth.UserID(c)
	var ch models.PageChange
	if h.db.First(&ch, "id = ? AND user_id = ?", c.Param("id"), uid).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "change not found"})
		return
	}
	if _, _, ok := h.wsAccess(c, ch.WorkspaceID, models.WSEditor); !ok {
		return
	}
	if ch.UndoneAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "already undone"})
		return
	}
	p, changed, gone := h.changeState(ch)
	if gone || !h.pageVisible(c, p) {
		c.JSON(http.StatusConflict, gin.H{"error": "the page is deleted for good"})
		return
	}
	if changed && c.Query("force") != "1" {
		c.JSON(http.StatusConflict, gin.H{"error": "the page changed since", "changed": true})
		return
	}
	before := decState(ch.Before)
	switch ch.Kind {
	case "create", "copy":
		if p.TrashedAt == nil {
			now := time.Now()
			h.db.Model(&models.Page{}).Where("id IN ? AND trashed_at IS NULL", append(descendants(h.db, p.ID), p.ID)).Update("trashed_at", &now)
		}
	case "move":
		parent := before.ParentID
		if parent != nil {
			var up models.Page
			if h.db.Select("id", "trashed_at").First(&up, "id = ?", *parent).Error != nil || up.TrashedAt != nil || h.wouldLoop(p.ID, *parent) {
				parent = nil // its old place is gone: the top
			}
		}
		h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(map[string]any{"parent_id": parent, "rank": before.Rank})
	case "rename":
		prev := p
		h.db.Model(&models.Page{}).Where("id = ?", p.ID).Updates(map[string]any{"title": before.Title, "updated_by": uid})
		recordVersion(h.db, reloadPage(h, p.ID), uid, viaOf(c), &prev, 0)
	case "trash":
		h.untrash(p)
	}
	now := time.Now()
	h.db.Model(&models.PageChange{}).Where("id = ?", ch.ID).Update("undone_at", &now)
	ch.UndoneAt = &now
	h.publishWS(c, ch.WorkspaceID, "pages")
	c.JSON(http.StatusOK, h.changeOut(ch))
}

// wouldLoop: would putting page id under parent put it inside itself?
func (h *Handlers) wouldLoop(id, parent string) bool {
	for cur, n := &parent, 0; cur != nil && n < 1000; n++ {
		if *cur == id {
			return true
		}
		var up models.Page
		if h.db.Select("id", "parent_id").First(&up, "id = ?", *cur).Error != nil {
			return false
		}
		cur = up.ParentID
	}
	return false
}

// untrash takes a page and what was trashed with it out of the trash; a
// page whose parent is still in the trash comes back at the top.
func (h *Handlers) untrash(p models.Page) {
	if p.TrashedAt == nil {
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
}

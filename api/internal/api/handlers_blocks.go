package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Time blocks: time a person set aside for their tasks (the week planner
// makes them). Each is its maker's alone; their calendar shows them. A plan
// replaces their blocks over a range, in the workspaces planned.

const (
	maxBlocks     = 300
	maxBlockRange = 32 * 24 * time.Hour
	maxBlockLen   = 12 * time.Hour
)

type blockResponse struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	WorkspaceID string    `json:"workspace_id"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Title       string    `json:"title"`
	Done        bool      `json:"done"`
	Color       string    `json:"color"` // its list's
}

func blockRange(c *gin.Context, from, to string) (time.Time, time.Time, bool) {
	f, err1 := time.Parse(time.RFC3339, from)
	t, err2 := time.Parse(time.RFC3339, to)
	if err1 != nil || err2 != nil || !t.After(f) || t.Sub(f) > maxBlockRange {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must be RFC 3339 times, a month apart at most"})
		return f, t, false
	}
	return f.UTC(), t.UTC(), true
}

// listBlocks lists my time blocks that overlap [from, to), with their tasks'
// titles (in the workspaces I'm still in).
//
//	@Summary	My time blocks
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		from	query	string	true	"RFC 3339"
//	@Param		to		query	string	true	"RFC 3339"
//	@Success	200		{array}	blockResponse
//	@Router		/me/blocks [get]
func (h *Handlers) listBlocks(c *gin.Context) {
	from, to, ok := blockRange(c, c.Query("from"), c.Query("to"))
	if !ok {
		return
	}
	uid := auth.UserID(c)
	var blocks []models.TaskBlock
	h.db.Where("user_id = ? AND starts_at < ? AND ends_at > ?", uid, to, from).
		Where("workspace_id IN (?)", h.db.Model(&models.WorkspaceMember{}).Select("workspace_id").Where("user_id = ?", uid)).
		Order("starts_at").Find(&blocks)
	out := make([]blockResponse, 0, len(blocks))
	if len(blocks) == 0 {
		c.JSON(http.StatusOK, out)
		return
	}
	ids := make([]string, 0, len(blocks))
	for _, b := range blocks {
		ids = append(ids, b.TaskID)
	}
	var tasks []models.Task
	h.db.Select("id", "title", "completed_at", "list_id").Where("id IN ?", ids).Find(&tasks)
	byID := map[string]models.Task{}
	listIDs := []string{}
	for _, t := range tasks {
		byID[t.ID] = t
		listIDs = append(listIDs, t.ListID)
	}
	var lists []models.TaskList
	h.db.Select("id", "color").Where("id IN ?", append(listIDs, "")).Find(&lists)
	color := map[string]string{}
	for _, l := range lists {
		color[l.ID] = l.Color
	}
	for _, b := range blocks {
		t, ok := byID[b.TaskID]
		if !ok {
			continue
		}
		out = append(out, blockResponse{ID: b.ID, TaskID: b.TaskID, WorkspaceID: b.WorkspaceID, StartsAt: b.StartsAt, EndsAt: b.EndsAt, Title: t.Title, Done: t.CompletedAt != nil, Color: color[t.ListID]})
	}
	c.JSON(http.StatusOK, out)
}

type planRequest struct {
	From         string   `json:"from" binding:"required"`
	To           string   `json:"to" binding:"required"`
	WorkspaceIDs []string `json:"workspace_ids" binding:"required,min=1,max=100"`
	Blocks       []struct {
		TaskID   string    `json:"task_id" binding:"required"`
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
	} `json:"blocks" binding:"max=300"`
}

// planBlocks replaces my time blocks within [from, to) in the workspaces
// planned with the ones given (a week planned; none clears it).
//
//	@Summary	Save a plan of time blocks
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body	planRequest	true	"The range, the workspaces, the blocks"
//	@Success	200		{array}	blockResponse
//	@Router		/me/blocks [put]
func (h *Handlers) planBlocks(c *gin.Context) {
	var req planRequest
	if !bindJSON(c, &req) {
		return
	}
	from, to, ok := blockRange(c, req.From, req.To)
	if !ok {
		return
	}
	uid := auth.UserID(c)
	for _, ws := range req.WorkspaceIDs {
		if _, _, ok := h.wsAccess(c, ws, models.WSViewer); !ok {
			return
		}
	}
	ids := make([]string, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		ids = append(ids, b.TaskID)
	}
	var tasks []models.Task
	h.db.Select("id", "workspace_id").Where("id IN ? AND workspace_id IN ?", append(ids, ""), req.WorkspaceIDs).Find(&tasks)
	wsOf := map[string]string{}
	for _, t := range tasks {
		wsOf[t.ID] = t.WorkspaceID
	}
	blocks := make([]models.TaskBlock, 0, len(req.Blocks))
	for _, b := range req.Blocks {
		ws, ok := wsOf[b.TaskID]
		s, e := b.StartsAt.UTC(), b.EndsAt.UTC()
		switch {
		case !ok:
			c.JSON(http.StatusBadRequest, gin.H{"error": "a block's task isn't in the workspaces planned"})
			return
		case !e.After(s) || e.Sub(s) > maxBlockLen:
			c.JSON(http.StatusBadRequest, gin.H{"error": "a block ends after it starts, 12 hours at most"})
			return
		case s.Before(from) || e.After(to):
			c.JSON(http.StatusBadRequest, gin.H{"error": "a block is outside the range planned"})
			return
		}
		blocks = append(blocks, models.TaskBlock{WorkspaceID: ws, TaskID: b.TaskID, UserID: uid, StartsAt: s, EndsAt: e})
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND workspace_id IN ? AND starts_at >= ? AND starts_at < ?", uid, req.WorkspaceIDs, from, to).Delete(&models.TaskBlock{}).Error; err != nil {
			return err
		}
		if len(blocks) == 0 {
			return nil
		}
		return tx.Create(&blocks).Error
	})
	if err != nil {
		h.internalError(c, err, "could not save the plan")
		return
	}
	h.publishBlocks(c, uid)
	c.Request.URL.RawQuery = "from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)
	h.listBlocks(c)
}

// deleteBlock removes one of my time blocks.
//
//	@Summary	Remove a time block
//	@Tags		tasks
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Block id"
//	@Success	204
//	@Router		/me/blocks/{id} [delete]
func (h *Handlers) deleteBlock(c *gin.Context) {
	uid := auth.UserID(c)
	res := h.db.Where("id = ? AND user_id = ?", c.Param("id"), uid).Delete(&models.TaskBlock{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "block not found"})
		return
	}
	h.publishBlocks(c, uid)
	c.Status(http.StatusNoContent)
}

// publishBlocks tells my other windows my plan changed.
func (h *Handlers) publishBlocks(c *gin.Context, uid string) {
	if h.events == nil {
		return
	}
	b, _ := json.Marshal(gin.H{"type": "ws", "ws": "me", "what": "blocks", "origin": c.GetHeader("X-Orbit-Client")})
	h.events.Publish(uid, string(b))
}

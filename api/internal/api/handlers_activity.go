package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// A task's activity, as in Plane: who created it, renamed it, gave it a
// date or a priority, assigned whom, moved it, completed or reopened it. The
// details show it among the comments. Changes made by the git sync aren't
// recorded here (git has their history).

// recordTask writes the lines for what changed from before to after (before
// nil: it was just created). Several notes edits by one person within ten
// minutes count once.
func (h *Handlers) recordTask(c *gin.Context, before *models.Task, after models.Task) {
	uid := auth.UserID(c)
	add := func(kind string, params map[string]any) {
		b, _ := json.Marshal(params)
		h.db.Create(&models.TaskEvent{TaskID: after.ID, WorkspaceID: after.WorkspaceID, ActorID: uid, Kind: kind, Params: string(b)})
	}
	if before == nil {
		add("created", nil)
		if ids := after.AssigneeList(); len(ids) > 0 {
			add("assigned", map[string]any{"names": h.namesOf(ids)})
		}
		return
	}
	b := *before
	if b.Title != after.Title {
		add("renamed", map[string]any{"from": b.Title, "to": after.Title})
	}
	if str(b.Due) != str(after.Due) || str(b.DueTime) != str(after.DueTime) {
		add("due", map[string]any{"date": str(after.Due), "time": str(after.DueTime)})
	}
	if b.Priority != after.Priority {
		add("priority", map[string]any{"to": after.Priority})
	}
	was, now := map[string]bool{}, map[string]bool{}
	for _, id := range b.AssigneeList() {
		was[id] = true
	}
	var added, removed []string
	for _, id := range after.AssigneeList() {
		now[id] = true
		if !was[id] {
			added = append(added, id)
		}
	}
	for id := range was {
		if !now[id] {
			removed = append(removed, id)
		}
	}
	if len(added) > 0 {
		add("assigned", map[string]any{"names": h.namesOf(added)})
	}
	if len(removed) > 0 {
		add("unassigned", map[string]any{"names": h.namesOf(removed)})
	}
	switch {
	case b.CompletedAt == nil && after.CompletedAt != nil:
		add("completed", nil)
	case b.CompletedAt != nil && after.CompletedAt == nil:
		add("reopened", nil)
	case b.ListID != after.ListID:
		var l models.TaskList
		h.db.Select("id", "name").First(&l, "id = ?", after.ListID)
		add("moved", map[string]any{"list": l.Name})
	case b.ColumnID != after.ColumnID && after.ParentID == nil:
		var l models.TaskList
		h.db.First(&l, "id = ?", after.ListID)
		if col, ok := planner.FindColumn(planner.DecodeColumns(l.Columns), after.ColumnID); ok {
			add("status", map[string]any{"to": col.Name})
		}
	}
	if b.Notes != after.Notes {
		var n int64
		h.db.Model(&models.TaskEvent{}).Where("task_id = ? AND actor_id = ? AND kind = ? AND created_at > ?", after.ID, uid, "notes", time.Now().Add(-10*time.Minute)).Count(&n)
		if n == 0 {
			add("notes", nil)
		}
	}
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// namesOf are people's display names (or user names), in order.
func (h *Handlers) namesOf(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, h.actorName(id))
	}
	return out
}

type activityResponse struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	ActorID   string         `json:"actor_id"`
	ActorName string         `json:"actor_name"`
	Params    map[string]any `json:"params"`
	CreatedAt time.Time      `json:"created_at"`
}

// taskActivity lists a task's activity, oldest first.
//
//	@Summary	A task's activity
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Task id"
//	@Success	200	{array}	activityResponse
//	@Router		/tasks/{id}/activity [get]
func (h *Handlers) taskActivity(c *gin.Context) {
	t, _, ok := h.commentTask(c, false)
	if !ok {
		return
	}
	var evs []models.TaskEvent
	h.db.Where("task_id = ?", t.ID).Order("created_at").Limit(500).Find(&evs)
	names := map[string]string{}
	out := make([]activityResponse, 0, len(evs))
	for _, e := range evs {
		if _, ok := names[e.ActorID]; !ok {
			names[e.ActorID] = h.actorName(e.ActorID)
		}
		p := map[string]any{}
		if e.Params != "" && e.Params != "null" {
			_ = json.Unmarshal([]byte(e.Params), &p)
		}
		out = append(out, activityResponse{ID: e.ID, Kind: e.Kind, ActorID: e.ActorID, ActorName: names[e.ActorID], Params: p, CreatedAt: e.CreatedAt})
	}
	c.JSON(http.StatusOK, out)
}

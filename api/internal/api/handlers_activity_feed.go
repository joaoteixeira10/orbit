package api

import (
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// A workspace's activity feed: who added, completed, commented, joined —
// the same events as the activity notifications, kept 90 days, for every
// member to read (muting a workspace mutes its notifications, not this).

// feed records one line of a workspace's activity.
func (h *Handlers) feed(wsID, actor, verb, title, link string) {
	if len([]rune(title)) > 300 {
		title = string([]rune(title)[:299]) + "…"
	}
	h.db.Create(&models.Activity{WorkspaceID: wsID, ActorID: actor, Verb: verb, Title: title, Link: link})
}

type activityLine struct {
	models.Activity
	ActorName   string `json:"actor_name"`
	ActorAvatar string `json:"actor_avatar"`
}

// workspaceActivity lists a workspace's activity, newest first
// (?before=<RFC 3339> for older ones).
//
//	@Summary	A workspace's activity
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string	true	"Workspace id"
//	@Param		before	query	string	false	"RFC 3339: older than this"
//	@Success	200		{array}	activityLine
//	@Router		/workspaces/{ws}/activity [get]
func (h *Handlers) workspaceActivity(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	q := h.db.Where("workspace_id = ?", ws.ID)
	if b, err := time.Parse(time.RFC3339Nano, c.Query("before")); err == nil {
		q = q.Where("created_at < ?", b)
	}
	var rows []models.Activity
	q.Order("created_at DESC").Limit(60).Find(&rows)
	type who struct{ ID, Username, DisplayName, AvatarURL string }
	ids := []string{}
	for _, r := range rows {
		ids = append(ids, r.ActorID)
	}
	var users []who
	if len(ids) > 0 {
		h.db.Model(&models.User{}).Select("id, username, display_name, avatar_url").Where("id IN ?", ids).Scan(&users)
	}
	byID := map[string]who{}
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]activityLine, 0, len(rows))
	for _, r := range rows {
		u := byID[r.ActorID]
		name := u.DisplayName
		if name == "" {
			name = u.Username
		}
		out = append(out, activityLine{Activity: r, ActorName: name, ActorAvatar: u.AvatarURL})
	}
	c.JSON(http.StatusOK, out)
}

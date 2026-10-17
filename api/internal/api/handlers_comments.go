package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Comments on tasks, as in Microsoft Planner: any member writes them (viewers
// too), in Markdown; their author changes or deletes them, an owner deletes
// any. A new comment tells the task's assignees, its creator and those who
// commented before; @username tells that member, whoever they are.

type commentResponse struct {
	models.TaskComment
	AuthorName   string `json:"author_name"`
	AuthorAvatar string `json:"author_avatar"`
}

func (h *Handlers) commentResponses(cs []models.TaskComment) []commentResponse {
	ids := make([]string, 0, len(cs))
	for _, c := range cs {
		ids = append(ids, c.AuthorID)
	}
	type who struct{ ID, Username, DisplayName, AvatarURL string }
	var users []who
	if len(ids) > 0 {
		h.db.Model(&models.User{}).Select("id, username, display_name, avatar_url").Where("id IN ?", ids).Scan(&users)
	}
	byID := map[string]who{}
	for _, u := range users {
		byID[u.ID] = u
	}
	out := make([]commentResponse, 0, len(cs))
	for _, c := range cs {
		u := byID[c.AuthorID]
		name := u.DisplayName
		if name == "" {
			name = u.Username
		}
		out = append(out, commentResponse{TaskComment: c, AuthorName: name, AuthorAvatar: u.AvatarURL})
	}
	return out
}

// commentTask loads the task :id and checks the caller is a member (need:
// viewer to read; to write, any member but not through a read-only token or
// in an archived workspace).
func (h *Handlers) commentTask(c *gin.Context, write bool) (models.Task, models.Workspace, bool) {
	var t models.Task
	if h.db.First(&t, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return t, models.Workspace{}, false
	}
	if write {
		ws, ok := h.selfAccess(c, t.WorkspaceID)
		return t, ws, ok
	}
	ws, _, ok := h.wsAccess(c, t.WorkspaceID, models.WSViewer)
	return t, ws, ok
}

// listComments lists a task's comments, oldest first.
//
//	@Summary	Comments on a task
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Task id"
//	@Success	200	{array}	commentResponse
//	@Router		/tasks/{id}/comments [get]
func (h *Handlers) listComments(c *gin.Context) {
	t, _, ok := h.commentTask(c, false)
	if !ok {
		return
	}
	var cs []models.TaskComment
	h.db.Where("task_id = ?", t.ID).Order("created_at").Limit(1000).Find(&cs)
	c.JSON(http.StatusOK, h.commentResponses(cs))
}

type commentRequest struct {
	Body string `json:"body" binding:"required,max=10000"`
}

// createComment adds a comment to a task (any member).
//
//	@Summary	Comment on a task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Task id"
//	@Param		body	body		commentRequest	true	"The comment (Markdown)"
//	@Success	201		{object}	commentResponse
//	@Router		/tasks/{id}/comments [post]
func (h *Handlers) createComment(c *gin.Context) {
	t, ws, ok := h.commentTask(c, true)
	if !ok {
		return
	}
	var req commentRequest
	if !bindJSON(c, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "write something first"})
		return
	}
	cm := models.TaskComment{TaskID: t.ID, WorkspaceID: t.WorkspaceID, AuthorID: auth.UserID(c), Body: body}
	if err := h.db.Create(&cm).Error; err != nil {
		h.internalError(c, err, "could not add the comment")
		return
	}
	h.publishWS(c, ws.ID, "comments")
	h.notifyComment(c, ws, t, cm)
	c.JSON(http.StatusCreated, h.commentResponses([]models.TaskComment{cm})[0])
}

var mentionRe = regexp.MustCompile(`(?:^|[^\w@.])@([a-z0-9][a-z0-9._-]{1,31})`)

// mentions are the members of ws named @username in body.
func (h *Handlers) mentions(wsID, body string) []string {
	var names []string
	for _, m := range mentionRe.FindAllStringSubmatch(strings.ToLower(body), 20) {
		names = append(names, strings.TrimRight(m[1], "."))
	}
	if len(names) == 0 {
		return nil
	}
	var ids []string
	h.db.Table("workspace_members").Joins("JOIN users ON users.id = workspace_members.user_id").
		Where("workspace_members.workspace_id = ? AND users.username IN ?", wsID, names).Pluck("users.id", &ids)
	return ids
}

// excerpt is a comment's first words, for a notification.
func excerpt(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		return string(r[:119]) + "…"
	}
	return s
}

// notifyComment tells those concerned about a new comment: whoever it
// mentions ("mentioned"), and the task's assignees, its creator and earlier
// commenters ("task_comment", one entry per task while unread).
func (h *Handlers) notifyComment(c *gin.Context, ws models.Workspace, t models.Task, cm models.TaskComment) {
	uid := auth.UserID(c)
	actor := h.actorName(uid)
	params := map[string]any{"actor": actor, "title": t.Title, "ws": ws.Name, "task": t.ID, "excerpt": excerpt(cm.Body)}
	told := map[string]bool{uid: true}
	for _, to := range h.mentions(ws.ID, cm.Body) {
		if told[to] {
			continue
		}
		told[to] = true
		h.inbox.Notify(inbox.Message{UserID: to, Kind: inbox.Comment, Verb: "mentioned", WorkspaceID: ws.ID, ActorID: uid, Params: params, Link: taskLink(t)})
	}
	followers := append(t.AssigneeList(), t.CreatedBy)
	var earlier []string
	h.db.Model(&models.TaskComment{}).Where("task_id = ? AND id <> ?", t.ID, cm.ID).Distinct("author_id").Pluck("author_id", &earlier)
	followers = append(followers, earlier...)
	members := map[string]bool{}
	for _, id := range h.memberIDs(ws.ID) {
		members[id] = true
	}
	for _, to := range followers {
		if told[to] || !members[to] {
			continue
		}
		told[to] = true
		h.inbox.Notify(inbox.Message{
			UserID: to, Kind: inbox.Comment, Verb: "task_comment", WorkspaceID: ws.ID, ActorID: uid,
			Params: params, Link: taskLink(t), Group: "comment:" + t.ID,
		})
	}
}

// commentAccess loads :id and checks the caller may change it (its author)
// or delete it (its author, or an owner of the workspace).
func (h *Handlers) commentAccess(c *gin.Context, deleting bool) (models.TaskComment, bool) {
	var cm models.TaskComment
	if h.db.First(&cm, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return cm, false
	}
	if _, ok := h.selfAccess(c, cm.WorkspaceID); !ok {
		return cm, false
	}
	if cm.AuthorID == auth.UserID(c) {
		return cm, true
	}
	if deleting {
		var m models.WorkspaceMember
		if h.db.Where("workspace_id = ? AND user_id = ?", cm.WorkspaceID, auth.UserID(c)).First(&m).Error == nil && m.Role == models.WSOwner {
			return cm, true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "only its author can change a comment (owners can delete it)"})
	return cm, false
}

// updateComment changes the caller's own comment.
//
//	@Summary	Edit a comment
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Comment id"
//	@Param		body	body		commentRequest	true	"The new text"
//	@Success	200		{object}	commentResponse
//	@Router		/comments/{id} [put]
func (h *Handlers) updateComment(c *gin.Context) {
	cm, ok := h.commentAccess(c, false)
	if !ok {
		return
	}
	var req commentRequest
	if !bindJSON(c, &req) {
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a comment can't be empty (delete it instead)"})
		return
	}
	if body != cm.Body {
		now := time.Now()
		cm.Body, cm.EditedAt = body, &now
		if err := h.db.Save(&cm).Error; err != nil {
			h.internalError(c, err, "could not change the comment")
			return
		}
		h.publishWS(c, cm.WorkspaceID, "comments")
	}
	c.JSON(http.StatusOK, h.commentResponses([]models.TaskComment{cm})[0])
}

// deleteComment deletes a comment (its author, or an owner).
//
//	@Summary	Delete a comment
//	@Tags		tasks
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Comment id"
//	@Success	204
//	@Router		/comments/{id} [delete]
func (h *Handlers) deleteComment(c *gin.Context) {
	cm, ok := h.commentAccess(c, true)
	if !ok {
		return
	}
	h.db.Delete(&cm)
	if cm.AuthorID != auth.UserID(c) {
		h.recordCtx(c, "comment.delete", "workspace", cm.WorkspaceID, gin.H{"task": cm.TaskID, "author": cm.AuthorID})
	}
	h.publishWS(c, cm.WorkspaceID, "comments")
	c.Status(http.StatusNoContent)
}

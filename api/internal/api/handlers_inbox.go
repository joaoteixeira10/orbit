package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// ---- Sending ----------------------------------------------------------------

// actorName is how a user is named in notifications.
func (h *Handlers) actorName(uid string) string {
	var u models.User
	if h.db.Select("id", "username", "display_name").First(&u, "id = ?", uid).Error != nil {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

// workspaceOf loads a workspace's id and name (for notifications).
func (h *Handlers) workspaceOf(id string) models.Workspace {
	var ws models.Workspace
	h.db.Select("id", "name").First(&ws, "id = ?", id)
	return ws
}

// notifyActivity tells the other members of a shared workspace what the
// caller did (unless they muted it). A burst of the same thing by the same
// person folds into one notification.
// Members in except (e.g. the one a new task is assigned to, who hears of it
// that way) are left out.
func (h *Handlers) notifyActivity(c *gin.Context, ws models.Workspace, verb, title, link string, except ...string) {
	uid := auth.UserID(c)
	h.feed(ws.ID, uid, verb, title, link)
	var others []models.WorkspaceMember
	h.db.Where("workspace_id = ? AND user_id NOT IN ? AND NOT muted", ws.ID, append(except, uid)).Find(&others)
	if len(others) == 0 {
		return
	}
	actor := h.actorName(uid)
	for _, m := range others {
		h.inbox.Notify(inbox.Message{
			UserID: m.UserID, Kind: inbox.Activity, Verb: verb, WorkspaceID: ws.ID, ActorID: uid,
			Params: map[string]any{"actor": actor, "title": title, "ws": ws.Name},
			Link:   link, Group: fmt.Sprintf("%s:%s:%s", ws.ID, uid, verb),
		})
	}
}

// notifyAssigned tells each person the caller just gave a task to (those
// not among before).
func (h *Handlers) notifyAssigned(c *gin.Context, t models.Task, before []string) {
	uid := auth.UserID(c)
	if t.CompletedAt != nil {
		return
	}
	had := map[string]bool{uid: true}
	for _, id := range before {
		had[id] = true
	}
	var ws models.Workspace
	for _, to := range t.AssigneeList() {
		if had[to] {
			continue
		}
		if ws.ID == "" {
			ws = h.workspaceOf(t.WorkspaceID)
		}
		h.inbox.Notify(inbox.Message{
			UserID: to, Kind: inbox.Assigned, Verb: "task_assigned", WorkspaceID: t.WorkspaceID, ActorID: uid,
			Params: map[string]any{"actor": h.actorName(uid), "title": t.Title, "ws": ws.Name, "task": t.ID},
			Link:   taskLink(t),
		})
	}
}

// notifyMembership tells someone they were added to a workspace, their role
// changed or they were removed (verb added | role_changed | removed).
func (h *Handlers) notifyMembership(c *gin.Context, ws models.Workspace, userID, verb, role string) {
	uid := auth.UserID(c)
	if userID == uid {
		return
	}
	link := "/w/" + ws.ID
	if verb == "removed" {
		link = "/"
	}
	h.inbox.Notify(inbox.Message{
		UserID: userID, Kind: inbox.Workspace, Verb: verb, WorkspaceID: ws.ID, ActorID: uid,
		Params: map[string]any{"actor": h.actorName(uid), "ws": ws.Name, "role": role}, Link: link,
	})
}

// notifyJoined tells the other members (not the one who added them) that
// someone joined a workspace.
func (h *Handlers) notifyJoined(c *gin.Context, ws models.Workspace, u models.User, role string) {
	h.feed(ws.ID, u.ID, "member_joined", "", "/w/"+ws.ID)
	var others []models.WorkspaceMember
	h.db.Where("workspace_id = ? AND user_id NOT IN ? AND NOT muted", ws.ID, []string{u.ID, auth.UserID(c)}).Find(&others)
	name := u.DisplayName
	if name == "" {
		name = u.Username
	}
	for _, m := range others {
		h.inbox.Notify(inbox.Message{
			UserID: m.UserID, Kind: inbox.Activity, Verb: "member_joined", WorkspaceID: ws.ID, ActorID: u.ID,
			Params: map[string]any{"actor": name, "ws": ws.Name, "role": role}, Link: "/w/" + ws.ID,
		})
	}
}

// ---- The inbox --------------------------------------------------------------

type inboxResponse struct {
	Items  []inbox.View `json:"items"`
	Unread int64        `json:"unread"`
	More   bool         `json:"more"`
}

// listNotifications returns the caller's notifications, newest first
// (?before=<RFC 3339 time> pages back), and how many are unread.
//
//	@Summary	My notifications
//	@Tags		notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Param		before	query		string	false	"Only older than this (RFC 3339)"
//	@Param		limit	query		int		false	"At most (default 30, max 100)"
//	@Success	200		{object}	inboxResponse
//	@Router		/me/notifications [get]
func (h *Handlers) listNotifications(c *gin.Context) {
	uid := auth.UserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if limit < 1 || limit > 100 {
		limit = 30
	}
	q := h.db.Where("user_id = ?", uid)
	if b := c.Query("before"); b != "" {
		if t, err := time.Parse(time.RFC3339Nano, b); err == nil {
			q = q.Where("updated_at < ?", t)
		}
	}
	var rows []models.Notification
	q.Order("updated_at DESC").Limit(limit + 1).Find(&rows)
	resp := inboxResponse{Items: []inbox.View{}, More: len(rows) > limit}
	if resp.More {
		rows = rows[:limit]
	}
	for _, n := range rows {
		resp.Items = append(resp.Items, inbox.ToView(n))
	}
	h.db.Model(&models.Notification{}).Where("user_id = ? AND read_at IS NULL", uid).Count(&resp.Unread)
	c.JSON(http.StatusOK, resp)
}

type readRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

// readNotifications marks notifications read (ids, or all).
//
//	@Summary	Mark notifications read
//	@Tags		notifications
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	readRequest	true	"Which"
//	@Success	204
//	@Router		/me/notifications/read [post]
func (h *Handlers) readNotifications(c *gin.Context) {
	var req readRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	q := h.db.Model(&models.Notification{}).Where("user_id = ? AND read_at IS NULL", uid)
	if !req.All {
		if len(req.IDs) == 0 || len(req.IDs) > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "give ids or all"})
			return
		}
		q = q.Where("id IN ?", req.IDs)
	}
	q.Update("read_at", time.Now())
	h.events.Publish(uid, `{"type":"notifications-read"}`)
	c.Status(http.StatusNoContent)
}

// ---- Web Push ---------------------------------------------------------------

type pushInfoResponse struct {
	Available bool                      `json:"available"`
	PublicKey string                    `json:"public_key"`
	Devices   []models.PushSubscription `json:"devices"`
}

// pushInfo returns the key browsers subscribe with and the caller's devices.
//
//	@Summary	Web Push: key and my devices
//	@Tags		notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	pushInfoResponse
//	@Router		/me/push [get]
func (h *Handlers) pushInfo(c *gin.Context) {
	var subs []models.PushSubscription
	h.db.Where("user_id = ?", auth.UserID(c)).Order("created_at").Find(&subs)
	c.JSON(http.StatusOK, pushInfoResponse{Available: h.inbox.PushReady(), PublicKey: h.inbox.PublicKey(), Devices: subs})
}

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint" binding:"required"`
	Keys     struct {
		P256dh string `json:"p256dh" binding:"required,max=200"`
		Auth   string `json:"auth" binding:"required,max=100"`
	} `json:"keys"`
}

// subscribePush registers this browser for Web Push (the same endpoint
// again just updates it, also when another account used it before).
//
//	@Summary	Web Push: add this device
//	@Tags		notifications
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	pushSubscribeRequest	true	"The browser's PushSubscription"
//	@Success	204
//	@Router		/me/push [post]
func (h *Handlers) subscribePush(c *gin.Context) {
	var req pushSubscribeRequest
	if !bindJSON(c, &req) {
		return
	}
	if !inbox.ValidEndpoint(req.Endpoint) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the push endpoint must be an https address"})
		return
	}
	ua := c.Request.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	sub := models.PushSubscription{UserID: auth.UserID(c), Endpoint: req.Endpoint, P256dh: req.Keys.P256dh, Auth: req.Keys.Auth, UserAgent: ua}
	var cur models.PushSubscription
	if h.db.Where("endpoint = ?", req.Endpoint).First(&cur).Error == nil {
		sub.ID, sub.CreatedAt = cur.ID, cur.CreatedAt
	}
	if err := h.db.Save(&sub).Error; err != nil {
		h.internalError(c, err, "could not save the device")
		return
	}
	c.Status(http.StatusNoContent)
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	ID       string `json:"id"`
}

// unsubscribePush removes one of the caller's devices (by endpoint or id).
//
//	@Summary	Web Push: remove a device
//	@Tags		notifications
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	pushUnsubscribeRequest	true	"Which"
//	@Success	204
//	@Router		/me/push/remove [post]
func (h *Handlers) unsubscribePush(c *gin.Context) {
	var req pushUnsubscribeRequest
	if !bindJSON(c, &req) {
		return
	}
	q := h.db.Where("user_id = ?", auth.UserID(c))
	switch {
	case req.ID != "":
		q = q.Where("id = ?", req.ID)
	case req.Endpoint != "":
		q = q.Where("endpoint = ?", req.Endpoint)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the endpoint or id"})
		return
	}
	q.Delete(&models.PushSubscription{})
	c.Status(http.StatusNoContent)
}

// testPush sends a test notification to the caller's devices.
//
//	@Summary	Web Push: send a test
//	@Tags		notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]int
//	@Router		/me/push/test [post]
func (h *Handlers) testPush(c *gin.Context) {
	p := h.loadOrInitPrefs(auth.UserID(c))
	n, err := h.inbox.TestPush(auth.UserID(c), p.Locale)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": n})
}

// ---- Muting a workspace -----------------------------------------------------

type muteRequest struct {
	Muted bool `json:"muted"`
}

// muteWorkspace stops (or resumes) notifications about the other members'
// activity in a workspace, for the caller.
//
//	@Summary	Mute a workspace's activity
//	@Tags		notifications
//	@Accept		json
//	@Security	BearerAuth
//	@Param		ws		path	string		true	"Workspace id"
//	@Param		body	body	muteRequest	true	"Muted or not"
//	@Success	204
//	@Router		/workspaces/{ws}/mute [put]
func (h *Handlers) muteWorkspace(c *gin.Context) {
	_, m, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var req muteRequest
	if !bindJSON(c, &req) {
		return
	}
	h.db.Model(&models.WorkspaceMember{}).Where("id = ?", m.ID).Update("muted", req.Muted)
	h.events.Publish(auth.UserID(c), liveEvent("workspaces"))
	c.Status(http.StatusNoContent)
}

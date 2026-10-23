package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/focus"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// The focus timer: one running session a person at a time (on a task or
// not); stopped early, the time so far counts; when its time is up the
// scheduler ends it and tells them (so a closed tab still hears).

type focusSessionView struct {
	models.FocusSession
	TaskTitle string `json:"task_title,omitempty"`
}

type focusState struct {
	Running *focusSessionView  `json:"running"`
	Today   []focusSessionView `json:"today"` // ended today (the caller's zone), newest first
	Minutes int                `json:"minutes"`
}

func (h *Handlers) focusViews(list []models.FocusSession) []focusSessionView {
	ids := []string{}
	for _, s := range list {
		if s.TaskID != "" {
			ids = append(ids, s.TaskID)
		}
	}
	titles := map[string]string{}
	if len(ids) > 0 {
		var tasks []models.Task
		h.db.Select("id", "title").Where("id IN ?", ids).Find(&tasks)
		for _, t := range tasks {
			titles[t.ID] = t.Title
		}
	}
	out := make([]focusSessionView, len(list))
	for i, s := range list {
		out[i] = focusSessionView{FocusSession: s, TaskTitle: titles[s.TaskID]}
	}
	return out
}

// focusNow is the caller's running session and today's.
func (h *Handlers) focusNow(c *gin.Context, wsID string) focusState {
	uid := auth.UserID(c)
	st := focusState{Today: []focusSessionView{}}
	var run models.FocusSession
	if h.db.Where("user_id = ? AND ended_at IS NULL", uid).First(&run).Error == nil {
		v := h.focusViews([]models.FocusSession{run})[0]
		st.Running = &v
	}
	var p models.UserPrefs
	h.db.Where("user_id = ?", uid).First(&p)
	loc := inbox.Zone(p)
	now := time.Now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	q := h.db.Where("user_id = ? AND ended_at IS NOT NULL AND ended_at >= ?", uid, midnight)
	if wsID != "" {
		q = q.Where("workspace_id = ?", wsID)
	}
	var done []models.FocusSession
	q.Order("ended_at DESC").Find(&done)
	st.Today = h.focusViews(done)
	for _, s := range done {
		st.Minutes += s.Spent
	}
	return st
}

// getFocus returns the caller's running session (any workspace) and what
// they focused on today (in ?ws=, or everywhere).
//
//	@Summary	My focus timer
//	@Tags		focus
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	query		string	false	"Only this workspace's sessions today"
//	@Success	200	{object}	focusState
//	@Router		/me/focus [get]
func (h *Handlers) getFocus(c *gin.Context) {
	c.JSON(http.StatusOK, h.focusNow(c, c.Query("ws")))
}

type focusStartRequest struct {
	WorkspaceID string `json:"workspace_id" binding:"required"`
	TaskID      string `json:"task_id"`
	Minutes     int    `json:"minutes" binding:"required,min=1,max=180"`
}

// startFocus starts a session (ending one still running).
//
//	@Summary	Start focusing
//	@Tags		focus
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		focusStartRequest	true	"Where, on what, how long"
//	@Success	201		{object}	focusState
//	@Router		/me/focus [post]
func (h *Handlers) startFocus(c *gin.Context) {
	var req focusStartRequest
	if !bindJSON(c, &req) {
		return
	}
	if _, _, ok := h.wsAccess(c, req.WorkspaceID, models.WSViewer); !ok {
		return
	}
	if req.TaskID != "" {
		var n int64
		h.db.Model(&models.Task{}).Where("id = ? AND workspace_id = ?", req.TaskID, req.WorkspaceID).Count(&n)
		if n == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that task isn't in this workspace"})
			return
		}
	}
	uid := auth.UserID(c)
	var running []models.FocusSession
	h.db.Where("user_id = ? AND ended_at IS NULL", uid).Find(&running)
	now := time.Now()
	for i := range running {
		focus.End(h.db, &running[i], now)
	}
	s := models.FocusSession{UserID: uid, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, Minutes: req.Minutes, StartedAt: now}
	if err := h.db.Create(&s).Error; err != nil {
		h.internalError(c, err, "could not start the timer")
		return
	}
	h.publishFocus(c, uid)
	c.JSON(http.StatusCreated, h.focusNow(c, req.WorkspaceID))
}

// stopFocus ends the running session now (its time so far counts).
//
//	@Summary	Stop focusing
//	@Tags		focus
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	focusState
//	@Router		/me/focus/stop [post]
func (h *Handlers) stopFocus(c *gin.Context) {
	uid := auth.UserID(c)
	var running []models.FocusSession
	h.db.Where("user_id = ? AND ended_at IS NULL", uid).Find(&running)
	ws := ""
	for i := range running {
		focus.End(h.db, &running[i], time.Now())
		ws = running[i].WorkspaceID
		h.publishWS(c, running[i].WorkspaceID, "tasks")
	}
	h.publishFocus(c, uid)
	c.JSON(http.StatusOK, h.focusNow(c, ws))
}

// publishFocus tells the person's other tabs and devices.
func (h *Handlers) publishFocus(c *gin.Context, uid string) {
	b, _ := json.Marshal(gin.H{"type": "focus", "origin": c.GetHeader("X-Orbit-Client")})
	h.events.Publish(uid, string(b))
}

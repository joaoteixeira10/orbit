package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Habits: each person's own, in a workspace with the Habits module. A habit
// lists the days it was done (the last 400, for streaks and the year grid).

const habitDays = 400

type habitResponse struct {
	models.Habit
	Checks []string `json:"checks"` // YYYY-MM-DD, oldest first
}

// habitAccess loads the caller's :id habit (anyone else's is not found).
func (h *Handlers) habitAccess(c *gin.Context) (models.Habit, bool) {
	var hb models.Habit
	if err := h.db.First(&hb, "id = ? AND owner_id = ?", c.Param("id"), auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "habit not found"})
		return hb, false
	}
	// Still a member (a write: not through a read-only token).
	need := models.WSEditor
	if c.Request.Method == http.MethodGet {
		need = models.WSViewer
	}
	if _, _, ok := h.wsAccess(c, hb.WorkspaceID, need); !ok {
		return hb, false
	}
	return hb, true
}

func (h *Handlers) habitResponses(list []models.Habit) []habitResponse {
	out := make([]habitResponse, len(list))
	ids := make([]string, len(list))
	at := map[string]int{}
	for i, hb := range list {
		out[i] = habitResponse{Habit: hb, Checks: []string{}}
		ids[i], at[hb.ID] = hb.ID, i
	}
	if len(ids) == 0 {
		return out
	}
	since := time.Now().AddDate(0, 0, -habitDays).Format(planner.DateLayout)
	var checks []models.HabitCheck
	h.db.Where("habit_id IN ? AND date >= ?", ids, since).Order("date").Find(&checks)
	for _, ch := range checks {
		out[at[ch.HabitID]].Checks = append(out[at[ch.HabitID]].Checks, ch.Date)
	}
	return out
}

// listHabits lists the caller's habits in a workspace.
//
//	@Summary	My habits in a workspace
//	@Tags		habits
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	habitResponse
//	@Router		/workspaces/{ws}/habits [get]
func (h *Handlers) listHabits(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.Habit
	h.db.Where("workspace_id = ? AND owner_id = ?", ws.ID, auth.UserID(c)).Order("archived_at IS NOT NULL, rank, created_at").Find(&list)
	c.JSON(http.StatusOK, h.habitResponses(list))
}

type habitRequest struct {
	Title    *string `json:"title" binding:"omitempty,max=200"`
	Icon     *string `json:"icon" binding:"omitempty,max=64"`
	Color    *string `json:"color"`
	Schedule *string `json:"schedule" binding:"omitempty,oneof=daily weekly"`
	Days     *string `json:"days"`   // seven 0/1, Monday first
	Target   *int    `json:"target"` // weekly: 1–7
	RemindAt *string `json:"remind_at"`
	Archived *bool   `json:"archived"`
}

func (req habitRequest) apply(hb *models.Habit) string {
	if req.Title != nil {
		hb.Title = strings.TrimSpace(*req.Title)
	}
	if hb.Title == "" {
		return "give the habit a name"
	}
	if req.Icon != nil {
		hb.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Color != nil {
		if *req.Color != "" && !planner.ValidColor(*req.Color) {
			return "color must be #rrggbb"
		}
		hb.Color = *req.Color
	}
	if req.Schedule != nil {
		hb.Schedule = *req.Schedule
	}
	if req.Days != nil {
		d := *req.Days
		if len(d) != 7 || strings.Trim(d, "01") != "" || d == "0000000" {
			return "days are seven 0 or 1 (Monday first), at least one 1"
		}
		hb.Days = d
	}
	if req.Target != nil {
		if *req.Target < 1 || *req.Target > 7 {
			return "a weekly habit is 1 to 7 times a week"
		}
		hb.Target = *req.Target
	}
	if req.RemindAt != nil {
		if *req.RemindAt != "" && !planner.ValidClock(*req.RemindAt) {
			return "remind_at must be HH:MM"
		}
		hb.RemindAt = *req.RemindAt
	}
	if req.Archived != nil {
		if *req.Archived && hb.ArchivedAt == nil {
			now := time.Now()
			hb.ArchivedAt = &now
		} else if !*req.Archived {
			hb.ArchivedAt = nil
		}
	}
	return ""
}

// createHabit adds one of the caller's habits to a workspace (members who
// may edit it).
//
//	@Summary	New habit
//	@Tags		habits
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		habitRequest	true	"The habit"
//	@Success	201		{object}	habitResponse
//	@Router		/workspaces/{ws}/habits [post]
func (h *Handlers) createHabit(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req habitRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	hb := models.Habit{WorkspaceID: ws.ID, OwnerID: uid, Schedule: "daily", Days: "1111111", Target: 1}
	if msg := req.apply(&hb); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.Habit{}).Select("MAX(rank) AS max").Where("workspace_id = ? AND owner_id = ?", ws.ID, uid).Scan(&last)
	hb.Rank = planner.Rank(last.Max, nil)
	if err := h.db.Create(&hb).Error; err != nil {
		h.internalError(c, err, "could not add the habit")
		return
	}
	c.JSON(http.StatusCreated, h.habitResponses([]models.Habit{hb})[0])
}

// updateHabit changes one of the caller's habits.
//
//	@Summary	Change a habit
//	@Tags		habits
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Habit id"
//	@Param		body	body		habitRequest	true	"What changes"
//	@Success	200		{object}	habitResponse
//	@Router		/habits/{id} [put]
func (h *Handlers) updateHabit(c *gin.Context) {
	hb, ok := h.habitAccess(c)
	if !ok {
		return
	}
	var req habitRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := req.apply(&hb); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Save(&hb).Error; err != nil {
		h.internalError(c, err, "could not save the habit")
		return
	}
	c.JSON(http.StatusOK, h.habitResponses([]models.Habit{hb})[0])
}

// deleteHabit deletes one of the caller's habits with its history.
//
//	@Summary	Delete a habit
//	@Tags		habits
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Habit id"
//	@Success	204
//	@Router		/habits/{id} [delete]
func (h *Handlers) deleteHabit(c *gin.Context) {
	hb, ok := h.habitAccess(c)
	if !ok {
		return
	}
	if err := deleteHabits(h.db, "id = ?", hb.ID); err != nil {
		h.internalError(c, err, "could not delete the habit")
		return
	}
	c.Status(http.StatusNoContent)
}

type habitCheckRequest struct {
	Done bool `json:"done"`
}

// checkHabit marks a day done (or not done).
//
//	@Summary	Mark a habit done on a day
//	@Tags		habits
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Habit id"
//	@Param		date	path		string				true	"YYYY-MM-DD"
//	@Param		body	body		habitCheckRequest	true	"Done or not"
//	@Success	200		{object}	habitResponse
//	@Router		/habits/{id}/checks/{date} [put]
func (h *Handlers) checkHabit(c *gin.Context) {
	hb, ok := h.habitAccess(c)
	if !ok {
		return
	}
	date := c.Param("date")
	if !planner.ValidDate(date) || date > time.Now().AddDate(0, 0, 1).Format(planner.DateLayout) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a day up to today, as YYYY-MM-DD"})
		return
	}
	var req habitCheckRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Done {
		var n int64
		h.db.Model(&models.HabitCheck{}).Where("habit_id = ? AND date = ?", hb.ID, date).Count(&n)
		if n == 0 {
			h.db.Create(&models.HabitCheck{HabitID: hb.ID, Date: date})
		}
	} else {
		h.db.Where("habit_id = ? AND date = ?", hb.ID, date).Delete(&models.HabitCheck{})
	}
	c.JSON(http.StatusOK, h.habitResponses([]models.Habit{hb})[0])
}

// deleteHabits removes habits (a query on models.Habit) with their checks.
func deleteHabits(tx *gorm.DB, where string, args ...any) error {
	var ids []string
	tx.Model(&models.Habit{}).Where(where, args...).Pluck("id", &ids)
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("habit_id IN ?", ids).Delete(&models.HabitCheck{}).Error; err != nil {
		return err
	}
	return tx.Where("id IN ?", ids).Delete(&models.Habit{}).Error
}

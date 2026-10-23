package api

import (
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// Goals: what a workspace works towards by a date, with progress from the
// tasks that count towards it (or by hand when none do).

type wsGoalResponse struct {
	models.Goal
	Tasks int `json:"tasks"` // its tasks (top-level)
	Done  int `json:"done"`  // of which done
}

func (h *Handlers) wsGoalResponses(list []models.Goal) []wsGoalResponse {
	out := make([]wsGoalResponse, len(list))
	at := map[string]int{}
	ids := make([]string, len(list))
	for i, g := range list {
		out[i] = wsGoalResponse{Goal: g}
		ids[i], at[g.ID] = g.ID, i
	}
	if len(ids) == 0 {
		return out
	}
	var rows []struct {
		GoalID string
		Total  int
		Done   int
	}
	h.db.Model(&models.Task{}).Select("goal_id, COUNT(*) AS total, SUM(CASE WHEN completed_at IS NOT NULL THEN 1 ELSE 0 END) AS done").
		Where("goal_id IN ? AND parent_id IS NULL", ids).Group("goal_id").Scan(&rows)
	for _, r := range rows {
		out[at[r.GoalID]].Tasks, out[at[r.GoalID]].Done = r.Total, r.Done
	}
	return out
}

func (h *Handlers) goalAccess(c *gin.Context, need string) (models.Goal, bool) {
	var g models.Goal
	if err := h.db.First(&g, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "goal not found"})
		return g, false
	}
	_, _, ok := h.wsAccess(c, g.WorkspaceID, need)
	return g, ok
}

// listGoals lists a workspace's goals (open ones first, by target date).
//
//	@Summary	A workspace's goals
//	@Tags		goals
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	wsGoalResponse
//	@Router		/workspaces/{ws}/goals [get]
func (h *Handlers) listGoals(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var list []models.Goal
	h.db.Where("workspace_id = ?", ws.ID).Order("achieved_at IS NOT NULL, due IS NULL, due, rank").Find(&list)
	c.JSON(http.StatusOK, h.wsGoalResponses(list))
}

type wsGoalRequest struct {
	Title    *string `json:"title" binding:"omitempty,max=200"`
	Notes    *string `json:"notes" binding:"omitempty,max=20000"`
	Icon     *string `json:"icon" binding:"omitempty,max=64"`
	Color    *string `json:"color"`
	Due      *string `json:"due"`      // YYYY-MM-DD, "" none
	Progress *int    `json:"progress"` // 0–100, by hand
	Achieved *bool   `json:"achieved"`
}

func (req wsGoalRequest) apply(g *models.Goal) string {
	if req.Title != nil {
		g.Title = strings.TrimSpace(*req.Title)
	}
	if g.Title == "" {
		return "give the goal a name"
	}
	if req.Notes != nil {
		g.Notes = *req.Notes
	}
	if req.Icon != nil {
		g.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Color != nil {
		if *req.Color != "" && !planner.ValidColor(*req.Color) {
			return "color must be #rrggbb"
		}
		g.Color = *req.Color
	}
	if req.Due != nil {
		g.Due = nil
		if *req.Due != "" {
			if !planner.ValidDate(*req.Due) {
				return "due must be YYYY-MM-DD"
			}
			d := *req.Due
			g.Due = &d
		}
	}
	if req.Progress != nil {
		if *req.Progress < 0 || *req.Progress > 100 {
			return "progress is 0 to 100"
		}
		g.Progress = *req.Progress
	}
	if req.Achieved != nil {
		if *req.Achieved && g.AchievedAt == nil {
			now := time.Now()
			g.AchievedAt = &now
		} else if !*req.Achieved {
			g.AchievedAt = nil
		}
	}
	return ""
}

// createGoal adds a goal (editors).
//
//	@Summary	New goal
//	@Tags		goals
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		wsGoalRequest	true	"The goal"
//	@Success	201		{object}	wsGoalResponse
//	@Router		/workspaces/{ws}/goals [post]
func (h *Handlers) createGoal(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req wsGoalRequest
	if !bindJSON(c, &req) {
		return
	}
	g := models.Goal{WorkspaceID: ws.ID, CreatedBy: auth.UserID(c)}
	if msg := req.apply(&g); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.Goal{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	g.Rank = planner.Rank(last.Max, nil)
	if err := h.db.Create(&g).Error; err != nil {
		h.internalError(c, err, "could not add the goal")
		return
	}
	h.publishWS(c, ws.ID, "goals")
	c.JSON(http.StatusCreated, h.wsGoalResponses([]models.Goal{g})[0])
}

// updateGoal changes a goal (editors).
//
//	@Summary	Change a goal
//	@Tags		goals
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"Goal id"
//	@Param		body	body		wsGoalRequest	true	"What changes"
//	@Success	200		{object}	wsGoalResponse
//	@Router		/goals/{id} [put]
func (h *Handlers) updateGoal(c *gin.Context) {
	g, ok := h.goalAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req wsGoalRequest
	if !bindJSON(c, &req) {
		return
	}
	if msg := req.apply(&g); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Save(&g).Error; err != nil {
		h.internalError(c, err, "could not save the goal")
		return
	}
	h.publishWS(c, g.WorkspaceID, "goals")
	c.JSON(http.StatusOK, h.wsGoalResponses([]models.Goal{g})[0])
}

// deleteGoal deletes a goal; its tasks stay (no longer towards it).
//
//	@Summary	Delete a goal
//	@Tags		goals
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Goal id"
//	@Success	204
//	@Router		/goals/{id} [delete]
func (h *Handlers) deleteGoal(c *gin.Context) {
	g, ok := h.goalAccess(c, models.WSEditor)
	if !ok {
		return
	}
	h.db.Model(&models.Task{}).Where("goal_id = ?", g.ID).UpdateColumn("goal_id", "")
	h.db.Delete(&g)
	h.publishWS(c, g.WorkspaceID, "goals")
	h.publishWS(c, g.WorkspaceID, "tasks")
	c.Status(http.StatusNoContent)
}

package api

import (
	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Task dependencies (finish-to-start): a task waits on others of its
// workspace; moving one's end moves what waits on it (planner.Cascade).

// waitGraph is each task of a workspace and what it waits on.
func (h *Handlers) waitGraph(wsID string) map[string][]string {
	var rows []struct {
		ID      string
		WaitsOn string
	}
	h.db.Model(&models.Task{}).Select("id", "waits_on").Where("workspace_id = ? AND waits_on <> ''", wsID).Scan(&rows)
	g := make(map[string][]string, len(rows))
	for _, r := range rows {
		g[r.ID] = models.Task{WaitsOn: r.WaitsOn}.WaitsOnList()
	}
	return g
}

// checkWaitsOn says what's wrong with t waiting on ids, if anything: they
// must be other tasks of its workspace, and none may (in the end) wait on t.
func (h *Handlers) checkWaitsOn(t *models.Task, ids []string) string {
	var want models.Task
	want.ID = t.ID
	want.SetWaitsOn(ids)
	list := want.WaitsOnList()
	if len(list) == 0 {
		return ""
	}
	var n int64
	h.db.Model(&models.Task{}).Where("workspace_id = ? AND id IN ?", t.WorkspaceID, list).Count(&n)
	if int(n) != len(list) {
		return "a task can only wait on tasks of its workspace"
	}
	if t.ID != "" && planner.Reaches(h.waitGraph(t.WorkspaceID), list, t.ID) {
		return "that would make a loop: one of those already waits on this task"
	}
	return ""
}

func endOf(t models.Task) string {
	if t.Due != nil {
		return *t.Due
	}
	if t.Start != nil {
		return *t.Start
	}
	return ""
}

// cascade moves what waits on t when its end moved (from before), and
// returns how many tasks moved. Linked issues of the moved get the change
// on the next sync.
func (h *Handlers) cascade(c *gin.Context, before, t models.Task) int {
	was, now := endOf(before), endOf(t)
	if was == "" || now == "" || was == now {
		return 0
	}
	var all []models.Task
	h.db.Select("id", "start", "due", "waits_on", "completed_at", "git_issue_id").Where("workspace_id = ?", t.WorkspaceID).Find(&all)
	dated := make([]planner.Dated, 0, len(all))
	linked := map[string]bool{}
	for _, x := range all {
		if x.ID == t.ID {
			x.Start, x.Due, x.CompletedAt = t.Start, t.Due, t.CompletedAt
		}
		dated = append(dated, planner.Dated{ID: x.ID, Start: x.Start, Due: x.Due, WaitsOn: x.WaitsOnList(), Done: x.CompletedAt != nil})
		linked[x.ID] = x.GitIssueID != 0
	}
	moved := planner.Cascade(dated, t.ID, planner.DaysBetween(was, now))
	if len(moved) == 0 {
		return 0
	}
	uid := auth.UserID(c)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, m := range moved {
			up := map[string]any{"start": m.Start, "due": m.Due}
			if linked[m.ID] {
				up["git_dirty"], up["git_dirty_by"] = true, uid
			}
			if err := tx.Model(&models.Task{}).Where("id = ?", m.ID).Updates(up).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.log.Warn("cascade: " + err.Error())
		return 0
	}
	return len(moved)
}

// forgetWaits takes deleted tasks out of what others wait on.
func (h *Handlers) forgetWaits(wsID string, gone []string) {
	if len(gone) == 0 {
		return
	}
	drop := map[string]bool{}
	for _, id := range gone {
		drop[id] = true
	}
	var tasks []models.Task
	h.db.Select("id", "waits_on").Where("workspace_id = ? AND waits_on <> ''", wsID).Find(&tasks)
	for _, x := range tasks {
		keep := []string{}
		for _, id := range x.WaitsOnList() {
			if !drop[id] {
				keep = append(keep, id)
			}
		}
		y := models.Task{}
		y.ID = x.ID
		y.SetWaitsOn(keep)
		if y.WaitsOn != x.WaitsOn {
			h.db.Model(&models.Task{}).Where("id = ?", x.ID).Update("waits_on", y.WaitsOn)
		}
	}
}

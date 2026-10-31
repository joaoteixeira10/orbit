package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// recentDoneDays is how long completed tasks stay in a list's normal view.
const recentDoneDays = 14

// ---- Lists ------------------------------------------------------------------

type listResponse struct {
	models.TaskList
	Columns   []planner.Column `json:"columns"`
	OpenCount int64            `json:"open_count"`
	DoneCount int64            `json:"done_count"`
	FolderID  string           `json:"folder_id,omitempty"` // the folder it's the project of
	Git       *gitLinkResponse `json:"git,omitempty"`
}

func (h *Handlers) toListResponse(c *gin.Context, l models.TaskList) listResponse {
	var open, done int64
	h.db.Model(&models.Task{}).Where("list_id = ? AND completed_at IS NULL AND parent_id IS NULL", l.ID).Count(&open)
	h.db.Model(&models.Task{}).Where("list_id = ? AND completed_at IS NOT NULL AND parent_id IS NULL", l.ID).Count(&done)
	var f models.Folder
	h.db.Select("id").Where("workspace_id = ? AND list_id = ?", l.WorkspaceID, l.ID).Limit(1).Find(&f)
	return listResponse{TaskList: l, Columns: planner.DecodeColumns(l.Columns), OpenCount: open, DoneCount: done, FolderID: f.ID, Git: h.gitLinkOf(l.ID, auth.UserID(c))}
}

func (h *Handlers) listAccess(c *gin.Context, need string) (models.TaskList, bool) {
	var l models.TaskList
	if err := h.db.First(&l, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "list not found"})
		return l, false
	}
	_, _, ok := h.wsAccess(c, l.WorkspaceID, need)
	return l, ok
}

// listLists returns a workspace's task lists with their columns.
//
//	@Summary	Task lists of a workspace
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	listResponse
//	@Router		/workspaces/{ws}/lists [get]
func (h *Handlers) listLists(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	var lists []models.TaskList
	h.db.Where("workspace_id = ?", ws.ID).Order("rank").Find(&lists)
	out := make([]listResponse, 0, len(lists))
	for _, l := range lists {
		out = append(out, h.toListResponse(c, l))
	}
	c.JSON(http.StatusOK, out)
}

type listRequest struct {
	Name    *string           `json:"name" binding:"omitempty,max=120"`
	Color   *string           `json:"color"`
	Icon    *string           `json:"icon" binding:"omitempty,max=64"`
	Columns *[]planner.Column `json:"columns"`
	// FolderID (making one only): its folder — none given: a new one of its
	// own (or the one at the top with its name); "none": no folder; a
	// folder's id: that one (taken from the list it had, if any).
	FolderID *string `json:"folder_id"`
}

// createList adds a task list (editors).
//
//	@Summary	Create a task list
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		listRequest	true	"List"
//	@Success	201		{object}	listResponse
//	@Router		/workspaces/{ws}/lists [post]
func (h *Handlers) createList(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req listRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the list a name"})
		return
	}
	l := models.TaskList{WorkspaceID: ws.ID, Name: strings.TrimSpace(*req.Name), Color: ws.Color}
	if req.Color != nil {
		if !planner.ValidColor(*req.Color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "color must be #rrggbb"})
			return
		}
		l.Color = *req.Color
	}
	if req.Icon != nil {
		l.Icon = *req.Icon
	}
	cols := planner.DefaultColumns()
	if req.Columns != nil {
		cols = *req.Columns
	}
	cols, err := planner.CleanColumns(cols)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	l.Columns = planner.EncodeColumns(cols)
	var last struct{ Max *float64 }
	h.db.Model(&models.TaskList{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	l.Rank = planner.Rank(last.Max, nil)
	var into *models.Folder
	if req.FolderID != nil && *req.FolderID != "" && *req.FolderID != "none" {
		into = &models.Folder{}
		if h.db.First(into, "id = ? AND workspace_id = ?", *req.FolderID, ws.ID).Error != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that folder isn't in this workspace"})
			return
		}
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&l).Error; err != nil {
			return err
		}
		switch {
		case into != nil:
			return tx.Model(&models.Folder{}).Where("id = ?", into.ID).Update("list_id", l.ID).Error
		case req.FolderID != nil && *req.FolderID == "none":
			return nil
		}
		return planner.ListFolder(tx, l, auth.UserID(c))
	}); err != nil {
		h.internalError(c, err, "could not create the list")
		return
	}
	h.publishWS(c, ws.ID, "lists")
	h.publishWS(c, ws.ID, "files")
	c.JSON(http.StatusCreated, h.toListResponse(c, l))
}

// updateList renames, recolours or changes a list's columns (editors). Tasks
// in a column that is removed move to the first open (or done) column.
//
//	@Summary	Update a task list
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"List id"
//	@Param		body	body		listRequest	true	"Fields to change"
//	@Success	200		{object}	listResponse
//	@Router		/lists/{id} [put]
func (h *Handlers) updateList(c *gin.Context) {
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	var req listRequest
	if !bindJSON(c, &req) {
		return
	}
	up := map[string]any{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the list a name"})
			return
		}
		up["name"] = n
	}
	if req.Color != nil {
		if !planner.ValidColor(*req.Color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "color must be #rrggbb"})
			return
		}
		up["color"] = *req.Color
	}
	if req.Icon != nil {
		up["icon"] = *req.Icon
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if req.Columns != nil {
			cols, err := planner.CleanColumns(*req.Columns)
			if err != nil {
				return badRequest(err.Error())
			}
			up["columns"] = planner.EncodeColumns(cols)
			// Re-home tasks whose column is gone, and keep completion in step
			// with a column that changed between open and done.
			var tasks []models.Task
			tx.Where("list_id = ?", l.ID).Find(&tasks)
			now := time.Now()
			for _, t := range tasks {
				col, found := planner.FindColumn(cols, t.ColumnID)
				if !found {
					if t.CompletedAt != nil {
						col = planner.FirstDone(cols)
					} else {
						col = planner.FirstOpen(cols)
					}
				}
				tu := map[string]any{"column_id": col.ID}
				if col.Done && t.CompletedAt == nil {
					tu["completed_at"] = &now
				} else if !col.Done && t.CompletedAt != nil {
					tu["completed_at"] = nil
				}
				if err := tx.Model(&models.Task{}).Where("id = ?", t.ID).Updates(tu).Error; err != nil {
					return err
				}
			}
		}
		if len(up) == 0 {
			return nil
		}
		before := l // Updates writes into l
		if err := tx.Model(&l).Updates(up).Error; err != nil {
			return err
		}
		var after models.TaskList
		tx.First(&after, "id = ?", l.ID)
		return planner.RenameListFolder(tx, before, after)
	})
	if writeTxError(c, h, err, "could not update the list") {
		return
	}
	l = reloadTaskList(h, l.ID)
	h.publishWS(c, l.WorkspaceID, "lists")
	if req.Name != nil || req.Color != nil {
		h.publishWS(c, l.WorkspaceID, "files")
	}
	c.JSON(http.StatusOK, h.toListResponse(c, l))
}

// deleteList deletes a list and its tasks (editors).
//
//	@Summary	Delete a task list
//	@Tags		tasks
//	@Security	BearerAuth
//	@Param		id	path	string	true	"List id"
//	@Success	204
//	@Router		/lists/{id} [delete]
func (h *Handlers) deleteList(c *gin.Context) {
	l, ok := h.listAccess(c, models.WSEditor)
	if !ok {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var taskIDs []string
		tx.Model(&models.Task{}).Where("list_id = ?", l.ID).Pluck("id", &taskIDs)
		if err := uploads.DeleteFor(tx, uploads.Task, taskIDs); err != nil {
			return err
		}
		if err := tx.Where("task_id IN ?", append(taskIDs, "")).Delete(&models.TaskComment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id IN ?", append(taskIDs, "")).Delete(&models.TaskEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("list_id = ?", l.ID).Delete(&models.Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("list_id = ?", l.ID).Delete(&models.GitLink{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Folder{}).Where("workspace_id = ? AND list_id = ?", l.WorkspaceID, l.ID).UpdateColumn("list_id", "").Error; err != nil {
			return err
		}
		return tx.Delete(&l).Error
	}); err != nil {
		h.internalError(c, err, "could not delete the list")
		return
	}
	h.publishWS(c, l.WorkspaceID, "lists")
	c.Status(http.StatusNoContent)
}

// orderLists sets the order of a workspace's lists (editors).
//
//	@Summary	Reorder task lists
//	@Tags		tasks
//	@Accept		json
//	@Security	BearerAuth
//	@Param		ws		path	string			true	"Workspace id"
//	@Param		body	body	orderRequest	true	"List ids in order"
//	@Success	204
//	@Router		/workspaces/{ws}/lists/order [put]
func (h *Handlers) orderLists(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req orderRequest
	if !bindJSON(c, &req) {
		return
	}
	for i, id := range req.IDs {
		h.db.Model(&models.TaskList{}).Where("id = ? AND workspace_id = ?", id, ws.ID).Update("rank", float64(i+1)*1024)
	}
	h.publishWS(c, ws.ID, "lists")
	c.Status(http.StatusNoContent)
}

// ---- Tasks ----------------------------------------------------------------------

type taskResponse struct {
	models.Task
	Tags        []string `json:"tags"`
	AssigneeIDs []string `json:"assignee_ids"`
	AssigneeID  *string  `json:"assignee_id"` // the first one (older clients)
	WaitsOn     []string `json:"waits_on"`    // tasks it starts after
	Done        bool     `json:"done"`
	Comments    int64    `json:"comment_count"`
	// GitError: the change is saved but didn't reach the linked issue (the
	// sync tries again).
	GitError string `json:"git_error,omitempty"`
}

func toTaskResponse(t models.Task) taskResponse {
	r := taskResponse{Task: t, Tags: decodeList(t.Tags), AssigneeIDs: t.AssigneeList(), WaitsOn: t.WaitsOnList(), Done: t.CompletedAt != nil}
	if len(r.AssigneeIDs) > 0 {
		first := r.AssigneeIDs[0]
		r.AssigneeID = &first
	}
	return r
}

// counted fills in how many comments each task has (one query).
func (h *Handlers) counted(rs []taskResponse) []taskResponse {
	if len(rs) == 0 {
		return rs
	}
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	var rows []struct {
		TaskID string
		N      int64
	}
	h.db.Model(&models.TaskComment{}).Select("task_id, COUNT(*) AS n").Where("task_id IN ?", ids).Group("task_id").Scan(&rows)
	n := map[string]int64{}
	for _, r := range rows {
		n[r.TaskID] = r.N
	}
	for i := range rs {
		rs[i].Comments = n[rs[i].ID]
	}
	return rs
}

func (h *Handlers) countedOne(r taskResponse) taskResponse { return h.counted([]taskResponse{r})[0] }

func toTaskResponses(ts []models.Task) []taskResponse {
	out := make([]taskResponse, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTaskResponse(t))
	}
	return out
}

// taskAccess loads the task in the path for a change (editors).
func (h *Handlers) taskAccess(c *gin.Context) (models.Task, bool) {
	var t models.Task
	if err := h.db.First(&t, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return t, false
	}
	_, _, ok := h.wsAccess(c, t.WorkspaceID, models.WSEditor)
	return t, ok
}

// listTasks returns a workspace's tasks: by default the open ones and those
// completed in the last two weeks (state=done: every completed one, newest
// first; state=all: everything), optionally of one list.
//
//	@Summary	Tasks of a workspace
//	@Tags		tasks
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path	string	true	"Workspace id"
//	@Param		list	query	string	false	"Only this list"
//	@Param		state	query	string	false	"active (default), done or all"
//	@Success	200		{array}	taskResponse
//	@Router		/workspaces/{ws}/tasks [get]
func (h *Handlers) listTasks(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	q := h.db.Where("workspace_id = ?", ws.ID)
	if l := c.Query("list"); l != "" {
		q = q.Where("list_id = ?", l)
	}
	switch c.Query("state") {
	case "done":
		q = q.Where("completed_at IS NOT NULL").Order("completed_at DESC").Limit(500)
	case "all":
		q = q.Order("rank")
	default:
		q = q.Where("completed_at IS NULL OR completed_at > ?", time.Now().AddDate(0, 0, -recentDoneDays)).Order("rank")
	}
	var tasks []models.Task
	q.Find(&tasks)
	c.JSON(http.StatusOK, h.counted(toTaskResponses(tasks)))
}

type taskRequest struct {
	ListID      *string   `json:"list_id"`
	ColumnID    *string   `json:"column_id"`
	ParentID    *string   `json:"parent_id"`
	Title       *string   `json:"title" binding:"omitempty,max=500"`
	Notes       *string   `json:"notes" binding:"omitempty,max=20000"`
	Priority    *int      `json:"priority" binding:"omitempty,min=0,max=3"`
	Estimate    *int      `json:"estimate" binding:"omitempty,min=0,max=10080"` // minutes (0 clears), a week at most
	Kind        *string   `json:"kind" binding:"omitempty,oneof=task exam"`     // "task" clears
	Tags        *[]string `json:"tags"`
	Start       *string   `json:"start"`                                   // YYYY-MM-DD, "" clears
	Due         *string   `json:"due"`                                     // YYYY-MM-DD, "" clears
	DueTime     *string   `json:"due_time"`                                // HH:MM, "" clears
	Repeat      *string   `json:"repeat"`                                  // RRULE, "" clears
	AssigneeIDs *[]string `json:"assignee_ids" binding:"omitempty,max=20"` // the whole list
	AssigneeID  *string   `json:"assignee_id"`                             // older clients: just one ("" = none)
	Completed   *bool     `json:"completed"`
	AfterID     string    `json:"after_id"` // create: place after this task ("" = at the end)
	// A linked list's task: its issue's milestone (a ref from
	// GET /lists/{id}/git/milestones; 0 = none) and its title.
	GitMilestoneID *int64  `json:"git_milestone_id"`
	GitMilestone   *string `json:"git_milestone" binding:"omitempty,max=255"`
	// GoalID: a goal of the task's workspace it counts towards ("" none).
	GoalID *string `json:"goal_id"`
	// WaitsOn: the tasks of its workspace it starts after (the whole list).
	WaitsOn *[]string `json:"waits_on" binding:"omitempty,max=20"`
}

// cleanTags trims, de-duplicates and bounds tags.
func cleanTags(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t == "" || len(t) > 40 || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
		if len(out) == 20 {
			break
		}
	}
	return out
}

// applyTaskFields validates and copies the plain fields of a request onto t.
func (h *Handlers) applyTaskFields(req taskRequest, t *models.Task) string {
	if req.Title != nil {
		t.Title = strings.TrimSpace(*req.Title)
		if t.Title == "" {
			return "give the task a title"
		}
	}
	if req.Notes != nil {
		t.Notes = *req.Notes
	}
	if req.GoalID != nil {
		t.GoalID = ""
		if *req.GoalID != "" {
			var n int64
			h.db.Model(&models.Goal{}).Where("id = ? AND workspace_id = ?", *req.GoalID, t.WorkspaceID).Count(&n)
			if n == 0 {
				return "that goal isn't in this workspace"
			}
			t.GoalID = *req.GoalID
		}
	}
	if req.GitMilestoneID != nil {
		t.GitMilestoneID = *req.GitMilestoneID
		t.GitMilestone = ""
		if t.GitMilestoneID != 0 && req.GitMilestone != nil {
			t.GitMilestone = strings.TrimSpace(*req.GitMilestone)
		}
	}
	if req.Kind != nil {
		t.Kind = ""
		if *req.Kind == "exam" {
			t.Kind = "exam"
		}
	}
	if req.Priority != nil {
		t.Priority = *req.Priority
	}
	if req.Estimate != nil {
		t.Estimate = *req.Estimate
	}
	if req.Tags != nil {
		t.Tags = encodeList(cleanTags(*req.Tags))
	}
	if req.Due != nil {
		if *req.Due == "" {
			t.Due, t.DueTime = nil, nil
		} else if !planner.ValidDate(*req.Due) {
			return "due must be a date (YYYY-MM-DD)"
		} else {
			d := *req.Due
			t.Due = &d
		}
	}
	if req.DueTime != nil {
		if *req.DueTime == "" {
			t.DueTime = nil
		} else if !planner.ValidClock(*req.DueTime) {
			return "due_time must be HH:MM"
		} else {
			tm := *req.DueTime
			t.DueTime = &tm
		}
	}
	if t.Due == nil {
		t.DueTime = nil
	}
	if req.Start != nil {
		if *req.Start == "" {
			t.Start = nil
		} else if !planner.ValidDate(*req.Start) {
			return "start must be a date (YYYY-MM-DD)"
		} else {
			d := *req.Start
			t.Start = &d
		}
	}
	// Keep the start on or before the due date: moving one past the other
	// takes the other along.
	if t.Start != nil && t.Due != nil && *t.Start > *t.Due {
		switch {
		case req.Start != nil && req.Due != nil:
			return "the start can't be after the due date"
		case req.Start != nil:
			d := *t.Start
			t.Due = &d
		default:
			d := *t.Due
			t.Start = &d
		}
	}
	if req.Repeat != nil {
		r, err := planner.CleanRRule(*req.Repeat)
		if err != nil {
			return err.Error()
		}
		t.Repeat = r
	}
	ids := req.AssigneeIDs
	if ids == nil && req.AssigneeID != nil {
		one := []string{}
		if *req.AssigneeID != "" {
			one = append(one, *req.AssigneeID)
		}
		ids = &one
	}
	if req.WaitsOn != nil {
		if msg := h.checkWaitsOn(t, *req.WaitsOn); msg != "" {
			return msg
		}
		t.SetWaitsOn(*req.WaitsOn)
	}
	if ids != nil {
		var want models.Task
		want.SetAssignees(*ids)
		if list := want.AssigneeList(); len(list) > 0 {
			var n int64
			h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ? AND user_id IN ?", t.WorkspaceID, list).Count(&n)
			if int(n) != len(list) {
				return "tasks can only be assigned to members of the workspace"
			}
		}
		t.Assignees = want.Assignees
	}
	return ""
}

// columnRank returns a rank at the end of a column, or right after afterID.
func (h *Handlers) columnRank(listID, columnID, afterID, exclude string) float64 {
	var sibs []struct {
		ID   string
		Rank float64
	}
	h.db.Model(&models.Task{}).Select("id", "rank").
		Where("list_id = ? AND column_id = ? AND parent_id IS NULL AND id <> ?", listID, columnID, exclude).
		Order("rank").Find(&sibs)
	if len(sibs) == 0 {
		return planner.Rank(nil, nil)
	}
	switch afterID {
	case "":
		return planner.Rank(&sibs[len(sibs)-1].Rank, nil)
	case "^":
		return planner.Rank(nil, &sibs[0].Rank)
	}
	for i := range sibs {
		if sibs[i].ID == afterID {
			if i+1 < len(sibs) {
				return planner.Rank(&sibs[i].Rank, &sibs[i+1].Rank)
			}
			return planner.Rank(&sibs[i].Rank, nil)
		}
	}
	return planner.Rank(&sibs[len(sibs)-1].Rank, nil)
}

// createTask adds a task (editors), at the end of its column (or after
// after_id). Without a list it goes to the workspace's first list.
//
//	@Summary	Create a task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string		true	"Workspace id"
//	@Param		body	body		taskRequest	true	"Task"
//	@Success	201		{object}	taskResponse
//	@Router		/workspaces/{ws}/tasks [post]
func (h *Handlers) createTask(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSEditor)
	if !ok {
		return
	}
	var req taskRequest
	if !bindJSON(c, &req) {
		return
	}
	var list models.TaskList
	q := h.db.Where("workspace_id = ?", ws.ID)
	if req.ListID != nil && *req.ListID != "" {
		q = q.Where("id = ?", *req.ListID)
	}
	if q.Order("rank").First(&list).Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "create a list first"})
		return
	}
	cols := planner.DecodeColumns(list.Columns)
	col := planner.FirstOpen(cols)
	if req.ColumnID != nil && *req.ColumnID != "" {
		var found bool
		if col, found = planner.FindColumn(cols, *req.ColumnID); !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that column isn't in the list"})
			return
		}
	}
	t := models.Task{WorkspaceID: ws.ID, ListID: list.ID, ColumnID: col.ID, CreatedBy: auth.UserID(c), Tags: "[]"}
	if req.Title == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the task a title"})
		return
	}
	if msg := h.applyTaskFields(req, &t); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if req.ParentID != nil && *req.ParentID != "" {
		var parent models.Task
		if h.db.First(&parent, "id = ? AND workspace_id = ?", *req.ParentID, ws.ID).Error != nil || parent.ParentID != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sub-tasks go one level deep, under a task of this workspace"})
			return
		}
		t.ParentID, t.ListID, t.ColumnID = &parent.ID, parent.ListID, parent.ColumnID
		var last struct{ Max *float64 }
		h.db.Model(&models.Task{}).Select("MAX(rank) AS max").Where("parent_id = ?", parent.ID).Scan(&last)
		t.Rank = planner.Rank(last.Max, nil)
	} else {
		t.Rank = h.columnRank(list.ID, col.ID, req.AfterID, "")
	}
	if col.Done || (req.Completed != nil && *req.Completed) {
		now := time.Now()
		t.CompletedAt = &now
		if !col.Done {
			t.ColumnID = planner.FirstDone(cols).ID
		}
	}
	if !h.gitGuard(c, nil, &t) {
		return
	}
	if err := h.db.Create(&t).Error; err != nil {
		h.internalError(c, err, "could not create the task")
		return
	}
	h.publishWS(c, ws.ID, "tasks")
	h.notifyAssigned(c, t, nil)
	h.recordTask(c, nil, t)
	if t.ParentID == nil {
		h.notifyActivity(c, ws, "task_added", t.Title, taskLink(t), t.AssigneeList()...)
	}
	resp := toTaskResponse(t)
	if resp.GitError = h.gitAfter(c, nil, &t); resp.GitError == "" && t.GitIssueID != 0 {
		resp = toTaskResponse(t)
		h.publishWS(c, ws.ID, "tasks")
	}
	c.JSON(http.StatusCreated, h.countedOne(resp))
}

// updateTask changes a task (editors). Completing a repeating task with a due
// date moves it to its next date instead; moving it to another list puts it
// in that list's first open (or done) column.
//
//	@Summary	Update a task
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string		true	"Task id"
//	@Param		body	body		taskRequest	true	"Fields to change"
//	@Success	200		{object}	taskResponse
//	@Router		/tasks/{id} [put]
func (h *Handlers) updateTask(c *gin.Context) {
	t, ok := h.taskAccess(c)
	if !ok {
		return
	}
	var req taskRequest
	if !bindJSON(c, &req) {
		return
	}
	wasDone, prevAssignees, before := t.CompletedAt != nil, t.AssigneeList(), t
	if msg := h.applyTaskFields(req, &t); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var list models.TaskList
	h.db.First(&list, "id = ?", t.ListID)
	moved := false
	if req.ListID != nil && *req.ListID != t.ListID && t.ParentID == nil {
		if h.db.First(&list, "id = ? AND workspace_id = ?", *req.ListID, t.WorkspaceID).Error != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that list isn't in this workspace"})
			return
		}
		t.ListID, moved = list.ID, true
	}
	cols := planner.DecodeColumns(list.Columns)
	col, found := planner.FindColumn(cols, t.ColumnID)
	if req.ColumnID != nil && *req.ColumnID != "" {
		if col, found = planner.FindColumn(cols, *req.ColumnID); !found {
			c.JSON(http.StatusBadRequest, gin.H{"error": "that column isn't in the list"})
			return
		}
	} else if !found || moved {
		if t.CompletedAt != nil {
			col = planner.FirstDone(cols)
		} else {
			col = planner.FirstOpen(cols)
		}
	}
	done := col.Done
	if req.Completed != nil {
		done = *req.Completed
	}
	repeated := false
	switch {
	case done && t.CompletedAt == nil && t.Repeat != "" && t.Due != nil:
		// A repeating task moves on to its next date and stays open.
		if next, ok := planner.NextDue(*t.Due, t.Repeat); ok {
			// The start keeps its distance from the due date.
			if t.Start != nil {
				if st, ok := planner.ShiftDate(*t.Start, planner.DaysBetween(*t.Due, next)); ok {
					t.Start = &st
				}
			}
			t.Due, repeated = &next, true
			col = planner.FirstOpen(cols)
		} else {
			now := time.Now()
			t.CompletedAt, t.Repeat = &now, ""
			col = planner.FirstDone(cols)
		}
	case done && t.CompletedAt == nil:
		now := time.Now()
		t.CompletedAt = &now
		if !col.Done {
			col = planner.FirstDone(cols)
		}
	case !done && t.CompletedAt != nil:
		t.CompletedAt = nil
		if col.Done {
			col = planner.FirstOpen(cols)
		}
	}
	if col.ID != t.ColumnID || moved {
		t.ColumnID = col.ID
		if t.ParentID == nil {
			t.Rank = h.columnRank(t.ListID, col.ID, "", t.ID)
		}
	}
	if !h.gitGuard(c, &before, &t) {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&t).Error; err != nil {
			return err
		}
		// Sub-tasks follow their parent to another list.
		if moved {
			return tx.Model(&models.Task{}).Where("parent_id = ?", t.ID).Updates(map[string]any{"list_id": t.ListID, "column_id": t.ColumnID}).Error
		}
		return nil
	}); err != nil {
		h.internalError(c, err, "could not update the task")
		return
	}
	shifted := h.cascade(c, before, t)
	h.publishWS(c, t.WorkspaceID, "tasks")
	h.recordTask(c, &before, t)
	h.notifyAssigned(c, t, prevAssignees)
	if !wasDone && (t.CompletedAt != nil || repeated) && t.ParentID == nil {
		h.notifyActivity(c, h.workspaceOf(t.WorkspaceID), "task_done", t.Title, taskLink(t))
	}
	gitErr := h.gitAfter(c, &before, &t)
	resp := toTaskResponse(t)
	resp.GitError = gitErr
	c.JSON(http.StatusOK, gin.H{"task": h.countedOne(resp), "repeated": repeated, "shifted": shifted})
}

type moveTaskRequest struct {
	ListID   string `json:"list_id"`
	ColumnID string `json:"column_id" binding:"required"`
	AfterID  string `json:"after_id"` // "^" = first, "" = last
}

// moveTask drops a task in a column of a board, at a position (editors).
// Dropping it in a done column completes it; out of one reopens it.
//
//	@Summary	Move a task on a board
//	@Tags		tasks
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Task id"
//	@Param		body	body		moveTaskRequest	true	"Where"
//	@Success	200		{object}	taskResponse
//	@Router		/tasks/{id}/move [post]
func (h *Handlers) moveTask(c *gin.Context) {
	t, ok := h.taskAccess(c)
	if !ok {
		return
	}
	var req moveTaskRequest
	if !bindJSON(c, &req) {
		return
	}
	if t.ParentID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sub-tasks move with their task"})
		return
	}
	listID := t.ListID
	if req.ListID != "" {
		listID = req.ListID
	}
	var list models.TaskList
	if h.db.First(&list, "id = ? AND workspace_id = ?", listID, t.WorkspaceID).Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that list isn't in this workspace"})
		return
	}
	col, found := planner.FindColumn(planner.DecodeColumns(list.Columns), req.ColumnID)
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that column isn't in the list"})
		return
	}
	up := map[string]any{"list_id": list.ID, "column_id": col.ID, "rank": h.columnRank(list.ID, col.ID, req.AfterID, t.ID)}
	completing := col.Done && t.CompletedAt == nil
	after := t
	after.ListID, after.ColumnID = list.ID, col.ID
	if completing {
		now := time.Now()
		up["completed_at"] = &now
		after.CompletedAt = &now
	} else if !col.Done && t.CompletedAt != nil {
		up["completed_at"] = nil
		after.CompletedAt = nil
	}
	if !h.gitGuard(c, &t, &after) {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Task{}).Where("id = ?", t.ID).Updates(up).Error; err != nil {
			return err
		}
		return tx.Model(&models.Task{}).Where("parent_id = ?", t.ID).Updates(map[string]any{"list_id": list.ID, "column_id": col.ID}).Error
	}); err != nil {
		h.internalError(c, err, "could not move the task")
		return
	}
	before := t
	t = reloadTask(h, t.ID)
	h.recordTask(c, &before, t)
	h.publishWS(c, t.WorkspaceID, "tasks")
	if completing && t.ParentID == nil {
		h.notifyActivity(c, h.workspaceOf(t.WorkspaceID), "task_done", t.Title, taskLink(t))
	}
	gitErr := h.gitAfter(c, &before, &t)
	resp := toTaskResponse(t)
	resp.GitError = gitErr
	c.JSON(http.StatusOK, h.countedOne(resp))
}

// deleteTask deletes a task and its sub-tasks (editors).
//
//	@Summary	Delete a task
//	@Tags		tasks
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Task id"
//	@Success	204
//	@Router		/tasks/{id} [delete]
func (h *Handlers) deleteTask(c *gin.Context) {
	t, ok := h.taskAccess(c)
	if !ok {
		return
	}
	var ids []string
	h.db.Model(&models.Task{}).Where("id = ? OR parent_id = ?", t.ID, t.ID).Pluck("id", &ids)
	_ = uploads.DeleteFor(h.db, uploads.Task, ids) // its attachments go with it
	h.db.Where("task_id IN ?", append(ids, "")).Delete(&models.TaskComment{})
	h.db.Where("task_id IN ?", append(ids, "")).Delete(&models.TaskEvent{})
	h.db.Where("task_id IN ?", append(ids, "")).Delete(&models.TaskBlock{})
	h.db.Where("id = ? OR parent_id = ?", t.ID, t.ID).Delete(&models.Task{})
	h.forgetWaits(t.WorkspaceID, ids)
	if h.git != nil {
		// Its issue stays on git, and mustn't come back as a new task.
		h.git.Ignore(t.ListID, t.GitIssueID)
	}
	h.publishWS(c, t.WorkspaceID, "tasks")
	c.Status(http.StatusNoContent)
}

// ---- helpers ----------------------------------------------------------------------

// badRequest is an error a transaction returns to answer 400 with its text.
type badRequest string

func (b badRequest) Error() string { return string(b) }

// writeTxError answers a transaction's error (400 for a badRequest, 500
// otherwise) and reports whether there was one.
func writeTxError(c *gin.Context, h *Handlers, err error, msg string) bool {
	if err == nil {
		return false
	}
	if br, ok := err.(badRequest); ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": string(br)})
		return true
	}
	h.internalError(c, err, msg)
	return true
}

// taskLink opens a task in the app.
func taskLink(t models.Task) string {
	return fmt.Sprintf("/w/%s/tasks/%s?task=%s", t.WorkspaceID, t.ListID, t.ID)
}

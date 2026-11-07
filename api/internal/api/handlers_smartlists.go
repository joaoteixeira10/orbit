package api

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
)

// Smart lists: saved task filters, shown with Today and Scheduled in Tasks.
// Anyone in a workspace keeps their own; editors can share one with every
// member. Its owner changes it — and, for a shared one, so can the
// workspace's owners.

// SmartFilter is what a smart list shows (the web's TaskFilter): tasks that
// match every part that's set.
type SmartFilter struct {
	Q        string   `json:"q"`
	Due      []string `json:"due"`      // overdue, today, week, next, later, none
	Priority []int    `json:"priority"` // 0 none … 3 high
	Progress []string `json:"progress"` // notStarted, inProgress, done
	Assignee []string `json:"assignee"` // user ids, "me", "none"
	Tags     []string `json:"tags"`
	Lists    []string `json:"lists"`
}

var (
	dueBuckets = []string{"overdue", "today", "week", "next", "later", "none"}
	progresses = []string{"notStarted", "inProgress", "done"}
)

// clean checks a filter and fills its empty parts (so it reads back as []).
func (f *SmartFilter) clean() string {
	f.Q = strings.TrimSpace(f.Q)
	if len(f.Q) > 200 {
		return "the keyword is too long"
	}
	for _, arr := range [][]string{f.Due, f.Progress, f.Assignee, f.Tags, f.Lists} {
		if len(arr) > 100 {
			return "too many choices in the filter"
		}
		for _, v := range arr {
			if v == "" || len(v) > 100 {
				return "a filter value is empty or too long"
			}
		}
	}
	for _, d := range f.Due {
		if !slices.Contains(dueBuckets, d) {
			return "unknown due bucket " + d
		}
	}
	for _, p := range f.Progress {
		if !slices.Contains(progresses, p) {
			return "unknown progress " + p
		}
	}
	if len(f.Priority) > 4 {
		return "too many priorities"
	}
	for _, p := range f.Priority {
		if p < 0 || p > 3 {
			return "priority is 0 to 3"
		}
	}
	for _, p := range []*[]string{&f.Due, &f.Progress, &f.Assignee, &f.Tags, &f.Lists} {
		if *p == nil {
			*p = []string{}
		}
	}
	if f.Priority == nil {
		f.Priority = []int{}
	}
	return ""
}

type smartListResponse struct {
	models.SmartList
	Filter  SmartFilter `json:"filter"`
	CanEdit bool        `json:"can_edit"`
}

func smartListOut(l models.SmartList, uid string, wsRole string) smartListResponse {
	var f SmartFilter
	_ = json.Unmarshal([]byte(l.Filter), &f)
	_ = f.clean()
	return smartListResponse{SmartList: l, Filter: f, CanEdit: l.OwnerID == uid || (l.Shared && wsRole == models.WSOwner)}
}

// smartListAccess finds a smart list its caller may see (and, with edit,
// change); anything else is 404, as if it weren't there.
func (h *Handlers) smartListAccess(c *gin.Context, edit bool) (models.SmartList, string, bool) {
	var l models.SmartList
	if err := h.db.First(&l, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "smart list not found"})
		return l, "", false
	}
	_, m, ok := h.wsAccess(c, l.WorkspaceID, models.WSViewer)
	if !ok {
		return l, "", false
	}
	role := m.Role
	uid := auth.UserID(c)
	if l.OwnerID != uid && !l.Shared {
		c.JSON(http.StatusNotFound, gin.H{"error": "smart list not found"})
		return l, "", false
	}
	if edit && !smartListOut(l, uid, role).CanEdit {
		c.JSON(http.StatusForbidden, gin.H{"error": "only whoever made this smart list (or the workspace's owners) can change it"})
		return l, "", false
	}
	return l, role, true
}

// listSmartLists lists the caller's smart lists and the shared ones.
//
//	@Summary	A workspace's smart lists (yours and the shared ones)
//	@Tags		smart lists
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	smartListResponse
//	@Router		/workspaces/{ws}/smart-lists [get]
func (h *Handlers) listSmartLists(c *gin.Context) {
	ws, m, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	role := m.Role
	uid := auth.UserID(c)
	var list []models.SmartList
	h.db.Where("workspace_id = ? AND (owner_id = ? OR shared = ?)", ws.ID, uid, true).Order("rank, created_at").Find(&list)
	out := make([]smartListResponse, len(list))
	for i, l := range list {
		out[i] = smartListOut(l, uid, role)
	}
	c.JSON(http.StatusOK, out)
}

type smartListRequest struct {
	Name   *string      `json:"name" binding:"omitempty,max=100"`
	Color  *string      `json:"color"`
	Icon   *string      `json:"icon" binding:"omitempty,max=64"`
	Shared *bool        `json:"shared"`
	Filter *SmartFilter `json:"filter"`
}

func (req smartListRequest) apply(l *models.SmartList, role string) string {
	if req.Name != nil {
		l.Name = strings.TrimSpace(*req.Name)
	}
	if l.Name == "" {
		return "give the smart list a name"
	}
	if req.Color != nil {
		if *req.Color != "" && !planner.ValidColor(*req.Color) {
			return "color must be #rrggbb"
		}
		l.Color = *req.Color
	}
	if req.Icon != nil {
		l.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Shared != nil && *req.Shared != l.Shared {
		if role == models.WSViewer {
			return "viewers keep their smart lists to themselves"
		}
		l.Shared = *req.Shared
	}
	if req.Filter != nil {
		f := *req.Filter
		if msg := f.clean(); msg != "" {
			return msg
		}
		b, _ := json.Marshal(f)
		l.Filter = string(b)
	}
	if l.Filter == "" {
		b, _ := json.Marshal(SmartFilter{})
		l.Filter = string(b)
	}
	return ""
}

// createSmartList saves a filter as a smart list (anyone in the workspace;
// sharing it takes an editor).
//
//	@Summary	New smart list
//	@Tags		smart lists
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		smartListRequest	true	"The smart list"
//	@Success	201		{object}	smartListResponse
//	@Router		/workspaces/{ws}/smart-lists [post]
func (h *Handlers) createSmartList(c *gin.Context) {
	ws, m, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	role := m.Role
	var req smartListRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	var n int64
	h.db.Model(&models.SmartList{}).Where("workspace_id = ? AND owner_id = ?", ws.ID, uid).Count(&n)
	if n >= 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "100 smart lists is the most"})
		return
	}
	l := models.SmartList{WorkspaceID: ws.ID, OwnerID: uid}
	if msg := req.apply(&l, role); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	var last struct{ Max *float64 }
	h.db.Model(&models.SmartList{}).Select("MAX(rank) AS max").Where("workspace_id = ?", ws.ID).Scan(&last)
	l.Rank = planner.Rank(last.Max, nil)
	if err := h.db.Create(&l).Error; err != nil {
		h.internalError(c, err, "could not save the smart list")
		return
	}
	if l.Shared {
		h.publishWS(c, ws.ID, "smart-lists")
	}
	c.JSON(http.StatusCreated, smartListOut(l, uid, role))
}

// updateSmartList changes a smart list: its name, look, filter or sharing.
//
//	@Summary	Change a smart list
//	@Tags		smart lists
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Smart list id"
//	@Param		body	body		smartListRequest	true	"What changes"
//	@Success	200		{object}	smartListResponse
//	@Router		/smart-lists/{id} [put]
func (h *Handlers) updateSmartList(c *gin.Context) {
	l, role, ok := h.smartListAccess(c, true)
	if !ok {
		return
	}
	var req smartListRequest
	if !bindJSON(c, &req) {
		return
	}
	wasShared := l.Shared
	if msg := req.apply(&l, role); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	if err := h.db.Save(&l).Error; err != nil {
		h.internalError(c, err, "could not save the smart list")
		return
	}
	if l.Shared || wasShared {
		h.publishWS(c, l.WorkspaceID, "smart-lists")
	}
	c.JSON(http.StatusOK, smartListOut(l, auth.UserID(c), role))
}

// deleteSmartList deletes a smart list (its tasks are untouched).
//
//	@Summary	Delete a smart list
//	@Tags		smart lists
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Smart list id"
//	@Success	204
//	@Router		/smart-lists/{id} [delete]
func (h *Handlers) deleteSmartList(c *gin.Context) {
	l, _, ok := h.smartListAccess(c, true)
	if !ok {
		return
	}
	h.db.Delete(&l)
	if l.Shared {
		h.publishWS(c, l.WorkspaceID, "smart-lists")
	}
	c.Status(http.StatusNoContent)
}

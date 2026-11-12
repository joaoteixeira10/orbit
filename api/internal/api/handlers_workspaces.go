package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/holidays"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/uploads"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ---- Access -----------------------------------------------------------------

// roleRank orders workspace roles; a role satisfies every need at or below it.
func roleRank(role string) int {
	switch role {
	case models.WSOwner:
		return 3
	case models.WSEditor:
		return 2
	case models.WSViewer:
		return 1
	}
	return 0
}

// wsAccess loads a workspace the caller is a member of and checks their role
// covers need (viewer, editor or owner). Someone who isn't a member gets 404,
// so workspace ids don't reveal anything. Writes are refused through a
// read-only API token and, except for the owner's settings, in an archived
// workspace. It writes the error response; ok false means stop.
func (h *Handlers) wsAccess(c *gin.Context, wsID, need string) (ws models.Workspace, member models.WorkspaceMember, ok bool) {
	if err := h.db.Where("workspace_id = ? AND user_id = ?", wsID, auth.UserID(c)).First(&member).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return ws, member, false
	}
	if err := h.db.First(&ws, "id = ?", wsID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return ws, member, false
	}
	if ws.AssistantOff && viaOf(c) == "assistant" {
		c.JSON(http.StatusForbidden, gin.H{"error": "the assistant is turned off in this workspace"})
		return ws, member, false
	}
	if need != models.WSViewer && auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return ws, member, false
	}
	if roleRank(member.Role) < roleRank(need) {
		msg := "you need to be an editor of this workspace"
		if need == models.WSOwner {
			msg = "only the workspace's owners can do this"
		}
		c.JSON(http.StatusForbidden, gin.H{"error": msg})
		return ws, member, false
	}
	if need == models.WSEditor && ws.ArchivedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the workspace is archived; restore it to make changes"})
		return ws, member, false
	}
	return ws, member, true
}

// memberNames maps a workspace's members to their names (display name, else
// username).
func (h *Handlers) memberNames(wsID string) map[string]string {
	var rows []struct {
		ID, Username, DisplayName string
	}
	h.db.Table("workspace_members").Joins("JOIN users ON users.id = workspace_members.user_id").
		Where("workspace_members.workspace_id = ?", wsID).Select("users.id, users.username, users.display_name").Scan(&rows)
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.DisplayName
		if out[r.ID] == "" {
			out[r.ID] = r.Username
		}
	}
	return out
}

// memberIDs lists the users of a workspace.
func (h *Handlers) memberIDs(wsID string) []string {
	var ids []string
	h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", wsID).Pluck("user_id", &ids)
	return ids
}

// myWorkspaceIDs lists the workspaces the caller belongs to (archived ones
// included unless active is set; for the assistant, not those it's kept out
// of).
func (h *Handlers) myWorkspaceIDs(c *gin.Context, active bool) []string {
	q := h.db.Model(&models.WorkspaceMember{}).Where("workspace_members.user_id = ?", auth.UserID(c)).
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id")
	if active {
		q = q.Where("workspaces.archived_at IS NULL")
	}
	if viaOf(c) == "assistant" {
		q = q.Where("workspaces.assistant_off = ?", false)
	}
	var ids []string
	q.Pluck("workspace_members.workspace_id", &ids)
	return ids
}

// publishWS tells every member's open sessions that part of a workspace
// changed ("pages", "tasks", "lists", "events", "subscriptions", "members",
// "workspace"), so their views re-fetch. origin is the X-Orbit-Client header
// of the request that made the change: that tab already has it and ignores it.
func (h *Handlers) publishWS(c *gin.Context, wsID, what string, extraUsers ...string) {
	b, _ := json.Marshal(gin.H{"type": "ws", "ws": wsID, "what": what, "origin": c.GetHeader("X-Orbit-Client")})
	msg := string(b)
	seen := map[string]bool{}
	for _, id := range append(h.memberIDs(wsID), extraUsers...) {
		if !seen[id] {
			seen[id] = true
			h.events.Publish(id, msg)
		}
	}
}

// ---- DTOs -------------------------------------------------------------------

type workspaceResponse struct {
	models.Workspace
	Modules     []string `json:"modules"`
	Role        string   `json:"role"`
	Position    float64  `json:"position"`
	MemberCount int64    `json:"member_count"`
	Muted       bool     `json:"muted"` // the caller muted its activity
}

type memberResponse struct {
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Role        string    `json:"role"`
	DirectRole  string    `json:"direct_role"`      // their own ("" in only through groups)
	Groups      []string  `json:"groups,omitempty"` // the groups that bring them in
	JoinedAt    time.Time `json:"joined_at"`
}

func decodeList(s string) []string {
	out := []string{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &out)
	}
	return out
}

func encodeList(v []string) string {
	if v == nil {
		v = []string{}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *Handlers) toWorkspaceResponse(ws models.Workspace, m models.WorkspaceMember) workspaceResponse {
	var n int64
	h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ?", ws.ID).Count(&n)
	return workspaceResponse{Workspace: ws, Modules: decodeList(ws.Modules), Role: m.Role, Position: m.Position, MemberCount: n, Muted: m.Muted}
}

func (h *Handlers) membersOf(wsID string) []memberResponse {
	type row struct {
		models.WorkspaceMember
		Username    string
		DisplayName string
		AvatarURL   string
	}
	var rows []row
	h.db.Table("workspace_members").
		Select("workspace_members.*, users.username, users.display_name, users.avatar_url").
		Joins("JOIN users ON users.id = workspace_members.user_id").
		Where("workspace_members.workspace_id = ?", wsID).
		Order("workspace_members.created_at").Scan(&rows)
	var via []struct{ UserID, Name string }
	h.db.Table("workspace_groups").Select("group_members.user_id, groups.name").
		Joins("JOIN groups ON groups.id = workspace_groups.group_id").
		Joins("JOIN group_members ON group_members.group_id = workspace_groups.group_id").
		Where("workspace_groups.workspace_id = ?", wsID).Order("groups.name").Scan(&via)
	groups := map[string][]string{}
	for _, v := range via {
		groups[v.UserID] = append(groups[v.UserID], v.Name)
	}
	out := make([]memberResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, memberResponse{UserID: r.UserID, Username: r.Username, DisplayName: r.DisplayName, AvatarURL: r.AvatarURL, Role: r.Role, DirectRole: r.DirectRole, Groups: groups[r.UserID], JoinedAt: r.CreatedAt})
	}
	return out
}

// ---- Catalog ----------------------------------------------------------------

// planCatalog returns the module catalog and the workspace templates, for the
// "new workspace" sheet.
//
//	@Summary	Modules and workspace templates
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]any
//	@Router		/catalog [get]
func (h *Handlers) planCatalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"modules": planner.Catalog, "templates": planner.Templates})
}

// ---- Workspaces ---------------------------------------------------------------

// listWorkspaces returns the caller's workspaces in their sidebar order.
//
//	@Summary	My workspaces
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	workspaceResponse
//	@Router		/workspaces [get]
func (h *Handlers) listWorkspaces(c *gin.Context) {
	var members []models.WorkspaceMember
	h.db.Where("user_id = ?", auth.UserID(c)).Order("position, created_at").Find(&members)
	out := make([]workspaceResponse, 0, len(members))
	for _, m := range members {
		var ws models.Workspace
		if h.db.First(&ws, "id = ?", m.WorkspaceID).Error == nil && (!ws.AssistantOff || viaOf(c) != "assistant") {
			out = append(out, h.toWorkspaceResponse(ws, m))
		}
	}
	c.JSON(http.StatusOK, out)
}

type createWorkspaceRequest struct {
	Name        string   `json:"name" binding:"required,max=120"`
	Icon        string   `json:"icon" binding:"max=64"`
	Color       string   `json:"color"`
	Kind        string   `json:"kind" binding:"omitempty,oneof=school work personal shared custom"`
	Description string   `json:"description" binding:"max=500"`
	Modules     []string `json:"modules"` // nil = the template's
	// Starter creates the template's lists and first pages (default true).
	Starter *bool  `json:"starter"`
	Locale  string `json:"locale"` // names the starter content (en, pt)
}

// createWorkspace creates a workspace from a template, with the caller as its
// owner and, unless starter is false, the template's lists and pages.
//
//	@Summary	Create a workspace
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createWorkspaceRequest	true	"Workspace"
//	@Success	201		{object}	workspaceResponse
//	@Router		/workspaces [post]
func (h *Handlers) createWorkspace(c *gin.Context) {
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return
	}
	var req createWorkspaceRequest
	if !bindJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the workspace a name"})
		return
	}
	tpl := planner.TemplateFor(req.Kind)
	if req.Color == "" {
		req.Color = tpl.Color
	}
	if !planner.ValidColor(req.Color) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "color must be #rrggbb"})
		return
	}
	if req.Icon == "" {
		req.Icon = tpl.Icon
	}
	modules := tpl.Modules
	if req.Modules != nil {
		var ok bool
		if modules, ok = planner.CleanModules(req.Modules); !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown or unavailable module"})
			return
		}
	}
	uid := auth.UserID(c)
	ws := models.Workspace{Name: req.Name, Icon: req.Icon, Color: req.Color, Kind: tpl.Kind, Description: req.Description, Modules: encodeList(modules), CreatedBy: uid}
	var member models.WorkspaceMember
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ws).Error; err != nil {
			return err
		}
		// New workspaces go to the bottom of the creator's sidebar.
		var last struct{ Max *float64 }
		tx.Model(&models.WorkspaceMember{}).Select("MAX(position) AS max").Where("user_id = ?", uid).Scan(&last)
		member = models.WorkspaceMember{WorkspaceID: ws.ID, UserID: uid, Role: models.WSOwner, DirectRole: models.WSOwner, Position: planner.Rank(last.Max, nil)}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		if req.Starter == nil || *req.Starter {
			return seedWorkspace(tx, ws, tpl, uid, req.Locale)
		}
		return nil
	})
	if err != nil {
		h.internalError(c, err, "could not create the workspace")
		return
	}
	h.recordCtx(c, "workspace.create", "workspace", ws.ID, gin.H{"name": ws.Name, "kind": ws.Kind})
	h.events.Publish(uid, liveEvent("workspaces"))
	c.JSON(http.StatusCreated, h.toWorkspaceResponse(ws, member))
}

// seedWorkspace creates a template's starter content.
func seedWorkspace(tx *gorm.DB, ws models.Workspace, tpl planner.Template, uid, locale string) error {
	for i, l := range tpl.Lists {
		cols := make([]planner.Column, len(l.Columns))
		for j, col := range l.Columns {
			cols[j] = planner.Column{Name: planner.Localize(col.Name, locale), Done: col.Done}
		}
		cols, err := planner.CleanColumns(cols)
		if err != nil {
			return err
		}
		list := models.TaskList{WorkspaceID: ws.ID, Name: planner.Localize(l.Name, locale), Color: l.Color, Icon: l.Icon, Columns: planner.EncodeColumns(cols), Rank: float64(i+1) * 1024}
		if err := tx.Create(&list).Error; err != nil {
			return err
		}
		if err := planner.ListFolder(tx, list, uid); err != nil {
			return err
		}
	}
	pages := []models.Page{{
		WorkspaceID: ws.ID, Title: planner.Localize("Welcome", locale), Icon: "👋",
		Content: planner.WelcomeDoc(ws.Name, locale), Rank: 1024, CreatedBy: uid, UpdatedBy: uid,
	}}
	for i, title := range tpl.Pages {
		pages = append(pages, models.Page{WorkspaceID: ws.ID, Title: planner.Localize(title, locale), Icon: "📝", Rank: float64(i+2) * 1024, CreatedBy: uid, UpdatedBy: uid})
	}
	for i := range pages {
		pages[i].ContentText = planner.DocText(pages[i].Content)
		if err := tx.Create(&pages[i]).Error; err != nil {
			return err
		}
	}
	if slices.Contains(decodeList(ws.Modules), planner.ModNotes) {
		tip := planner.Localize(planner.NoteTip, locale)
		n := models.Note{
			WorkspaceID: ws.ID, Title: planner.Localize("Quick notes", locale), Body: tip, PlainText: tip,
			Color: "yellow", Pinned: true, Items: "[]", Rank: 1024, CreatedBy: uid, UpdatedBy: uid,
		}
		if err := tx.Create(&n).Error; err != nil {
			return err
		}
	}
	return nil
}

// getWorkspace returns one workspace with its members.
//
//	@Summary	Get a workspace
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path		string	true	"Workspace id"
//	@Success	200	{object}	map[string]any
//	@Router		/workspaces/{ws} [get]
func (h *Handlers) getWorkspace(c *gin.Context) {
	ws, m, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"workspace": h.toWorkspaceResponse(ws, m), "members": h.membersOf(ws.ID)})
}

type updateWorkspaceRequest struct {
	Name        *string   `json:"name" binding:"omitempty,max=120"`
	Icon        *string   `json:"icon" binding:"omitempty,max=64"`
	Color       *string   `json:"color"`
	Description *string   `json:"description" binding:"omitempty,max=500"`
	Modules     *[]string `json:"modules"`
	Archived    *bool     `json:"archived"`
	GradeScale  *int      `json:"grade_scale"` // 20, 100, 10 or 5
	Holidays    *string   `json:"holidays"`    // a country code, "" none
	// The built-in assistant: kept out of it, and what it's told about it.
	AssistantOff   *bool   `json:"assistant_off"`
	AssistantNotes *string `json:"assistant_notes" binding:"omitempty,max=2000"`
}

// updateWorkspace changes a workspace's settings (owners).
//
//	@Summary	Update a workspace
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string					true	"Workspace id"
//	@Param		body	body		updateWorkspaceRequest	true	"Fields to change"
//	@Success	200		{object}	workspaceResponse
//	@Router		/workspaces/{ws} [put]
func (h *Handlers) updateWorkspace(c *gin.Context) {
	ws, m, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var req updateWorkspaceRequest
	if !bindJSON(c, &req) {
		return
	}
	up := map[string]any{}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the workspace a name"})
			return
		}
		up["name"] = n
	}
	if req.Icon != nil {
		up["icon"] = *req.Icon
	}
	if req.Color != nil {
		if !planner.ValidColor(*req.Color) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "color must be #rrggbb"})
			return
		}
		up["color"] = *req.Color
	}
	if req.Description != nil {
		up["description"] = *req.Description
	}
	if req.Modules != nil {
		mods, ok := planner.CleanModules(*req.Modules)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown or unavailable module"})
			return
		}
		up["modules"] = encodeList(mods)
	}
	if req.Archived != nil {
		if *req.Archived {
			now := time.Now()
			up["archived_at"] = &now
		} else {
			up["archived_at"] = nil
		}
	}
	if req.AssistantOff != nil {
		up["assistant_off"] = *req.AssistantOff
	}
	if req.AssistantNotes != nil {
		up["assistant_notes"] = strings.TrimSpace(*req.AssistantNotes)
	}
	if req.Holidays != nil {
		if *req.Holidays != "" && !holidays.Known(*req.Holidays) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown country for holidays"})
			return
		}
		up["holidays"] = *req.Holidays
	}
	if req.GradeScale != nil && *req.GradeScale != ws.GradeScale {
		if !planner.ValidScale(*req.GradeScale) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "grade_scale is 20, 100, 10 or 5"})
			return
		}
		up["grade_scale"] = *req.GradeScale
		// Goals are on the scale: keep them where they were.
		if old := ws.GradeScale; old > 0 {
			var ids []string
			h.db.Model(&models.Course{}).Where("workspace_id = ?", ws.ID).Pluck("id", &ids)
			if len(ids) > 0 {
				h.db.Model(&models.CourseGoal{}).Where("course_id IN ?", ids).
					Update("target", gorm.Expr("target * ? / ?", *req.GradeScale, old))
			}
		}
	}
	if len(up) > 0 {
		if err := h.db.Model(&ws).Updates(up).Error; err != nil {
			h.internalError(c, err, "could not update the workspace")
			return
		}
	}
	ws = reloadWorkspace(h, ws.ID)
	if req.Archived != nil {
		h.recordCtx(c, map[bool]string{true: "workspace.archive", false: "workspace.restore"}[*req.Archived], "workspace", ws.ID, nil)
	}
	h.publishWS(c, ws.ID, "workspace")
	c.JSON(http.StatusOK, h.toWorkspaceResponse(ws, m))
}

// deleteWorkspace deletes a workspace and everything in it (owners). The
// SPA confirms by having the name typed.
//
//	@Summary	Delete a workspace
//	@Tags		workspaces
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	204
//	@Router		/workspaces/{ws} [delete]
func (h *Handlers) deleteWorkspace(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	members := h.memberIDs(ws.ID)
	if err := h.db.Transaction(func(tx *gorm.DB) error { return purgeWorkspace(tx, ws.ID) }); err != nil {
		h.internalError(c, err, "could not delete the workspace")
		return
	}
	h.recordCtx(c, "workspace.delete", "workspace", ws.ID, gin.H{"name": ws.Name})
	for _, id := range members {
		h.events.Publish(id, liveEvent("workspaces"))
	}
	c.Status(http.StatusNoContent)
}

// purgeWorkspace deletes a workspace and all its content.
func purgeWorkspace(tx *gorm.DB, wsID string) error {
	var subs []string
	tx.Model(&models.CalendarSubscription{}).Where("workspace_id = ?", wsID).Pluck("id", &subs)
	var pages []string
	tx.Model(&models.Page{}).Where("workspace_id = ?", wsID).Pluck("id", &pages)
	var davLinks []string
	tx.Model(&models.CalDAVLink{}).Where("workspace_id = ?", wsID).Pluck("id", &davLinks)
	steps := []func() error{
		func() error {
			return tx.Where("subscription_id IN ?", append(subs, "")).Delete(&models.ExternalEvent{}).Error
		},
		func() error { return tx.Where("page_id IN ?", append(pages, "")).Delete(&models.PageFavorite{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.CalendarSubscription{}).Error },
		func() error { return tx.Where("link_id IN ?", append(davLinks, "")).Delete(&models.CalDAVItem{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.CalDAVLink{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Event{}).Error },
		func() error { return uploads.Delete(tx, "workspace_id = ?", wsID) },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.TaskComment{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Activity{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.TaskEvent{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Task{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.TaskList{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.GitLink{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Page{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.PageVersion{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.ShareLink{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.PageChange{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Form{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Note{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Drawing{}).Error },
		func() error { return deleteHabits(tx, "workspace_id = ?", wsID) },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.FocusSession{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Goal{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.SmartList{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Sheet{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Diagram{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Folder{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.TaskBlock{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.JournalEntry{}).Error },
		func() error { return deleteBookmarks(tx, "workspace_id = ?", wsID) },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.Notification{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.WorkspaceInvite{}).Error },
		func() error { return deleteCourses(tx, "workspace_id = ?", wsID) },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.WorkspaceMember{}).Error },
		func() error { return tx.Where("workspace_id = ?", wsID).Delete(&models.WorkspaceGroup{}).Error },
		func() error { return tx.Where("id = ?", wsID).Delete(&models.Workspace{}).Error },
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	return nil
}

type orderRequest struct {
	IDs []string `json:"ids" binding:"required"`
}

// orderWorkspaces sets the caller's sidebar order of their workspaces.
//
//	@Summary	Reorder my workspaces
//	@Tags		workspaces
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	orderRequest	true	"Workspace ids in order"
//	@Success	204
//	@Router		/me/workspace-order [put]
func (h *Handlers) orderWorkspaces(c *gin.Context) {
	var req orderRequest
	if !bindJSON(c, &req) {
		return
	}
	uid := auth.UserID(c)
	for i, id := range req.IDs {
		h.db.Model(&models.WorkspaceMember{}).Where("workspace_id = ? AND user_id = ?", id, uid).Update("position", float64(i+1)*1024)
	}
	h.events.Publish(uid, liveEvent("workspaces"))
	c.Status(http.StatusNoContent)
}

// ---- Members --------------------------------------------------------------------

type addMemberRequest struct {
	User string `json:"user" binding:"required"` // username or email
	Role string `json:"role" binding:"required,oneof=owner editor viewer"`
}

type memberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=owner editor viewer"`
}

// findAccount finds an enabled account by exact username or email (case
// insensitive). Only exact matches: the directory can't be browsed.
func (h *Handlers) findAccount(q string) (models.User, bool) {
	q = strings.ToLower(strings.TrimSpace(q))
	var u models.User
	if q == "" {
		return u, false
	}
	err := h.db.Where("disabled = ? AND (LOWER(username) = ? OR LOWER(email) = ?)", false, q, q).First(&u).Error
	return u, err == nil
}

// lookupUser finds an account by exact username or email, to invite it.
//
//	@Summary	Find an account to invite
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		q	query		string	true	"Exact username or email"
//	@Success	200	{object}	map[string]string
//	@Router		/users/lookup [get]
func (h *Handlers) lookupUser(c *gin.Context) {
	u, ok := h.findAccount(c.Query("q"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no account with that username or email"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": u.ID, "username": u.Username, "display_name": u.DisplayName, "avatar_url": u.AvatarURL})
}

// listMembers lists a workspace's members.
//
//	@Summary	Workspace members
//	@Tags		workspaces
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	memberResponse
//	@Router		/workspaces/{ws}/members [get]
func (h *Handlers) listMembers(c *gin.Context) {
	if _, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer); !ok {
		return
	}
	c.JSON(http.StatusOK, h.membersOf(c.Param("ws")))
}

// addMember adds an account to a workspace (owners).
//
//	@Summary	Add a member
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		body	body		addMemberRequest	true	"Who and with which role"
//	@Success	201		{array}		memberResponse
//	@Router		/workspaces/{ws}/members [post]
func (h *Handlers) addMember(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var req addMemberRequest
	if !bindJSON(c, &req) {
		return
	}
	u, found := h.findAccount(req.User)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "no account with that username or email"})
		return
	}
	var cur models.WorkspaceMember
	inAlready := h.db.Where("workspace_id = ? AND user_id = ?", ws.ID, u.ID).Limit(1).Find(&cur).RowsAffected > 0
	if inAlready && cur.DirectRole != "" {
		c.JSON(http.StatusConflict, gin.H{"error": u.Username + " is already a member"})
		return
	}
	if inAlready { // in through a group: now a role of their own too
		if _, err := h.regroup([]string{ws.ID}, func(tx *gorm.DB) error {
			return tx.Model(&cur).Update("direct_role", req.Role).Error
		}); h.regroupFailed(c, err) {
			return
		}
	} else {
		var last struct{ Max *float64 }
		h.db.Model(&models.WorkspaceMember{}).Select("MAX(position) AS max").Where("user_id = ?", u.ID).Scan(&last)
		m := models.WorkspaceMember{WorkspaceID: ws.ID, UserID: u.ID, Role: req.Role, DirectRole: req.Role, Position: planner.Rank(last.Max, nil)}
		if err := h.db.Create(&m).Error; err != nil {
			h.internalError(c, err, "could not add the member")
			return
		}
	}
	h.recordCtx(c, "workspace.member_add", "workspace", ws.ID, gin.H{"user": u.Username, "role": req.Role})
	h.events.Publish(u.ID, liveEvent("workspaces"))
	h.publishWS(c, ws.ID, "members")
	h.notifyMembership(c, ws, u.ID, "added", req.Role)
	h.notifyJoined(c, ws, u, req.Role)
	c.JSON(http.StatusCreated, h.membersOf(ws.ID))
}

// setMemberRole changes a member's role (owners). The last owner stays one.
//
//	@Summary	Change a member's role
//	@Tags		workspaces
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string				true	"Workspace id"
//	@Param		uid		path		string				true	"User id"
//	@Param		body	body		memberRoleRequest	true	"New role"
//	@Success	200		{array}		memberResponse
//	@Router		/workspaces/{ws}/members/{uid} [put]
func (h *Handlers) setMemberRole(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var req memberRoleRequest
	if !bindJSON(c, &req) {
		return
	}
	var m models.WorkspaceMember
	if h.db.Where("workspace_id = ? AND user_id = ?", ws.ID, c.Param("uid")).First(&m).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not a member"})
		return
	}
	before := m.Role
	if _, err := h.regroup([]string{ws.ID}, func(tx *gorm.DB) error {
		return tx.Model(&m).Update("direct_role", req.Role).Error
	}); h.regroupFailed(c, err) {
		return
	}
	h.db.First(&m, "id = ?", m.ID)
	changed := m.Role != before
	h.recordCtx(c, "workspace.member_role", "workspace", ws.ID, gin.H{"user_id": m.UserID, "role": req.Role})
	if changed {
		h.notifyMembership(c, ws, m.UserID, "role_changed", req.Role)
	}
	h.publishWS(c, ws.ID, "members")
	c.JSON(http.StatusOK, h.membersOf(ws.ID))
}

// removeMember removes a member (owners), or the caller leaves (anyone).
//
//	@Summary	Remove a member, or leave
//	@Tags		workspaces
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Param		uid	path	string	true	"User id (your own to leave)"
//	@Success	204
//	@Router		/workspaces/{ws}/members/{uid} [delete]
func (h *Handlers) removeMember(c *gin.Context) {
	target := c.Param("uid")
	need := models.WSOwner
	if target == auth.UserID(c) {
		need = models.WSViewer // leaving
	}
	ws, _, ok := h.wsAccess(c, c.Param("ws"), need)
	if !ok {
		return
	}
	var m models.WorkspaceMember
	if h.db.Where("workspace_id = ? AND user_id = ?", ws.ID, target).First(&m).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not a member"})
		return
	}
	if m.DirectRole == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "they're in it through a group: take the group out of the workspace, or them out of the group"})
		return
	}
	if _, err := h.regroup([]string{ws.ID}, func(tx *gorm.DB) error {
		return tx.Model(&m).Update("direct_role", "").Error
	}); err != nil {
		if errors.Is(err, planner.ErrNoOwner) {
			c.JSON(http.StatusConflict, gin.H{"error": "the last owner can't leave; make someone else an owner, or delete the workspace"})
			return
		}
		h.internalError(c, err, "could not remove the member")
		return
	}
	if h.db.Where("workspace_id = ? AND user_id = ?", ws.ID, target).Limit(1).Find(&models.WorkspaceMember{}).RowsAffected > 0 {
		// Still in through a group: only their own role went.
		h.recordCtx(c, "workspace.member_remove", "workspace", ws.ID, gin.H{"user_id": target, "still_via_group": true})
		h.publishWS(c, ws.ID, "members", target)
		c.Status(http.StatusNoContent)
		return
	}
	h.db.Where("user_id = ? AND page_id IN (?)", target, h.db.Model(&models.Page{}).Select("id").Where("workspace_id = ?", ws.ID)).Delete(&models.PageFavorite{})
	// Their own smart lists go with them; shared ones stay for the others.
	h.db.Where("workspace_id = ? AND owner_id = ? AND shared = ?", ws.ID, target, false).Delete(&models.SmartList{})
	h.recordCtx(c, "workspace.member_remove", "workspace", ws.ID, gin.H{"user_id": target})
	h.publishWS(c, ws.ID, "members", target)
	h.events.Publish(target, liveEvent("workspaces"))
	h.notifyMembership(c, ws, target, "removed", "")
	c.Status(http.StatusNoContent)
}

// releaseWorkspaces runs when an account is deleted: workspaces it was alone
// in are deleted; where it was the last owner, the longest-standing other
// member becomes the owner. Then its memberships, favourites and feeds go.
func releaseWorkspaces(tx *gorm.DB, userID string) error {
	var mine []models.WorkspaceMember
	if err := tx.Where("user_id = ?", userID).Find(&mine).Error; err != nil {
		return err
	}
	for _, m := range mine {
		var others []models.WorkspaceMember
		tx.Where("workspace_id = ? AND user_id <> ?", m.WorkspaceID, userID).Order("created_at").Find(&others)
		if len(others) == 0 {
			if err := purgeWorkspace(tx, m.WorkspaceID); err != nil {
				return err
			}
			continue
		}
		if m.Role == models.WSOwner {
			hasOwner := false
			for _, o := range others {
				hasOwner = hasOwner || o.Role == models.WSOwner
			}
			if !hasOwner {
				heir := others[0]
				for _, o := range others {
					if o.Role == models.WSEditor {
						heir = o
						break
					}
				}
				if err := tx.Model(&heir).Update("role", models.WSOwner).Error; err != nil {
					return err
				}
			}
		}
	}
	if err := tx.Where("owner_id = ? AND shared = ?", userID, false).Delete(&models.SmartList{}).Error; err != nil {
		return err
	}
	for _, q := range []any{&models.WorkspaceMember{}, &models.PageFavorite{}, &models.CalendarFeed{}, &models.GroupMember{}} {
		if err := tx.Where("user_id = ?", userID).Delete(q).Error; err != nil {
			return err
		}
	}
	return nil
}

// canCreateWorkspaces is for /auth/me: whether the "new workspace" button shows.
func canCreateWorkspaces(e rbac.Effective) bool {
	return e.HasGlobal(rbac.PWorkspacesCreate) && !e.ReadOnly
}

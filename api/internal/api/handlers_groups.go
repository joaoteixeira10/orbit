package api

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Groups: sets of people that carry instance roles and are added to
// workspaces with a role (planner.SyncGroupAccess). Kept by whoever manages
// users (users.manage); a group that makes its people admins — a superuser
// role, or one with an owner-only privilege — only by the owner, as making
// someone an admin is the owner's alone. A group the identity provider keeps
// (Source oidc / scim) isn't changed here, but for its roles and workspaces.

type groupResponse struct {
	models.Group
	RoleIDs    []string `json:"role_ids"`
	Members    int64    `json:"members"`
	Workspaces int64    `json:"workspaces"`
}

type groupMember struct {
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type groupWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type groupDetail struct {
	groupResponse
	People []groupMember    `json:"people"`
	In     []groupWorkspace `json:"in"`
}

func (h *Handlers) groupOut(g models.Group) groupResponse {
	out := groupResponse{Group: g, RoleIDs: []string{}}
	h.db.Model(&models.GroupRole{}).Where("group_id = ?", g.ID).Pluck("role_id", &out.RoleIDs)
	h.db.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&out.Members)
	h.db.Model(&models.WorkspaceGroup{}).Where("group_id = ?", g.ID).Count(&out.Workspaces)
	return out
}

// rolesMakeAdmins: whether roles make their holders admins — a superuser
// role, or one with an owner-only privilege.
func (h *Handlers) rolesMakeAdmins(ids []string) bool {
	if len(ids) == 0 {
		return false
	}
	var roles []models.Role
	h.db.Where("id IN ?", ids).Find(&roles)
	for _, r := range roles {
		if r.Superuser {
			return true
		}
		for _, p := range rbac.ParseStrings(r.GlobalPrivileges) {
			if rbac.IsOwnerOnlyGlobal(p) {
				return true
			}
		}
	}
	return false
}

// groupAccess loads :id; one that makes admins only for the owner.
func (h *Handlers) groupAccess(c *gin.Context) (models.Group, bool) {
	var g models.Group
	if h.db.First(&g, "id = ?", c.Param("id")).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return g, false
	}
	if c.Request.Method != http.MethodGet && !auth.IsOwner(c) {
		var ids []string
		h.db.Model(&models.GroupRole{}).Where("group_id = ?", g.ID).Pluck("role_id", &ids)
		if h.rolesMakeAdmins(ids) {
			c.JSON(http.StatusForbidden, gin.H{"error": "this group makes its people admins: only the owner changes it"})
			return g, false
		}
	}
	return g, true
}

// listGroups lists the groups.
//
//	@Summary	Groups
//	@Tags		groups
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	groupResponse
//	@Router		/groups [get]
func (h *Handlers) listGroups(c *gin.Context) {
	var gs []models.Group
	h.db.Order("name").Find(&gs)
	out := make([]groupResponse, len(gs))
	for i, g := range gs {
		out[i] = h.groupOut(g)
	}
	c.JSON(http.StatusOK, out)
}

// getGroup: a group, with its people and workspaces.
//
//	@Summary	A group
//	@Tags		groups
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Group id"
//	@Success	200	{object}	groupDetail
//	@Router		/groups/{id} [get]
func (h *Handlers) getGroup(c *gin.Context) {
	g, ok := h.groupAccess(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.groupDetailOf(g))
}

func (h *Handlers) groupDetailOf(g models.Group) groupDetail {
	d := groupDetail{groupResponse: h.groupOut(g), People: []groupMember{}, In: []groupWorkspace{}}
	h.db.Table("group_members").Select("users.id AS user_id, users.username, users.display_name, users.avatar_url").
		Joins("JOIN users ON users.id = group_members.user_id").Where("group_members.group_id = ?", g.ID).
		Order("users.username").Scan(&d.People)
	h.db.Table("workspace_groups").Select("workspaces.id, workspaces.name, workspace_groups.role").
		Joins("JOIN workspaces ON workspaces.id = workspace_groups.workspace_id").Where("workspace_groups.group_id = ?", g.ID).
		Order("workspaces.name").Scan(&d.In)
	return d
}

type groupRequest struct {
	Name        *string   `json:"name" binding:"omitempty,max=120"`
	Description *string   `json:"description" binding:"omitempty,max=500"`
	RoleIDs     *[]string `json:"role_ids"`
}

// checkGroupRoles: the roles exist, and making admins is the owner's.
func (h *Handlers) checkGroupRoles(c *gin.Context, ids []string) bool {
	if len(h.loadRoles(ids)) != len(ids) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown role"})
		return false
	}
	if h.rolesMakeAdmins(ids) && !auth.IsOwner(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can give a group an admin role"})
		return false
	}
	return true
}

func setGroupRoles(tx *gorm.DB, gid string, ids []string) error {
	if err := tx.Where("group_id = ?", gid).Delete(&models.GroupRole{}).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := tx.Create(&models.GroupRole{GroupID: gid, RoleID: id}).Error; err != nil {
			return err
		}
	}
	return nil
}

// createGroup makes a group.
//
//	@Summary	New group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		groupRequest	true	"The group"
//	@Success	201		{object}	groupResponse
//	@Router		/groups [post]
func (h *Handlers) createGroup(c *gin.Context) {
	var req groupRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "give the group a name"})
		return
	}
	g := models.Group{Name: strings.TrimSpace(*req.Name)}
	if req.Description != nil {
		g.Description = strings.TrimSpace(*req.Description)
	}
	var ids []string
	if req.RoleIDs != nil {
		ids = *req.RoleIDs
		if !h.checkGroupRoles(c, ids) {
			return
		}
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			return err
		}
		return setGroupRoles(tx, g.ID, ids)
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "there's a group of that name already"})
		return
	}
	h.recordCtx(c, "group.create", "group", g.ID, gin.H{"name": g.Name, "role_ids": ids})
	c.JSON(http.StatusCreated, h.groupOut(g))
}

// updateGroup renames a group or sets its roles.
//
//	@Summary	Change a group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Group id"
//	@Param		body	body		groupRequest	true	"What changes"
//	@Success	200		{object}	groupDetail
//	@Router		/groups/{id} [put]
func (h *Handlers) updateGroup(c *gin.Context) {
	g, ok := h.groupAccess(c)
	if !ok {
		return
	}
	var req groupRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Name != nil {
		if g.Source != "" {
			c.JSON(http.StatusConflict, gin.H{"error": "the identity provider keeps this group's name"})
			return
		}
		if strings.TrimSpace(*req.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the group a name"})
			return
		}
		g.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		g.Description = strings.TrimSpace(*req.Description)
	}
	if req.RoleIDs != nil && !h.checkGroupRoles(c, *req.RoleIDs) {
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&g).Error; err != nil {
			return err
		}
		if req.RoleIDs != nil {
			return setGroupRoles(tx, g.ID, *req.RoleIDs)
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "there's a group of that name already"})
		return
	}
	h.recordCtx(c, "group.update", "group", g.ID, gin.H{"name": g.Name, "role_ids": req.RoleIDs})
	c.JSON(http.StatusOK, h.groupDetailOf(g))
}

// deleteGroup deletes a group: its people lose what it gave them.
//
//	@Summary	Delete a group
//	@Tags		groups
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Group id"
//	@Success	204
//	@Router		/groups/{id} [delete]
func (h *Handlers) deleteGroup(c *gin.Context) {
	g, ok := h.groupAccess(c)
	if !ok {
		return
	}
	wss := planner.GroupWorkspaces(h.db, g.ID)
	changed, err := h.regroup(wss, func(tx *gorm.DB) error {
		for _, m := range []any{&models.WorkspaceGroup{}, &models.GroupMember{}, &models.GroupRole{}} {
			if err := tx.Where("group_id = ?", g.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&g).Error
	})
	if h.regroupFailed(c, err) {
		return
	}
	h.recordCtx(c, "group.delete", "group", g.ID, gin.H{"name": g.Name})
	h.regrouped(changed)
	c.Status(http.StatusNoContent)
}

type groupMemberRequest struct {
	User string `json:"user" binding:"required"` // username or email
}

// addGroupMember adds someone to a group.
//
//	@Summary	Add someone to a group
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Group id"
//	@Param		body	body		groupMemberRequest	true	"Who"
//	@Success	200		{object}	groupDetail
//	@Router		/groups/{id}/members [post]
func (h *Handlers) addGroupMember(c *gin.Context) {
	g, ok := h.groupAccess(c)
	if !ok || !h.handKept(c, g) {
		return
	}
	var req groupMemberRequest
	if !bindJSON(c, &req) {
		return
	}
	u, found := h.findAccount(req.User)
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "no account with that username or email"})
		return
	}
	changed, err := h.regroup(planner.GroupWorkspaces(h.db, g.ID), func(tx *gorm.DB) error {
		return tx.Where(models.GroupMember{GroupID: g.ID, UserID: u.ID}).FirstOrCreate(&models.GroupMember{GroupID: g.ID, UserID: u.ID}).Error
	})
	if h.regroupFailed(c, err) {
		return
	}
	h.recordCtx(c, "group.member_add", "group", g.ID, gin.H{"user": u.Username})
	h.regrouped(append(changed, u.ID))
	c.JSON(http.StatusOK, h.groupDetailOf(g))
}

// removeGroupMember takes someone out of a group.
//
//	@Summary	Remove someone from a group
//	@Tags		groups
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Group id"
//	@Param		uid	path	string	true	"User id"
//	@Success	200	{object}	groupDetail
//	@Router		/groups/{id}/members/{uid} [delete]
func (h *Handlers) removeGroupMember(c *gin.Context) {
	g, ok := h.groupAccess(c)
	if !ok || !h.handKept(c, g) {
		return
	}
	uid := c.Param("uid")
	changed, err := h.regroup(planner.GroupWorkspaces(h.db, g.ID), func(tx *gorm.DB) error {
		return tx.Where("group_id = ? AND user_id = ?", g.ID, uid).Delete(&models.GroupMember{}).Error
	})
	if h.regroupFailed(c, err) {
		return
	}
	h.recordCtx(c, "group.member_remove", "group", g.ID, gin.H{"user_id": uid})
	h.regrouped(append(changed, uid))
	c.JSON(http.StatusOK, h.groupDetailOf(g))
}

// handKept: a group the identity provider keeps isn't changed by hand.
func (h *Handlers) handKept(c *gin.Context, g models.Group) bool {
	if g.Source != "" {
		c.JSON(http.StatusConflict, gin.H{"error": "the identity provider keeps who's in this group"})
		return false
	}
	return true
}

// regroup runs a change to groups and brings the workspaces it touches in
// line, in one transaction: who changed, or ErrNoOwner (nothing done).
func (h *Handlers) regroup(wss []string, change func(tx *gorm.DB) error) ([]string, error) {
	var changed []string
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := change(tx); err != nil {
			return err
		}
		for _, ws := range wss {
			ch, err := planner.SyncGroupAccess(tx, ws)
			if err != nil {
				return err
			}
			changed = append(changed, ch...)
		}
		return nil
	})
	return changed, err
}

func (h *Handlers) regroupFailed(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, planner.ErrNoOwner):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		h.internalError(c, err, "could not change the group")
	}
	return true
}

// regrouped tells those whose access changed (their sidebar reloads).
func (h *Handlers) regrouped(users []string) {
	sort.Strings(users)
	last := ""
	for _, u := range users {
		if u != last {
			h.events.Publish(u, liveEvent("workspaces"))
		}
		last = u
	}
}

// ---- Groups in a workspace (its owners) ----

type wsGroupResponse struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Members int64  `json:"members"`
}

func (h *Handlers) groupsOf(wsID string) []wsGroupResponse {
	out := []wsGroupResponse{}
	h.db.Table("workspace_groups").Select("workspace_groups.group_id, groups.name, workspace_groups.role, (SELECT COUNT(*) FROM group_members WHERE group_members.group_id = workspace_groups.group_id) AS members").
		Joins("JOIN groups ON groups.id = workspace_groups.group_id").Where("workspace_groups.workspace_id = ?", wsID).
		Order("groups.name").Scan(&out)
	return out
}

// listWorkspaceGroups: the groups in a workspace.
//
//	@Summary	A workspace's groups
//	@Tags		groups
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Success	200	{array}	wsGroupResponse
//	@Router		/workspaces/{ws}/groups [get]
func (h *Handlers) listWorkspaceGroups(c *gin.Context) {
	if _, _, ok := h.wsAccess(c, c.Param("ws"), models.WSViewer); !ok {
		return
	}
	c.JSON(http.StatusOK, h.groupsOf(c.Param("ws")))
}

// groupNames: every group's name, for a workspace's owners to add one.
//
//	@Summary	Groups to add to a workspace
//	@Tags		groups
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	wsGroupResponse
//	@Router		/group-names [get]
func (h *Handlers) groupNames(c *gin.Context) {
	out := []wsGroupResponse{}
	h.db.Table("groups").Select("groups.id AS group_id, groups.name, (SELECT COUNT(*) FROM group_members WHERE group_members.group_id = groups.id) AS members").
		Order("groups.name").Scan(&out)
	c.JSON(http.StatusOK, out)
}

type wsGroupRequest struct {
	GroupID string `json:"group_id"`
	Role    string `json:"role" binding:"required,oneof=owner editor viewer"`
}

// addWorkspaceGroup adds a group to a workspace (or changes its role there).
//
//	@Summary	Add a group to a workspace
//	@Tags		groups
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		ws		path		string			true	"Workspace id"
//	@Param		body	body		wsGroupRequest	true	"Which, with which role"
//	@Success	200		{array}		wsGroupResponse
//	@Router		/workspaces/{ws}/groups [post]
func (h *Handlers) addWorkspaceGroup(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	var req wsGroupRequest
	if !bindJSON(c, &req) {
		return
	}
	if gid := c.Param("gid"); gid != "" {
		req.GroupID = gid
	}
	var g models.Group
	if h.db.First(&g, "id = ?", req.GroupID).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	changed, err := h.regroup([]string{ws.ID}, func(tx *gorm.DB) error {
		wg := models.WorkspaceGroup{WorkspaceID: ws.ID, GroupID: g.ID}
		return tx.Where(wg).Assign(models.WorkspaceGroup{Role: req.Role}).FirstOrCreate(&wg).Error
	})
	if h.regroupFailed(c, err) {
		return
	}
	h.recordCtx(c, "workspace.group_set", "workspace", ws.ID, gin.H{"group": g.Name, "role": req.Role})
	h.regrouped(changed)
	h.publishWS(c, ws.ID, "members")
	c.JSON(http.StatusOK, h.groupsOf(ws.ID))
}

// removeWorkspaceGroup takes a group out of a workspace: its people lose
// what it gave them there (a role of their own stays).
//
//	@Summary	Remove a group from a workspace
//	@Tags		groups
//	@Security	BearerAuth
//	@Param		ws	path	string	true	"Workspace id"
//	@Param		gid	path	string	true	"Group id"
//	@Success	200	{array}	wsGroupResponse
//	@Router		/workspaces/{ws}/groups/{gid} [delete]
func (h *Handlers) removeWorkspaceGroup(c *gin.Context) {
	ws, _, ok := h.wsAccess(c, c.Param("ws"), models.WSOwner)
	if !ok {
		return
	}
	gid := c.Param("gid")
	changed, err := h.regroup([]string{ws.ID}, func(tx *gorm.DB) error {
		return tx.Where("workspace_id = ? AND group_id = ?", ws.ID, gid).Delete(&models.WorkspaceGroup{}).Error
	})
	if h.regroupFailed(c, err) {
		return
	}
	h.recordCtx(c, "workspace.group_remove", "workspace", ws.ID, gin.H{"group_id": gid})
	h.regrouped(changed)
	h.publishWS(c, ws.ID, "members", changed...)
	c.JSON(http.StatusOK, h.groupsOf(ws.ID))
}

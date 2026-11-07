package api

import (
	"net/http"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
)

// ---- DTOs ----

type roleRequest struct {
	Name             string   `json:"name" binding:"required"`
	Description      string   `json:"description"`
	GlobalPrivileges []string `json:"global_privileges"`
}

type roleResponse struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Builtin          bool     `json:"builtin"`
	Superuser        bool     `json:"superuser"`
	GlobalPrivileges []string `json:"global_privileges"`
}

func toRoleResponse(r models.Role) roleResponse {
	return roleResponse{
		ID: r.ID, Name: r.Name, Description: r.Description,
		Builtin: r.Builtin, Superuser: r.Superuser,
		GlobalPrivileges: rbac.DecodeStrings(r.GlobalPrivileges),
	}
}

// validateRolePrivileges ensures all referenced privileges exist in the
// catalog. It returns a message for the first problem.
func validateRolePrivileges(req *roleRequest) (problem string, ok bool) {
	for _, p := range req.GlobalPrivileges {
		if !rbac.IsValidGlobal(p) {
			return "unknown privilege: " + p, false
		}
	}
	return "", true
}

// ownerOnlySet returns the owner-only privileges present in a list.
func ownerOnlySet(privs []string) map[string]bool {
	m := map[string]bool{}
	for _, p := range privs {
		if rbac.IsOwnerOnlyGlobal(p) {
			m[p] = true
		}
	}
	return m
}

func sameStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// ---- Handlers ----

// listPrivileges returns the privilege catalog for the role editor.
//
//	@Summary	List available privileges
//	@Tags		roles
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string][]string
//	@Router		/privileges [get]
func (h *Handlers) listPrivileges(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"global": rbac.GlobalPrivileges})
}

// listRoles returns all roles.
//
//	@Summary	List roles
//	@Tags		roles
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	roleResponse
//	@Router		/roles [get]
func (h *Handlers) listRoles(c *gin.Context) {
	var roles []models.Role
	if err := h.db.Order("id").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list roles"})
		return
	}
	out := make([]roleResponse, 0, len(roles))
	for _, r := range roles {
		out = append(out, toRoleResponse(r))
	}
	c.JSON(http.StatusOK, out)
}

// getRole returns one role.
//
//	@Summary	Get role
//	@Tags		roles
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"Role ID"
//	@Success	200	{object}	roleResponse
//	@Router		/roles/{id} [get]
func (h *Handlers) getRole(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var role models.Role
	if err := h.db.First(&role, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, toRoleResponse(role))
}

// createRole creates a custom role.
//
//	@Summary	Create role
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		roleRequest	true	"Role"
//	@Success	201		{object}	roleResponse
//	@Router		/roles [post]
func (h *Handlers) createRole(c *gin.Context) {
	var req roleRequest
	if !bindJSON(c, &req) {
		return
	}
	if bad, ok := validateRolePrivileges(&req); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": bad})
		return
	}
	// Only the instance owner may grant owner-only privileges (branding /
	// notifications / impersonate).
	if !auth.IsOwner(c) && len(ownerOnlySet(req.GlobalPrivileges)) > 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can grant owner-only privileges"})
		return
	}
	var existing int64
	h.db.Model(&models.Role{}).Where("name = ?", req.Name).Count(&existing)
	if existing > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "role name already exists"})
		return
	}
	role := models.Role{
		Name:             req.Name,
		Description:      req.Description,
		Builtin:          false,
		GlobalPrivileges: rbac.EncodeStrings(req.GlobalPrivileges),
	}
	if err := h.db.Create(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create role"})
		return
	}
	h.recordCtx(c, "role.create", "role", role.ID, gin.H{"name": role.Name})
	h.notifyRolesChanged()
	c.JSON(http.StatusCreated, toRoleResponse(role))
}

// updateRole edits a custom role (built-ins are immutable).
//
//	@Summary	Update role
//	@Tags		roles
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int			true	"Role ID"
//	@Param		body	body		roleRequest	true	"Role"
//	@Success	200		{object}	roleResponse
//	@Router		/roles/{id} [put]
func (h *Handlers) updateRole(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req roleRequest
	if !bindJSON(c, &req) {
		return
	}
	if bad, ok := validateRolePrivileges(&req); !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": bad})
		return
	}
	var role models.Role
	if err := h.db.First(&role, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.Builtin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "built-in roles cannot be edited"})
		return
	}
	// A non-owner may neither add nor remove owner-only privileges on a role.
	if !auth.IsOwner(c) && !sameStringSet(ownerOnlySet(rbac.DecodeStrings(role.GlobalPrivileges)), ownerOnlySet(req.GlobalPrivileges)) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner can change owner-only privileges"})
		return
	}
	// Replace the fields.
	if err := h.db.Model(&role).Updates(map[string]any{
		"name":              req.Name,
		"description":       req.Description,
		"global_privileges": rbac.EncodeStrings(req.GlobalPrivileges),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update role"})
		return
	}
	h.recordCtx(c, "role.update", "role", role.ID, nil)
	// Editing a role changes the privileges of everyone who holds it.
	h.notifyPrivilegeChange(h.usersWithRole(role.ID)...)
	h.notifyRolesChanged()
	h.db.First(&role, "id = ?", id)
	c.JSON(http.StatusOK, toRoleResponse(role))
}

// deleteRole removes a custom role (built-ins are protected).
//
//	@Summary	Delete role
//	@Tags		roles
//	@Security	BearerAuth
//	@Param		id	path	int	true	"Role ID"
//	@Success	204
//	@Router		/roles/{id} [delete]
func (h *Handlers) deleteRole(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var role models.Role
	if err := h.db.First(&role, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if role.Builtin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "built-in roles cannot be deleted"})
		return
	}
	// Capture holders before we drop the assignments, so we can notify them.
	affected := h.usersWithRole(role.ID)
	// Remove assignments, then the role.
	h.db.Exec("DELETE FROM user_roles WHERE role_id = ?", role.ID)
	if err := h.db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete role"})
		return
	}
	h.recordCtx(c, "role.delete", "role", id, gin.H{"name": role.Name})
	h.notifyPrivilegeChange(affected...)
	h.notifyRolesChanged()
	c.Status(http.StatusNoContent)
}

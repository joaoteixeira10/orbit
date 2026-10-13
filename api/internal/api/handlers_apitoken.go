package api

import (
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
)

type createAPITokenRequest struct {
	Name          string `json:"name" binding:"required"`
	ExpiresInDays int    `json:"expires_in_days"` // 0 = never expires
	// Optional narrowing (never widens the owner's rights): only these
	// instance privileges, and read-only access to workspace content.
	Privileges []string `json:"privileges"`
	ReadOnly   bool     `json:"read_only"`
}

// createAPIToken mints a personal API token (inheriting the caller's RBAC). The
// plaintext is returned exactly once.
//
//	@Summary	Create an API token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		createAPITokenRequest	true	"Token name + optional expiry"
//	@Success	201		{object}	map[string]any
//	@Router		/me/api-tokens [post]
func (h *Handlers) createAPIToken(c *gin.Context) {
	// A token can't mint tokens: a limited one would otherwise get around its
	// own limits (a new token starts from the owner's full rights).
	if auth.ViaAPIToken(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "API tokens can't create API tokens; create them in Settings → API tokens"})
		return
	}
	var req createAPITokenRequest
	if !bindJSON(c, &req) {
		return
	}
	for _, p := range req.Privileges {
		if !rbac.IsValidGlobal(p) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown privilege: " + p})
			return
		}
	}
	plaintext, hash, prefix := auth.NewAPITokenSecret()
	row := models.APIToken{UserID: auth.UserID(c), Name: req.Name, Prefix: prefix, TokenHash: hash, ReadOnly: req.ReadOnly}
	if len(req.Privileges) > 0 {
		row.ScopePrivileges, row.Privileges = rbac.EncodeStrings(req.Privileges), req.Privileges
	}
	if req.ExpiresInDays > 0 {
		exp := time.Now().AddDate(0, 0, req.ExpiresInDays)
		row.ExpiresAt = &exp
	}
	if err := h.db.Create(&row).Error; err != nil {
		h.internalError(c, err, "could not create token")
		return
	}
	h.recordCtx(c, "auth.api_token_create", "api_token", row.ID, nil)
	c.JSON(http.StatusCreated, gin.H{"token": plaintext, "api_token": row})
}

// listAPITokens lists the caller's active API tokens (no secrets).
//
//	@Summary	List my API tokens
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	models.APIToken
//	@Router		/me/api-tokens [get]
func (h *Handlers) listAPITokens(c *gin.Context) {
	rows := []models.APIToken{} // non-nil → marshals to [] (not null) when empty
	h.db.Where("user_id = ? AND revoked = ?", auth.UserID(c), false).Order("created_at desc").Find(&rows)
	c.JSON(http.StatusOK, rows)
}

// deleteAPIToken revokes one of the caller's API tokens.
//
//	@Summary	Revoke an API token
//	@Tags		auth
//	@Security	BearerAuth
//	@Param		id	path	int	true	"Token id"
//	@Success	204
//	@Router		/me/api-tokens/{id} [delete]
func (h *Handlers) deleteAPIToken(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	res := h.db.Where("id = ? AND user_id = ?", id, auth.UserID(c)).Delete(&models.APIToken{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	h.recordCtx(c, "auth.api_token_revoke", "api_token", id, nil)
	c.Status(http.StatusNoContent)
}

// listAllAPITokens lists every API token (admin oversight).
//
//	@Summary	List all API tokens (admin)
//	@Tags		users
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	models.APIToken
//	@Router		/api-tokens [get]
func (h *Handlers) listAllAPITokens(c *gin.Context) {
	rows := []models.APIToken{}
	h.db.Where("revoked = ?", false).Order("created_at desc").Find(&rows)
	var users []models.User
	h.db.Select("id", "username").Find(&users)
	names := map[string]string{}
	for _, u := range users {
		names[u.ID] = u.Username
	}
	for i := range rows {
		rows[i].Username = names[rows[i].UserID]
	}
	c.JSON(http.StatusOK, rows)
}

// revokeAnyAPIToken revokes any user's API token (admin).
//
//	@Summary	Revoke any API token (admin)
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	int	true	"Token id"
//	@Success	204
//	@Router		/api-tokens/{id} [delete]
func (h *Handlers) revokeAnyAPIToken(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var tk models.APIToken
	if err := h.db.First(&tk, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	var owner models.User
	if h.db.First(&owner, "id = ?", tk.UserID).Error == nil && !h.mayManage(c, owner) {
		return
	}
	h.db.Delete(&tk)
	h.recordCtx(c, "auth.api_token_revoke_admin", "api_token", id, nil)
	c.Status(http.StatusNoContent)
}

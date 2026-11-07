package api

import (
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// impersonationEnabled gates impersonation behind its feature flag; when off the
// route 404s so the instance behaves exactly as before.
func (h *Handlers) impersonationEnabled(c *gin.Context) bool {
	if feature.Enabled(h.db, feature.Impersonation) {
		return true
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	return false
}

// impersonateUser issues a short-lived access token that authenticates as the
// target user (stamped with the caller's id in the imp claim). No refresh token
// is returned, so the impersonation session ends on its own within the access
// token TTL; the client keeps its own tokens to return to afterwards.
//
// Privilege-escalation guard: the instance owner can never be impersonated, and
// only the owner may step into a superuser administrator. Combined with the
// users.impersonate privilege gate on the route, a regular admin can only ever
// drop into a lower-or-equal-privilege account.
//
//	@Summary	Impersonate a user
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path		int	true	"User id"
//	@Success	200	{object}	tokenResponse
//	@Router		/users/{id}/impersonate [post]
func (h *Handlers) impersonateUser(c *gin.Context) {
	if !h.impersonationEnabled(c) {
		return
	}
	id, ok := idParam(c)
	if !ok {
		return
	}
	if id == auth.UserID(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "you cannot impersonate yourself"})
		return
	}
	var target models.User
	if err := h.db.First(&target, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Disabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot impersonate a disabled account"})
		return
	}
	if target.Owner {
		c.JSON(http.StatusForbidden, gin.H{"error": "the instance owner cannot be impersonated"})
		return
	}
	if h.userIsSuperuser(target.ID) && !auth.IsOwner(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the owner may impersonate an administrator"})
		return
	}

	tok, exp, err := h.auth.IssueImpersonationToken(&target, auth.UserID(c), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start impersonation"})
		return
	}
	h.recordCtx(c, "user.impersonate", "user", target.ID, nil)
	h.securityAlert(c, "impersonation", "Impersonation started on "+target.Username)
	c.JSON(http.StatusOK, tokenResponse{
		AccessToken: tok,
		ExpiresIn:   int(time.Until(exp).Seconds()),
		User:        &target,
	})
}

// endImpersonation records that an impersonation ended: the admin, back in
// their own session, says whose it was (stopped by hand, or its short token
// ran out). The start is user.impersonate; between the two, every change
// carries the admin as impersonator.
//
//	@Summary	Record the end of an impersonation
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id		path	string					true	"The impersonated user's id"
//	@Param		body	body	endImpersonationRequest	false	"Why it ended"
//	@Success	204
//	@Router		/users/{id}/impersonate/end [post]
func (h *Handlers) endImpersonation(c *gin.Context) {
	if !h.impersonationEnabled(c) {
		return
	}
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req endImpersonationRequest
	_ = c.ShouldBindJSON(&req)
	if req.Reason != "expired" {
		req.Reason = "returned"
	}
	var target models.User
	if err := h.db.First(&target, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	h.recordCtx(c, "user.impersonate_end", "user", target.ID, gin.H{"reason": req.Reason})
	c.Status(http.StatusNoContent)
}

type endImpersonationRequest struct {
	Reason string `json:"reason"` // returned | expired
}

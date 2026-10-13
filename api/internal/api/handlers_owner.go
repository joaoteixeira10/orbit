package api

import (
	"net/http"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// listFeatures returns the instance feature flags (any authenticated user — the
// SPA gates UI on them). Writing is owner-only (updateFeatures).
//
//	@Summary	List feature flags
//	@Tags		owner
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]bool
//	@Router		/features [get]
func (h *Handlers) listFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, feature.All(h.db))
}

// updateFeatures toggles feature flags (owner only). Unknown flag names are
// rejected so the catalog stays the source of truth.
//
//	@Summary	Update feature flags
//	@Tags		owner
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		map[string]bool	true	"flag -> enabled"
//	@Success	200		{object}	map[string]bool
//	@Router		/features [put]
func (h *Handlers) updateFeatures(c *gin.Context) {
	var req map[string]bool
	if !bindJSON(c, &req) {
		return
	}
	for name, on := range req {
		if !feature.IsKnown(name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown feature: " + name})
			return
		}
		if err := feature.Set(h.db, name, on); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update features"})
			return
		}
	}
	h.recordCtx(c, "owner.features_update", "settings", "features", req)
	c.JSON(http.StatusOK, feature.All(h.db))
}

type transferOwnerRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// transferOwner moves instance ownership to another (active, local) user. Owner
// only; exactly one owner exists at any time.
//
//	@Summary	Transfer instance ownership
//	@Tags		owner
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	transferOwnerRequest	true	"New owner user id"
//	@Success	204
//	@Router		/owner/transfer [post]
func (h *Handlers) transferOwner(c *gin.Context) {
	var req transferOwnerRequest
	if !bindJSON(c, &req) {
		return
	}
	me := auth.UserID(c)
	if req.UserID == me {
		c.JSON(http.StatusBadRequest, gin.H{"error": "already the owner"})
		return
	}
	var target models.User
	if err := h.db.First(&target, "id = ?", req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if target.Disabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot transfer ownership to a disabled account"})
		return
	}
	// Single owner: clear the current, set the target, in one transaction.
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("owner = ?", true).Update("owner", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", target.ID).Update("owner", true).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not transfer ownership"})
		return
	}
	h.audit.Record(me, "owner.transfer", "user", target.ID, c.ClientIP(), gin.H{"to": target.Username})
	h.securityAlert(c, "owner_transfer", "Instance ownership transferred to "+target.Username)
	c.Status(http.StatusNoContent)
}

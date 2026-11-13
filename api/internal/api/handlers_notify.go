package api

import (
	"net/http"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/feature"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/notify"

	"github.com/gin-gonic/gin"
)

// getNotifConfig returns the channel configuration (admin).
//
//	@Summary	Get notification config
//	@Tags		notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	notify.Config
//	@Router		/notifications/config [get]
func (h *Handlers) getNotifConfig(c *gin.Context) {
	c.JSON(http.StatusOK, notify.Redact(h.notify.Load()))
}

// updateNotifConfig saves the channel configuration (admin).
//
//	@Summary	Update notification config
//	@Tags		notifications
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		notify.Config	true	"Channel config"
//	@Success	200		{object}	notify.Config
//	@Router		/notifications/config [put]
func (h *Handlers) updateNotifConfig(c *gin.Context) {
	var incoming notify.Config
	if !bindJSON(c, &incoming) {
		return
	}
	// Preserve any secret the UI left redacted (unchanged).
	cfg := notify.Merge(h.notify.Load(), incoming)
	if err := h.notify.Save(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save config"})
		return
	}
	h.recordCtx(c, "notifications.config_update", "settings", "notifications", nil)
	c.JSON(http.StatusOK, notify.Redact(h.notify.Load()))
}

type testNotifRequest struct {
	Channel string `json:"channel" binding:"required"`
}

// testNotif sends a test message via a channel (admin). Email goes to the
// admin's own address.
//
//	@Summary	Send a test notification
//	@Tags		notifications
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body	testNotifRequest	true	"Channel to test"
//	@Success	204
//	@Failure	400	{object}	map[string]string
//	@Router		/notifications/test [post]
func (h *Handlers) testNotif(c *gin.Context) {
	var req testNotifRequest
	if !bindJSON(c, &req) {
		return
	}
	var u models.User
	h.db.Select("email").First(&u, "id = ?", auth.UserID(c))
	if err := h.notify.Test(req.Channel, u.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "sent"})
}

// notifChannels returns the channel keys the admin has enabled, and the
// kinds people may add as their own channels (for any user).
//
//	@Summary	List enabled notification channels
//	@Tags		notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string][]string
//	@Router		/notifications/channels [get]
func (h *Handlers) notifChannels(c *gin.Context) {
	personal := []string{}
	if feature.Enabled(h.db, feature.PersonalChannels) {
		personal = h.notify.PersonalAllowed()
	}
	c.JSON(http.StatusOK, gin.H{"enabled": h.notify.EnabledChannels(), "personal": personal})
}

type personalKindsRequest struct {
	Kinds []string `json:"kinds" binding:"max=10"`
}

// setPersonalKinds sets which kinds people may add as their own channels
// (the owner). A channel of a kind no longer allowed is kept but gets
// nothing.
//
//	@Summary	Kinds of personal channels allowed
//	@Tags		notifications
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		personalKindsRequest	true	"Allowed kinds"
//	@Success	200		{object}	map[string][]string
//	@Router		/notifications/personal-kinds [put]
func (h *Handlers) setPersonalKinds(c *gin.Context) {
	var req personalKindsRequest
	if !bindJSON(c, &req) {
		return
	}
	kinds, err := h.notify.SetPersonalAllowed(req.Kinds)
	if err != nil {
		h.internalError(c, err, "could not save")
		return
	}
	h.recordCtx(c, "owner.personal_channel_kinds", "settings", "notifications", gin.H{"kinds": kinds})
	c.JSON(http.StatusOK, gin.H{"personal": kinds})
}

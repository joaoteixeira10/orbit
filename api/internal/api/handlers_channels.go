package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"

	"github.com/gin-gonic/gin"
)

// A person's own notification channels (flag personal_channels, on by
// default): their Discord / Slack / Teams webhook, Telegram chat, ntfy topic
// or Gotify app. Theirs alone, so they get the full text. The settings are
// sealed and never shown again (only a hint); sending goes through the
// public-only client.

const maxPersonalChannels = 10

func (h *Handlers) personalOn(c *gin.Context) bool {
	if !feature.Enabled(h.db, feature.PersonalChannels) {
		c.JSON(http.StatusNotFound, gin.H{"error": "personal channels are turned off on this instance"})
		return false
	}
	return true
}

// listMyChannels lists the caller's own channels (no secrets).
//
//	@Summary	My own notification channels
//	@Tags		prefs
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	models.PersonalChannel
//	@Router		/me/channels [get]
func (h *Handlers) listMyChannels(c *gin.Context) {
	if !h.personalOn(c) {
		return
	}
	var rows []models.PersonalChannel
	h.db.Where("user_id = ?", auth.UserID(c)).Order("created_at").Find(&rows)
	out := make([]personalResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, personalResponse{PersonalChannel: r, Allowed: h.notify.PersonalKindAllowed(r.Kind)})
	}
	c.JSON(http.StatusOK, out)
}

type personalResponse struct {
	models.PersonalChannel
	Allowed bool `json:"allowed"` // its kind is allowed (else it gets nothing)
}

type personalRequest struct {
	Name string `json:"name" binding:"max=60"`
	notify.Personal
}

// addMyChannel adds one of the caller's own channels and sends it a test.
//
//	@Summary	Add my own notification channel
//	@Tags		prefs
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		personalRequest	true	"Kind and its settings"
//	@Success	201		{object}	models.PersonalChannel
//	@Router		/me/channels [post]
func (h *Handlers) addMyChannel(c *gin.Context) {
	if !h.personalOn(c) {
		return
	}
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return
	}
	var req personalRequest
	if !bindJSON(c, &req) {
		return
	}
	p := req.Personal
	p.WebhookURL, p.ServerURL = strings.TrimSpace(p.WebhookURL), strings.TrimRight(strings.TrimSpace(p.ServerURL), "/")
	p.Topic, p.ChatID, p.BotToken, p.Token = strings.TrimSpace(p.Topic), strings.TrimSpace(p.ChatID), strings.TrimSpace(p.BotToken), strings.TrimSpace(p.Token)
	if err := p.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.notify.PersonalKindAllowed(p.Kind) {
		c.JSON(http.StatusForbidden, gin.H{"error": "this kind of channel isn't allowed on this instance"})
		return
	}
	uid := auth.UserID(c)
	var n int64
	h.db.Model(&models.PersonalChannel{}).Where("user_id = ?", uid).Count(&n)
	if n >= maxPersonalChannels {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ten channels at most"})
		return
	}
	// It must answer before it's kept.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := h.notify.SendPersonal(ctx, p, "Orbit", testText(h.prefsLocale(uid))); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the channel didn't take the test message: " + err.Error()})
		return
	}
	sealed, err := h.notify.SealPersonal(p)
	if err != nil {
		h.internalError(c, err, "could not store the channel")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.ToUpper(p.Kind[:1]) + p.Kind[1:]
	}
	ch := models.PersonalChannel{UserID: uid, Kind: p.Kind, Name: name, Hint: p.Hint(), ConfigSealed: sealed}
	if err := h.db.Create(&ch).Error; err != nil {
		h.internalError(c, err, "could not store the channel")
		return
	}
	h.recordCtx(c, "user.channel_add", "user", uid, gin.H{"kind": ch.Kind, "hint": ch.Hint})
	c.JSON(http.StatusCreated, personalResponse{PersonalChannel: ch, Allowed: true})
}

func (h *Handlers) myChannel(c *gin.Context) (models.PersonalChannel, bool) {
	var ch models.PersonalChannel
	if h.db.First(&ch, "id = ? AND user_id = ?", c.Param("id"), auth.UserID(c)).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return ch, false
	}
	return ch, true
}

// testMyChannel sends one of the caller's channels a test message.
//
//	@Summary	Test my own notification channel
//	@Tags		prefs
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Channel id"
//	@Success	204
//	@Router		/me/channels/{id}/test [post]
func (h *Handlers) testMyChannel(c *gin.Context) {
	if !h.personalOn(c) {
		return
	}
	ch, ok := h.myChannel(c)
	if !ok {
		return
	}
	if !h.notify.PersonalKindAllowed(ch.Kind) {
		c.JSON(http.StatusForbidden, gin.H{"error": "this kind of channel isn't allowed on this instance"})
		return
	}
	p, err := h.notify.OpenPersonal(ch.ConfigSealed)
	if err != nil {
		h.internalError(c, err, "could not read the channel")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if err := h.notify.SendPersonal(ctx, p, "Orbit", testText(h.prefsLocale(ch.UserID))); err != nil {
		h.db.Model(&ch).Update("last_error", truncate(err.Error(), 300))
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	h.db.Model(&ch).Update("last_error", "")
	c.Status(http.StatusNoContent)
}

// deleteMyChannel removes one of the caller's channels.
//
//	@Summary	Remove my own notification channel
//	@Tags		prefs
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Channel id"
//	@Success	204
//	@Router		/me/channels/{id} [delete]
func (h *Handlers) deleteMyChannel(c *gin.Context) {
	ch, ok := h.myChannel(c)
	if !ok {
		return
	}
	if auth.Effective(c).ReadOnly {
		c.JSON(http.StatusForbidden, gin.H{"error": "this API token is read-only"})
		return
	}
	h.db.Delete(&ch)
	h.recordCtx(c, "user.channel_remove", "user", ch.UserID, gin.H{"kind": ch.Kind, "hint": ch.Hint})
	c.Status(http.StatusNoContent)
}

func (h *Handlers) prefsLocale(uid string) string {
	var p models.UserPrefs
	h.db.Select("locale").Where("user_id = ?", uid).First(&p)
	return p.Locale
}

func testText(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		return "Este canal vai receber as suas notificações do Orbit."
	}
	return "This channel will get your Orbit notifications."
}

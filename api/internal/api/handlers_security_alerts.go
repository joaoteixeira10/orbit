package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"

	"github.com/gin-gonic/gin"
)

const settingSecurityAlerts = "security_alerts"

// securityAlertEvents is the catalog of alertable high-signal security events.
// Each is owner-toggleable; all default off (opt-in).
var securityAlertEvents = []string{
	"impersonation",
	"owner_transfer",
	"admin_grant",
	"config_change",
	"refresh_reuse",
	"backup_export",
}

// alertChannelKeys is the set of valid notification-channel keys.
var alertChannelKeys = notify.AllChannels

// securityAlertConfig is the owner-managed alert configuration: which events
// fire, which channels they go to, and the email recipient. Channels nil means
// "all enabled channels" (the default until the owner narrows it); a non-nil
// (possibly empty) list is taken literally. Email empty falls back to the owner.
type securityAlertConfig struct {
	Events   map[string]bool `json:"events"`
	Channels []string        `json:"channels"`
	Email    string          `json:"email"`
}

func (h *Handlers) loadSecurityAlertConfig() securityAlertConfig {
	cfg := securityAlertConfig{Events: map[string]bool{}}
	for _, e := range securityAlertEvents {
		cfg.Events[e] = false
	}
	var row models.Setting
	if err := h.db.Where(&models.Setting{Key: settingSecurityAlerts}).First(&row).Error; err == nil && row.Value != "" {
		var stored securityAlertConfig
		if json.Unmarshal([]byte(row.Value), &stored) == nil && stored.Events != nil {
			for _, e := range securityAlertEvents {
				if v, ok := stored.Events[e]; ok {
					cfg.Events[e] = v
				}
			}
			cfg.Channels = stored.Channels
			cfg.Email = stored.Email
		} else {
			// Legacy format: a flat {event: bool} map.
			flat := map[string]bool{}
			if json.Unmarshal([]byte(row.Value), &flat) == nil {
				for _, e := range securityAlertEvents {
					if v, ok := flat[e]; ok {
						cfg.Events[e] = v
					}
				}
			}
		}
	}
	return cfg
}

// alertChannels resolves the channels an alert should use: the configured set
// intersected with the currently-enabled channels (nil config → all enabled).
func (h *Handlers) alertChannels(cfg securityAlertConfig) []string {
	enabled := h.notify.EnabledChannels()
	if cfg.Channels == nil {
		return enabled
	}
	set := map[string]bool{}
	for _, ch := range enabled {
		set[ch] = true
	}
	out := []string{}
	for _, ch := range cfg.Channels {
		if set[ch] {
			out = append(out, ch)
		}
	}
	return out
}

// securityAlert sends an owner alert for a high-signal action when the event is
// enabled, honouring the configured channels + recipient. Best-effort and
// asynchronous, so it never blocks or fails the action it describes.
func (h *Handlers) securityAlert(c *gin.Context, event, action string) {
	h.raiseSecurityAlert(event, action, h.usernameOf(auth.UserID(c)), c.ClientIP())
}

// raiseSecurityAlert is the context-independent core of securityAlert: it
// dispatches the owner alert when the event is enabled, honouring the configured
// channels + recipient. by/ip may be empty for system-originated alerts.
// Best-effort and
// asynchronous — it never blocks or fails the action it describes.
func (h *Handlers) raiseSecurityAlert(event, action, by, ip string) {
	if h.notify == nil {
		return
	}
	cfg := h.loadSecurityAlertConfig()
	if !cfg.Events[event] {
		return
	}
	channels := h.alertChannels(cfg)
	if len(channels) == 0 {
		return
	}
	recipient := cfg.Email
	if recipient == "" {
		recipient = h.ownerEmail()
	}
	if by == "" {
		by = "system"
	}
	body := fmt.Sprintf("%s\n\nBy: %s\nIP: %s\nTime: %s",
		action, by, ip, time.Now().UTC().Format(time.RFC3339))
	go h.notify.Send(channels, recipient, "Security alert: "+action, body)
}

// ownerEmail returns the instance owner's email (default recipient for emailed
// security alerts), or "" if unset.
func (h *Handlers) ownerEmail() string {
	var u models.User
	if err := h.db.Where("owner = ?", true).Select("email").First(&u).Error; err == nil {
		return u.Email
	}
	return ""
}

// usernameOf resolves a user id to a username, or "#<id>" when unresolvable.
func (h *Handlers) usernameOf(id string) string {
	var u models.User
	if err := h.db.Select("username").First(&u, "id = ?", id).Error; err == nil && u.Username != "" {
		return u.Username
	}
	return "#" + id
}

func (h *Handlers) securityAlertsResponse(cfg securityAlertConfig) gin.H {
	return gin.H{
		"events":             cfg.Events,
		"channels":           cfg.Channels,
		"email":              cfg.Email,
		"available_channels": h.notify.EnabledChannels(),
	}
}

// getSecurityAlerts returns the alert configuration (owner only).
//
//	@Summary	Get security-alert configuration
//	@Tags		owner
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/security-alerts [get]
func (h *Handlers) getSecurityAlerts(c *gin.Context) {
	c.JSON(http.StatusOK, h.securityAlertsResponse(h.loadSecurityAlertConfig()))
}

type securityAlertUpdate struct {
	Events   map[string]bool `json:"events"`
	Channels *[]string       `json:"channels"`
	Email    *string         `json:"email"`
}

// updateSecurityAlerts persists the alert configuration (owner only).
//
//	@Summary	Update security-alert configuration
//	@Tags		owner
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]interface{}
//	@Router		/security-alerts [put]
func (h *Handlers) updateSecurityAlerts(c *gin.Context) {
	var req securityAlertUpdate
	if !bindJSON(c, &req) {
		return
	}
	cfg := h.loadSecurityAlertConfig()
	for name, on := range req.Events {
		if !contains(securityAlertEvents, name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown alert event: " + name})
			return
		}
		cfg.Events[name] = on
	}
	if req.Channels != nil {
		for _, ch := range *req.Channels {
			if !contains(alertChannelKeys, ch) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "unknown channel: " + ch})
				return
			}
		}
		cfg.Channels = *req.Channels
	}
	if req.Email != nil {
		cfg.Email = strings.TrimSpace(*req.Email)
	}
	b, _ := json.Marshal(cfg)
	if err := h.db.Save(&models.Setting{Key: settingSecurityAlerts, Value: string(b), UpdatedAt: time.Now()}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update security alerts"})
		return
	}
	h.recordCtx(c, "owner.security_alerts_update", "settings", "security_alerts", nil)
	c.JSON(http.StatusOK, h.securityAlertsResponse(cfg))
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

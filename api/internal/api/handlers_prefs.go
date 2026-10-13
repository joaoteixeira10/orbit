package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// prefsResponse exposes prefs including the parsed notify channel list.
type prefsResponse struct {
	Timezone       string       `json:"timezone"`
	Theme          models.Theme `json:"theme"`
	AccentColor    string       `json:"accent_color"`
	NotifyEvents   bool         `json:"notify_events"`
	NotifyChannels []string     `json:"notify_channels"`
	WeekStart      int          `json:"week_start"`
	TimeFormat     string       `json:"time_format"`
}

func toPrefsResponse(p models.UserPrefs) prefsResponse {
	chans := []string{}
	if p.NotifyChannels != "" {
		_ = json.Unmarshal([]byte(p.NotifyChannels), &chans)
	}
	return prefsResponse{
		Timezone:       p.Timezone,
		Theme:          p.Theme,
		AccentColor:    p.AccentColor,
		NotifyEvents:   p.NotifyEvents,
		NotifyChannels: chans,
		WeekStart:      p.WeekStart,
		TimeFormat:     orDefault(p.TimeFormat, "24h"),
	}
}

// getPrefs returns the authenticated user's UI preferences.
//
//	@Summary	Get my preferences
//	@Tags		prefs
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	prefsResponse
//	@Router		/me/prefs [get]
func (h *Handlers) getPrefs(c *gin.Context) {
	c.JSON(http.StatusOK, toPrefsResponse(h.loadOrInitPrefs(auth.UserID(c))))
}

// updatePrefs patches the authenticated user's UI preferences.
//
//	@Summary	Update my preferences
//	@Tags		prefs
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		updatePrefsRequest	true	"Preferences to update"
//	@Success	200		{object}	prefsResponse
//	@Router		/me/prefs [put]
func (h *Handlers) updatePrefs(c *gin.Context) {
	var req updatePrefsRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.AccentColor != nil && !validHexColor(*req.AccentColor) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accent_color must be a #rrggbb hex colour (or empty)"})
		return
	}
	prefs := h.loadOrInitPrefs(auth.UserID(c))

	updates := map[string]any{}
	if req.Timezone != nil {
		if *req.Timezone != "local" {
			if _, err := time.LoadLocation(*req.Timezone); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "timezone must be \"local\" or an IANA zone, like Europe/Lisbon"})
				return
			}
		}
		updates["timezone"] = *req.Timezone
	}
	if req.AccentColor != nil {
		updates["accent_color"] = *req.AccentColor
	}
	if req.Theme != nil {
		updates["theme"] = *req.Theme
	}
	if req.DeviceTZ != nil {
		if _, err := time.LoadLocation(*req.DeviceTZ); err == nil {
			updates["device_tz"] = *req.DeviceTZ
		}
	}
	if req.WeekStart != nil {
		updates["week_start"] = *req.WeekStart
	}
	if req.TimeFormat != nil {
		updates["time_format"] = *req.TimeFormat
	}
	if req.NotifyEvents != nil {
		updates["notify_events"] = *req.NotifyEvents
	}
	if req.NotifyChannels != nil {
		b, _ := json.Marshal(*req.NotifyChannels)
		updates["notify_channels"] = string(b)
	}
	if len(updates) > 0 {
		if err := h.db.Model(&prefs).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update preferences"})
			return
		}
	}
	h.db.Where("user_id = ?", auth.UserID(c)).First(&prefs)
	c.JSON(http.StatusOK, toPrefsResponse(prefs))
}

// loadOrInitPrefs returns the user's prefs, creating defaults if absent.
func (h *Handlers) loadOrInitPrefs(userID string) models.UserPrefs {
	var prefs models.UserPrefs
	err := h.db.Where("user_id = ?", userID).First(&prefs).Error
	if err != nil {
		prefs = models.UserPrefs{
			UserID:     userID,
			Timezone:   "local",
			Theme:      models.ThemeSystem,
			WeekStart:  1,
			TimeFormat: "24h",
		}
		h.db.Create(&prefs)
	}
	return prefs
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

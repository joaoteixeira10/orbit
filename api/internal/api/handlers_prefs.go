package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/inbox"
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
	ChannelDetails bool         `json:"channel_details"` // titles in shared channels too
	WeekStart      int          `json:"week_start"`
	TimeFormat     string       `json:"time_format"`
	// NotifyPrefs is where each kind of notification goes besides the inbox.
	NotifyPrefs   map[string]inbox.Delivery `json:"notify_prefs"`
	DigestEnabled bool                      `json:"digest_enabled"`
	DigestTime    string                    `json:"digest_time"`
	Locale        string                    `json:"locale"`
	PageEditor    string                    `json:"page_editor"`   // visual | markdown
	DevTheme      string                    `json:"dev_theme"`     // "" (as the instance) | none | a theme id
	RemoteImages  string                    `json:"remote_images"` // ask | always
	SketchColors  []string                  `json:"sketch_colors"` // their own palette in Quick Draw
	EditorThemes  map[string]string         `json:"editor_themes"` // sheets / diagrams / whiteboards: light | dark
	// CalendarColors: own colours for calendars (a workspace id, list:<id>, sub:<id>, holidays).
	CalendarColors map[string]string `json:"calendar_colors"`
	// Quiet hours ("" = none): nothing to devices or channels between them.
	QuietFrom      string `json:"quiet_from"`
	QuietTo        string `json:"quiet_to"`
	QuietReminders bool   `json:"quiet_reminders"`
	Quiet          bool   `json:"quiet"` // it's quiet now
}

// editorKinds are the editors that may keep their own theme.
var editorKinds = map[string]bool{"sheets": true, "diagrams": true, "whiteboards": true}

func toPrefsResponse(p models.UserPrefs) prefsResponse {
	chans := []string{}
	if p.NotifyChannels != "" {
		_ = json.Unmarshal([]byte(p.NotifyChannels), &chans)
	}
	colors := []string{}
	if p.SketchColors != "" {
		_ = json.Unmarshal([]byte(p.SketchColors), &colors)
	}
	editors := map[string]string{}
	if p.EditorThemes != "" {
		_ = json.Unmarshal([]byte(p.EditorThemes), &editors)
	}
	calColors := map[string]string{}
	if p.CalendarColors != "" {
		_ = json.Unmarshal([]byte(p.CalendarColors), &calColors)
	}
	return prefsResponse{
		CalendarColors: calColors,
		EditorThemes:   editors,
		Timezone:       p.Timezone,
		Theme:          p.Theme,
		AccentColor:    p.AccentColor,
		NotifyEvents:   p.NotifyEvents,
		NotifyChannels: chans,
		ChannelDetails: p.ChannelDetails,
		WeekStart:      p.WeekStart,
		TimeFormat:     orDefault(p.TimeFormat, "24h"),
		NotifyPrefs:    inbox.PrefsOf(p),
		DigestEnabled:  p.DigestEnabled,
		DigestTime:     orDefault(p.DigestTime, "08:00"),
		Locale:         p.Locale,
		PageEditor:     orDefault(p.PageEditor, "visual"),
		DevTheme:       p.DevTheme,
		RemoteImages:   orDefault(p.RemoteImages, "ask"),
		SketchColors:   colors,
		QuietFrom:      p.QuietFrom,
		QuietTo:        p.QuietTo,
		QuietReminders: p.QuietReminders,
		Quiet:          inbox.Quiet(p, time.Now()),
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
	if req.ChannelDetails != nil {
		updates["channel_details"] = *req.ChannelDetails
	}
	if req.NotifyPrefs != nil {
		merged := inbox.PrefsOf(prefs)
		for k, d := range *req.NotifyPrefs {
			merged[k] = d
		}
		updates["notify_prefs"] = inbox.EncodePrefs(merged)
		// The older switch is "reminders also go to my channels".
		updates["notify_events"] = merged[inbox.Reminder].Channels
	}
	if req.DigestEnabled != nil {
		updates["digest_enabled"] = *req.DigestEnabled
	}
	if req.DigestTime != nil {
		if _, err := time.Parse("15:04", *req.DigestTime); err != nil || len(*req.DigestTime) != 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "digest_time must be HH:MM"})
			return
		}
		updates["digest_time"] = *req.DigestTime
		// A new time today may still send today's summary.
		updates["digest_sent"] = ""
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.PageEditor != nil {
		updates["page_editor"] = *req.PageEditor
	}
	if req.SketchColors != nil {
		for _, col := range *req.SketchColors {
			if col == "" || !validHexColor(col) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "sketch_colors must be #rrggbb hex colours"})
				return
			}
		}
		b, _ := json.Marshal(*req.SketchColors)
		updates["sketch_colors"] = string(b)
	}
	if req.RemoteImages != nil {
		updates["remote_images"] = *req.RemoteImages
	}
	if req.EditorThemes != nil {
		editors := map[string]string{}
		if prefs.EditorThemes != "" {
			_ = json.Unmarshal([]byte(prefs.EditorThemes), &editors)
		}
		for k, v := range *req.EditorThemes {
			if !editorKinds[k] || (v != "light" && v != "dark" && v != "follow") {
				c.JSON(http.StatusBadRequest, gin.H{"error": "editor_themes: sheets, diagrams or whiteboards, each light, dark or follow"})
				return
			}
			if v == "follow" {
				delete(editors, k)
			} else {
				editors[k] = v
			}
		}
		b, _ := json.Marshal(editors)
		updates["editor_themes"] = string(b)
	}
	if req.CalendarColors != nil {
		colors := map[string]string{}
		if prefs.CalendarColors != "" {
			_ = json.Unmarshal([]byte(prefs.CalendarColors), &colors)
		}
		for k, v := range *req.CalendarColors {
			if k == "" || len(k) > 64 || (v != "" && !validHexColor(v)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "calendar_colors: a calendar's key, and a #rrggbb colour (or empty)"})
				return
			}
			if v == "" {
				delete(colors, k)
			} else {
				colors[k] = v
			}
		}
		if len(colors) > 500 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "too many calendar colours"})
			return
		}
		b, _ := json.Marshal(colors)
		updates["calendar_colors"] = string(b)
	}
	for _, q := range []struct {
		v   *string
		col string
	}{{req.QuietFrom, "quiet_from"}, {req.QuietTo, "quiet_to"}} {
		if q.v == nil {
			continue
		}
		if *q.v != "" && !inbox.ValidClock(*q.v) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "quiet hours are HH:MM"})
			return
		}
		updates[q.col] = *q.v
	}
	if req.QuietReminders != nil {
		updates["quiet_reminders"] = *req.QuietReminders
	}
	if req.DevTheme != nil {
		if *req.DevTheme != "" && *req.DevTheme != "none" && validDevTheme(*req.DevTheme) != *req.DevTheme {
			c.JSON(http.StatusBadRequest, gin.H{"error": "dev_theme must be empty, none, blue, gold, red, ink or green"})
			return
		}
		updates["dev_theme"] = *req.DevTheme
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

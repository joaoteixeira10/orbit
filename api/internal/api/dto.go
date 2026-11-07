package api

import (
	"github.com/joaoteixeira10/orbit/api/internal/inbox"
	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// ---- Auth ----

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	OTP      string `json:"otp"`
}

type refreshRequest struct {
	// Optional: browsers send the refresh token via the HttpOnly cookie instead.
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	// The refresh token is delivered as an HttpOnly cookie, never in the body.
	ExpiresIn int          `json:"expires_in"` // seconds until access token expiry
	User      *models.User `json:"user,omitempty"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

type updateProfileRequest struct {
	Email       *string `json:"email" binding:"omitempty,email"`
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

type webauthnRenameRequest struct {
	Name string `json:"name" binding:"required"`
}

// ---- Users ----

type createUserRequest struct {
	Username           string   `json:"username" binding:"required"`
	DisplayName        string   `json:"display_name"`
	Email              string   `json:"email"`
	Password           string   `json:"password"` // required unless GeneratePassword
	GeneratePassword   bool     `json:"generate_password"`
	MustChangePassword bool     `json:"must_change_password"`
	OTPRequired        bool     `json:"otp_required"` // two-factor required for this user
	RoleIDs            []string `json:"role_ids"`
}

type updateUserRequest struct {
	DisplayName *string   `json:"display_name"`
	Email       *string   `json:"email"`
	RoleIDs     *[]string `json:"role_ids"`
	Disabled    *bool     `json:"disabled"`
	OTPRequired *bool     `json:"otp_required"`
}

type resetPasswordRequest struct {
	// NewPassword (>= 8 characters), or Generate for a one-time temporary
	// password that is returned once.
	NewPassword string `json:"new_password"`
	Generate    bool   `json:"generate"`
	// MustChangePassword forces the user to replace the password on next
	// sign-in. Defaults to true when omitted (an admin-set password is temporary).
	MustChangePassword *bool `json:"must_change_password"`
}

// ---- Prefs ----

type updatePrefsRequest struct {
	Timezone       *string       `json:"timezone"`
	Theme          *models.Theme `json:"theme" binding:"omitempty,oneof=light dark system"`
	AccentColor    *string       `json:"accent_color"` // #rrggbb or "" (validated in the handler)
	NotifyEvents   *bool         `json:"notify_events"`
	NotifyChannels *[]string     `json:"notify_channels"`
	ChannelDetails *bool         `json:"channel_details"`
	DeviceTZ       *string       `json:"device_tz" binding:"omitempty,max=64"`
	WeekStart      *int          `json:"week_start" binding:"omitempty,oneof=0 1"`
	TimeFormat     *string       `json:"time_format" binding:"omitempty,oneof=24h 12h"`
	// Per kind (reminder, assigned, workspace, activity, digest): also to my
	// devices (push) and/or my channels.
	NotifyPrefs   *map[string]inbox.Delivery `json:"notify_prefs"`
	DigestEnabled *bool                      `json:"digest_enabled"`
	DigestTime    *string                    `json:"digest_time"` // HH:MM
	Locale        *string                    `json:"locale" binding:"omitempty,max=8"`
	PageEditor    *string                    `json:"page_editor" binding:"omitempty,oneof=visual markdown"`
	DevTheme      *string                    `json:"dev_theme" binding:"omitempty,max=10"`
	RemoteImages  *string                    `json:"remote_images" binding:"omitempty,oneof=ask always"`
	SketchColors  *[]string                  `json:"sketch_colors" binding:"omitempty,max=40"`
	// Sheets, diagrams, whiteboards: "light" or "dark" whatever Orbit's
	// theme; "follow" (or leaving one out) follows it.
	EditorThemes *map[string]string `json:"editor_themes"`
	// Own colours for calendars: key → "#rrggbb", or "" to use the
	// calendar's own again.
	CalendarColors *map[string]string `json:"calendar_colors"`
	// Own lengths for the focus timer, in minutes (1–180), besides 15, 25, 50.
	FocusLengths *[]int `json:"focus_lengths" binding:"omitempty,max=8"`
	// Quiet hours: "HH:MM" each ("" both = none); reminders through or not.
	QuietFrom      *string `json:"quiet_from"`
	QuietTo        *string `json:"quiet_to"`
	QuietReminders *bool   `json:"quiet_reminders"`
	// A holiday theme's falling snow, bats, confetti… on or off.
	HolidayEffects *bool `json:"holiday_effects"`
}

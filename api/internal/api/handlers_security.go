package api

import (
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

const (
	settingOTPEnforced    = "otp_enforced"
	settingHideLocalLogin = "hide_local_login"     // hide the password form on /login when SSO is enabled
	settingPasswordless   = "passwordless_enabled" // allow "Sign in with a passkey" (discoverable login)
	settingAutoSSO        = "auto_sso"             // auto-start SSO on the login page (unless ?local=1)
)

// boolSetting / intSetting back the security policy on the global Setting store
// so changes take effect live, without a restart (unlike the .env config).

// boolSetting reads a boolean Setting, returning def when unset.
func (h *Handlers) boolSetting(key string, def bool) bool {
	var row models.Setting
	if err := h.db.Where(&models.Setting{Key: key}).First(&row).Error; err != nil {
		return def
	}
	return row.Value == "true"
}

func (h *Handlers) setBoolSetting(key string, v bool) error {
	val := "false"
	if v {
		val = "true"
	}
	return h.db.Save(&models.Setting{Key: key, Value: val, UpdatedAt: time.Now()}).Error
}

// otpRequiredFor reports whether an admin requires two-factor for this one user.
func (h *Handlers) otpRequiredFor(userID string) bool {
	var u models.User
	return h.db.Select("otp_required").First(&u, "id = ?", userID).Error == nil && u.OTPRequired
}

// otpEnforced reports whether the admin requires TOTP for all (local) users.
func (h *Handlers) otpEnforced() bool { return h.boolSetting(settingOTPEnforced, false) }

// hideLocalLogin reports whether the password form should be hidden on /login
// (only meaningful when SSO is enabled; the escape hatch
// /login?local=1 always shows it, and the /auth/login endpoint always works).
func (h *Handlers) hideLocalLogin() bool { return h.boolSetting(settingHideLocalLogin, false) }

// passwordlessEnabled reports whether passwordless passkey login is allowed.
// Defaults to true to preserve prior behaviour.
func (h *Handlers) passwordlessEnabled() bool { return h.boolSetting(settingPasswordless, true) }

// autoSSO reports whether the login page should auto-start SSO (unless ?local=1).
func (h *Handlers) autoSSO() bool { return h.boolSetting(settingAutoSSO, false) }

type securityConfigRequest struct {
	OTPEnforced         *bool `json:"otp_enforced"`
	HideLocalLogin      *bool `json:"hide_local_login"`
	PasswordlessEnabled *bool `json:"passwordless_enabled"`
	AutoSSO             *bool `json:"auto_sso"`
}

func (h *Handlers) securityConfig() gin.H {
	return gin.H{
		"otp_enforced":         h.otpEnforced(),
		"hide_local_login":     h.hideLocalLogin(),
		"passwordless_enabled": h.passwordlessEnabled(),
		"auto_sso":             h.autoSSO(),
	}
}

// getSecurityConfig returns the security/login policy (admin).
//
//	@Summary	Get security policy
//	@Tags		security
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]bool
//	@Router		/security/config [get]
func (h *Handlers) getSecurityConfig(c *gin.Context) {
	c.JSON(http.StatusOK, h.securityConfig())
}

// updateSecurityConfig sets the security/login policy (admin). Only the provided
// fields are changed.
//
//	@Summary	Update security policy
//	@Tags		security
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		securityConfigRequest	true	"Policy"
//	@Success	200		{object}	map[string]bool
//	@Router		/security/config [put]
func (h *Handlers) updateSecurityConfig(c *gin.Context) {
	var req securityConfigRequest
	if !bindJSON(c, &req) {
		return
	}
	// Auto-start SSO only makes sense while SSO is enabled.
	resAutoSSO := h.autoSSO()
	if req.AutoSSO != nil {
		resAutoSSO = *req.AutoSSO
	}
	if resAutoSSO && (h.oidc == nil || !h.oidc.Enabled()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enable SSO before auto-starting it"})
		return
	}
	apply := func(key string, v *bool) bool {
		if v == nil {
			return true
		}
		return h.setBoolSetting(key, *v) == nil
	}
	if !apply(settingOTPEnforced, req.OTPEnforced) ||
		!apply(settingHideLocalLogin, req.HideLocalLogin) ||
		!apply(settingPasswordless, req.PasswordlessEnabled) ||
		!apply(settingAutoSSO, req.AutoSSO) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save policy"})
		return
	}
	h.recordCtx(c, "security.config_update", "settings", "security", nil)
	h.securityAlert(c, "config_change", "Security policy changed")
	c.JSON(http.StatusOK, h.securityConfig())
}

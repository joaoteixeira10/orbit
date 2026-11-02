package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/metrics"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// login authenticates a user and returns access + refresh tokens.
//
//	@Summary	Log in
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		loginRequest	true	"Credentials"
//	@Success	200		{object}	tokenResponse
//	@Failure	401		{object}	map[string]string
//	@Router		/auth/login [post]
func (h *Handlers) login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	now := time.Now()
	user, err := h.auth.Authenticate(req.Username, req.Password)
	if err != nil {
		metrics.Login("password", "failure")
		if errors.Is(err, auth.ErrAccountLocked) && user != nil {
			h.audit.Record(user.ID, "auth.lockout", "user", user.ID, c.ClientIP(), gin.H{"reason": "too many failed login attempts"})
		} else {
			// Every failed sign-in is audited (the name tried, never the password);
			// the per-IP throttle bounds how many a client can cause.
			h.audit.Record("", "auth.login_failed", "", "", c.ClientIP(), gin.H{"username": truncate(req.Username, 64)})
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	// Second factor: TOTP and/or passkey. A code supplied in the request
	// completes TOTP; otherwise, if any second factor exists, return a challenge
	// listing what's available so the SPA can offer the choice. The passkey
	// challenge is finished via /auth/webauthn/login/finish.
	otpAvailable := user.OTPEnabled
	passkeyAvailable := h.webauthn.Enabled() && auth.UserHasPasskey(h.db, user.ID)
	if req.OTP != "" {
		secret, ok := h.unsealOTP(*user)
		valid := otpAvailable && ok && auth.ValidateTOTP(secret, req.OTP)
		if !valid {
			valid = h.consumeBackupCode(user, req.OTP) // accept a recovery code
		}
		if !valid {
			metrics.Login("password", "failure")
			if h.auth.RecordFailure(user) { // a wrong 2nd factor counts toward lockout
				h.audit.Record(user.ID, "auth.lockout", "user", user.ID, c.ClientIP(), gin.H{"reason": "too many failed 2FA attempts"})
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authentication code", "otp_required": true})
			return
		}
	} else if otpAvailable || passkeyAvailable {
		resp := gin.H{}
		if otpAvailable {
			resp["otp_required"] = true
		}
		if passkeyAvailable {
			if opts, sid, err := h.webauthn.BeginLogin(user); err == nil {
				resp["webauthn_required"] = true
				resp["webauthn"] = gin.H{"session_id": sid, "options": opts}
			}
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	resp, err := h.issueTokens(c, user, now, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	h.auth.RecordLoginSuccess(user) // clear the failed-attempt counter / any lock
	metrics.Login("password", "success")
	h.audit.Record(user.ID, "auth.login", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, resp)
}

// refresh exchanges a refresh token for a new access + refresh token pair.
//
//	@Summary	Refresh access token
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		refreshRequest	true	"Refresh token"
//	@Success	200		{object}	tokenResponse
//	@Failure	401		{object}	map[string]string
//	@Router		/auth/refresh [post]
func (h *Handlers) refresh(c *gin.Context) {
	var req refreshRequest
	_ = c.ShouldBindJSON(&req) // body optional — browsers use the HttpOnly cookie
	token := refreshTokenFromRequest(c, req.RefreshToken)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}
	// The refresh token is an HttpOnly cookie (an ambient credential), so require
	// the double-submit CSRF token on every refresh.
	if !csrfOK(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid CSRF token"})
		return
	}
	now := time.Now()
	user, newRefresh, sid, err := h.auth.Refresh(token, c.Request.UserAgent(), c.ClientIP(), now)
	if err != nil {
		if errors.Is(err, auth.ErrRefreshReuse) && user != nil && user.ID != "" {
			// A rotated refresh token was replayed: someone else holds a copy.
			// The session is already revoked; record it and alert the owner.
			h.log.Warn("refresh token reuse detected; session revoked",
				zap.String("user_id", user.ID), zap.String("ip", c.ClientIP()))
			h.audit.Record(user.ID, "auth.refresh_reuse", "user", user.ID, c.ClientIP(),
				gin.H{"reason": "rotated refresh token replayed; session revoked"})
			h.raiseSecurityAlert("refresh_reuse", "Refresh-token reuse detected for "+user.Username+"; session revoked", user.Username, c.ClientIP())
		}
		h.clearAuthCookies(c)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	access, exp, err := h.auth.IssueAccessToken(user, sid, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	h.setAuthCookies(c, newRefresh) // rotate the cookie
	c.JSON(http.StatusOK, tokenResponse{
		AccessToken: access,
		ExpiresIn:   int(time.Until(exp).Seconds()),
	})
}

// logout revokes the supplied refresh token.
//
//	@Summary	Log out
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	refreshRequest	true	"Refresh token to revoke"
//	@Success	204
//	@Router		/auth/logout [post]
func (h *Handlers) logout(c *gin.Context) {
	var req refreshRequest
	_ = c.ShouldBindJSON(&req) // body optional — browsers use the HttpOnly cookie
	token := refreshTokenFromRequest(c, req.RefreshToken)
	if !csrfOK(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid CSRF token"})
		return
	}
	if token != "" {
		_ = h.auth.RevokeRefreshToken(token)
	}
	h.clearAuthCookies(c)
	c.Status(http.StatusNoContent)
}

// me returns the authenticated user (with prefs).
//
//	@Summary	Current user
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	models.User
//	@Router		/auth/me [get]
func (h *Handlers) me(c *gin.Context) {
	var user models.User
	if err := h.db.Preload("Prefs").Preload("Roles").First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	hasPasskey := auth.UserHasPasskey(h.db, user.ID)
	resp := gin.H{
		"user":                  user,
		"privileges":            privilegesPayload(auth.Effective(c)),
		"must_setup_otp":        h.auth.NeedsOTPSetup(&user),
		"otp_backup_remaining":  backupRemaining(user),
		"has_passkey":           hasPasskey,
		"webauthn_enabled":      h.webauthn.Enabled(),
		"is_owner":              user.Owner,
		"features":              feature.All(h.db),
		"can_create_workspaces": canCreateWorkspaces(auth.Effective(c)),
	}
	// When acting through an impersonation token, tell the SPA who the real
	// admin is so it can show the "return to <admin>" banner.
	if impID := auth.ImpersonatorID(c); impID != "" {
		var imp models.User
		if h.db.Select("id", "username").First(&imp, "id = ?", impID).Error == nil {
			resp["impersonator"] = gin.H{"id": imp.ID, "username": imp.Username}
		}
	}
	c.JSON(http.StatusOK, resp)
}

// privilegesPayload serializes an effective privilege set for the SPA to gate
// UI (display only; the server is authoritative).
func privilegesPayload(e rbac.Effective) gin.H {
	return gin.H{
		"superuser": e.Superuser,
		"global":    mapKeys(e.Global),
		"read_only": e.ReadOnly,
	}
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}

// changePassword updates the authenticated user's own password and signs out
// every other session: the token version is bumped (rejecting any other live
// access token at once) and the other sessions' refresh tokens are revoked. The
// caller's own session survives and gets a fresh access token in the response.
//
//	@Summary	Change own password
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		changePasswordRequest	true	"Old and new password"
//	@Success	200		{object}	tokenResponse
//	@Failure	400		{object}	map[string]string
//	@Router		/me/password [put]
func (h *Handlers) changePassword(c *gin.Context) {
	var req changePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if user.OIDCSubject != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is managed by your identity provider"})
		return
	}
	if !h.recheckPassword(c, &user, req.OldPassword, http.StatusBadRequest, "current password is incorrect") {
		return
	}
	// A forced change (the first admin's password from .env, one an admin
	// chose) exists because someone else knows the current password: the same
	// one again would change nothing. Only then; no history is kept.
	if user.MustChangePassword && req.NewPassword == req.OldPassword {
		c.JSON(http.StatusBadRequest, gin.H{"error": "choose a new password, not the current one"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}
	user.TokenVersion++
	if err := h.db.Model(&user).Updates(map[string]any{
		"password_hash":        hash,
		"must_change_password": false,
		"token_version":        user.TokenVersion,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}
	sid := auth.Sid(c)
	_ = h.auth.RevokeOtherSessions(user.ID, sid)
	access, exp, err := h.auth.IssueAccessToken(&user, sid, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	h.audit.Record(user.ID, "auth.password_change", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, tokenResponse{AccessToken: access, ExpiresIn: int(time.Until(exp).Seconds())})
}

// updateProfile lets the authenticated user change their own profile (email).
//
//	@Summary	Update my profile
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		updateProfileRequest	true	"Profile fields"
//	@Success	200		{object}	models.User
//	@Router		/me/profile [put]
func (h *Handlers) updateProfile(c *gin.Context) {
	var req updateProfileRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	updates := map[string]any{}
	// For SSO accounts, email + display name are owned by the identity provider
	// (synced on each login); only the local avatar is editable.
	if req.Email != nil && user.OIDCSubject == "" {
		email := strings.TrimSpace(*req.Email)
		if !strings.EqualFold(email, user.Email) {
			if h.emailTaken(email, user.ID) {
				c.JSON(http.StatusConflict, gin.H{"error": errEmailTaken})
				return
			}
		}
		updates["email"] = email
	}
	if req.DisplayName != nil && user.OIDCSubject == "" {
		updates["display_name"] = *req.DisplayName
	}
	if req.AvatarURL != nil {
		if !validImageDataURL(*req.AvatarURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "avatar must be an image data URL"})
			return
		}
		updates["avatar_url"] = *req.AvatarURL
	}
	if len(updates) > 0 {
		if err := h.db.Model(&user).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update profile"})
			return
		}
	}
	h.audit.Record(user.ID, "auth.profile_update", "user", user.ID, c.ClientIP(), nil)
	h.db.Preload("Prefs").Preload("Roles").First(&user, "id = ?", user.ID)
	c.JSON(http.StatusOK, user)
}

// rotateToken bumps the user's token version (invalidating any access token
// previously issued — e.g. one copied to a shell) and returns a fresh token
// pair for this client. Existing refresh tokens are revoked too.
//
//	@Summary	Rotate access token
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	tokenResponse
//	@Router		/me/rotate-token [post]
func (h *Handlers) rotateToken(c *gin.Context) {
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	user.TokenVersion++
	if err := h.db.Model(&user).Update("token_version", user.TokenVersion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not rotate token"})
		return
	}
	// Revoke existing refresh tokens, then mint a fresh pair for this client.
	h.db.Model(&models.RefreshToken{}).Where("user_id = ?", user.ID).Update("revoked", true)
	resp, err := h.issueTokens(c, &user, time.Now(), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	h.audit.Record(user.ID, "auth.token_rotate", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, resp)
}

// issueTokens starts a new session for a user (refresh cookie) and returns the
// access token for it.
func (h *Handlers) issueTokens(c *gin.Context, user *models.User, now time.Time, includeUser bool) (*tokenResponse, error) {
	sid, err := h.startSession(c, user, now)
	if err != nil {
		return nil, err
	}
	return h.accessResponse(user, sid, now, includeUser)
}

// startSession opens a new login session: it issues the refresh token and sets
// it as the HttpOnly cookie (+ CSRF companion), returning the session id.
func (h *Handlers) startSession(c *gin.Context, user *models.User, now time.Time) (string, error) {
	refresh, sid, err := h.auth.IssueRefreshToken(user.ID, c.Request.UserAgent(), c.ClientIP(), now)
	if err != nil {
		return "", err
	}
	h.setAuthCookies(c, refresh)
	return sid, nil
}

// accessResponse mints the access token for an existing session.
func (h *Handlers) accessResponse(user *models.User, sid string, now time.Time, includeUser bool) (*tokenResponse, error) {
	access, exp, err := h.auth.IssueAccessToken(user, sid, now)
	if err != nil {
		return nil, err
	}
	resp := &tokenResponse{
		AccessToken: access,
		ExpiresIn:   int(time.Until(exp).Seconds()),
	}
	if includeUser {
		// Reload with prefs for the client.
		var full models.User
		if err := h.db.Preload("Prefs").Preload("Roles").First(&full, "id = ?", user.ID).Error; err == nil {
			resp.User = &full
		}
	}
	return resp, nil
}

// OIDC login/callback/exchange live in handlers_oidc.go.

// truncate caps a user-supplied string written to the audit log.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

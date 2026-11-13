package api

import (
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/metrics"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// webauthnEnabled writes a 501 and returns false when passkeys are not
// configured (no RP ID), mirroring the OIDC scaffolding pattern.
func (h *Handlers) webauthnEnabled(c *gin.Context) bool {
	if h.webauthn == nil || !h.webauthn.Enabled() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "passkeys are not enabled on this server"})
		return false
	}
	return true
}

// webauthnRegisterBegin starts enrolling a new passkey for the current user.
//
//	@Summary	Begin passkey registration
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]any
//	@Router		/me/webauthn/register/begin [post]
func (h *Handlers) webauthnRegisterBegin(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	opts, sid, err := h.webauthn.BeginRegistration(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start registration"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": sid, "options": opts})
}

// webauthnRegisterFinish verifies the attestation and stores the passkey. The
// raw attestation JSON is the request body; `sid` and `name` are query params.
//
//	@Summary	Finish passkey registration
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Param		sid		query		string	true	"Session id from begin"
//	@Param		name	query		string	false	"Friendly label"
//	@Success	200		{object}	models.WebAuthnCredential
//	@Router		/me/webauthn/register/finish [post]
func (h *Handlers) webauthnRegisterFinish(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	name := c.Query("name")
	if name == "" {
		name = "Passkey"
	}
	cred, err := h.webauthn.FinishRegistration(&user, name, c.Query("sid"), c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "registration failed: " + err.Error()})
		return
	}
	h.audit.Record(user.ID, "auth.webauthn_register", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, cred)
}

// listWebAuthnCredentials returns the current user's registered passkeys.
//
//	@Summary	List my passkeys
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	models.WebAuthnCredential
//	@Router		/me/webauthn [get]
func (h *Handlers) listWebAuthnCredentials(c *gin.Context) {
	rows := []models.WebAuthnCredential{} // non-nil → [] (not null) when empty
	h.db.Where("user_id = ?", auth.UserID(c)).Order("created_at").Find(&rows)
	c.JSON(http.StatusOK, rows)
}

// renameWebAuthnCredential changes the label of one of the user's passkeys.
//
//	@Summary	Rename a passkey
//	@Tags		auth
//	@Accept		json
//	@Security	BearerAuth
//	@Param		id		path	int						true	"Credential id"
//	@Param		body	body	webauthnRenameRequest	true	"New name"
//	@Success	204
//	@Router		/me/webauthn/{id} [patch]
func (h *Handlers) renameWebAuthnCredential(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var req webauthnRenameRequest
	if !bindJSON(c, &req) {
		return
	}
	res := h.db.Model(&models.WebAuthnCredential{}).
		Where("id = ? AND user_id = ?", id, auth.UserID(c)).
		Update("name", req.Name)
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "passkey not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// deleteWebAuthnCredential removes one of the user's passkeys.
//
//	@Summary	Delete a passkey
//	@Tags		auth
//	@Security	BearerAuth
//	@Param		id	path	int	true	"Credential id"
//	@Success	204
//	@Router		/me/webauthn/{id} [delete]
func (h *Handlers) deleteWebAuthnCredential(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	res := h.db.Where("id = ? AND user_id = ?", id, auth.UserID(c)).Delete(&models.WebAuthnCredential{})
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "passkey not found"})
		return
	}
	h.recordCtx(c, "auth.webauthn_delete", "webauthn", id, nil)
	c.Status(http.StatusNoContent)
}

// webauthnLoginBegin starts a passwordless (discoverable) assertion.
//
//	@Summary	Begin passwordless passkey login
//	@Tags		auth
//	@Produce	json
//	@Success	200	{object}	map[string]any
//	@Router		/auth/webauthn/login/begin [post]
func (h *Handlers) webauthnLoginBegin(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	if !h.passwordlessEnabled() {
		c.JSON(http.StatusForbidden, gin.H{"error": "passwordless sign-in is disabled"})
		return
	}
	opts, sid, err := h.webauthn.BeginDiscoverableLogin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not start login"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": sid, "options": opts})
}

// webauthnLoginFinish completes a passkey assertion (passwordless OR the
// second-factor step) and issues tokens. The raw assertion JSON is the body;
// `sid` is a query param.
//
//	@Summary	Finish passkey login
//	@Tags		auth
//	@Produce	json
//	@Param		sid	query		string	true	"Session id from begin"
//	@Success	200	{object}	tokenResponse
//	@Failure	401	{object}	map[string]string
//	@Router		/auth/webauthn/login/finish [post]
func (h *Handlers) webauthnLoginFinish(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	now := time.Now()
	user, err := h.webauthn.FinishLogin(c.Query("sid"), c.Request)
	if err != nil {
		metrics.Login("passkey", "failure")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "passkey authentication failed"})
		return
	}
	// A lockout holds for every way in, passkeys included.
	if h.auth.IsLocked(user) {
		metrics.Login("passkey", "failure")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "passkey authentication failed"})
		return
	}
	resp, err := h.issueTokens(c, user, now, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	metrics.Login("passkey", "success")
	h.auth.RecordLoginSuccess(user)
	h.audit.Record(user.ID, "auth.webauthn_login", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, resp)
}

// resetUserWebAuthn (admin) removes all of a user's passkeys.
//
//	@Summary	Reset a user's passkeys (admin)
//	@Tags		users
//	@Security	BearerAuth
//	@Param		id	path	int	true	"User id"
//	@Success	204
//	@Router		/users/{id}/reset-webauthn [post]
func (h *Handlers) resetUserWebAuthn(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !h.mayManage(c, user) {
		return
	}
	if err := h.db.Where("user_id = ?", id).Delete(&models.WebAuthnCredential{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset passkeys"})
		return
	}
	h.recordCtx(c, "user.reset_webauthn", "user", id, nil)
	c.Status(http.StatusNoContent)
}

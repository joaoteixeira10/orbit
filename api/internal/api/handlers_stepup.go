package api

import (
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/feature"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// stepUpTTL is how long a re-authentication stays valid for gated actions.
const stepUpTTL = 5 * time.Minute

// requireStepUp gates an action behind fresh re-authentication. It is a no-op
// unless the step_up_auth feature flag is on; when on, the caller's token must
// carry a step-up assertion (sua claim) newer than stepUpTTL. Runs after the
// RequireOwner/RequireGlobal gate on a route.
func (h *Handlers) requireStepUp() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !feature.Enabled(h.db, feature.StepUp) {
			c.Next()
			return
		}
		sua := auth.StepUpAt(c)
		if sua == 0 || time.Since(time.Unix(sua, 0)) > stepUpTTL {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "step_up_required"})
			return
		}
		c.Next()
	}
}

type stepUpRequest struct {
	Password string `json:"password"`
}

// stepUp re-authenticates the current user with their password and returns a
// fresh access token carrying a step-up assertion. SSO/password-less accounts
// must use the passkey path instead.
//
//	@Summary	Step-up re-authentication (password)
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	200	{object}	tokenResponse
//	@Router		/auth/step-up [post]
func (h *Handlers) stepUp(c *gin.Context) {
	var req stepUpRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account no longer exists"})
		return
	}
	if user.PasswordHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this account has no password; use a passkey to confirm"})
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}
	h.issueStepUp(c, &user)
}

// stepUpWebAuthnBegin starts a passkey assertion for the current user to
// confirm a step-up action.
//
//	@Summary	Step-up re-authentication (passkey, begin)
//	@Tags		auth
//	@Security	BearerAuth
//	@Router		/auth/step-up/webauthn/begin [post]
func (h *Handlers) stepUpWebAuthnBegin(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account no longer exists"})
		return
	}
	opts, sid, err := h.webauthn.BeginLogin(&user)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no passkey registered"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": sid, "options": opts})
}

// stepUpWebAuthnFinish verifies the passkey assertion and, when it belongs to
// the current user, returns a fresh access token carrying a step-up assertion.
//
//	@Summary	Step-up re-authentication (passkey, finish)
//	@Tags		auth
//	@Security	BearerAuth
//	@Success	200	{object}	tokenResponse
//	@Router		/auth/step-up/webauthn/finish [post]
func (h *Handlers) stepUpWebAuthnFinish(c *gin.Context) {
	if !h.webauthnEnabled(c) {
		return
	}
	asserted, err := h.webauthn.FinishLogin(c.Query("sid"), c.Request)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "passkey authentication failed"})
		return
	}
	// The asserted credential must belong to the logged-in user — never let one
	// account's passkey step up a different session.
	if asserted.ID != auth.UserID(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "passkey does not match the signed-in user"})
		return
	}
	h.issueStepUp(c, asserted)
}

// issueStepUp mints and returns the step-up access token for the caller's
// current session and audits the re-authentication.
func (h *Handlers) issueStepUp(c *gin.Context, user *models.User) {
	tok, exp, err := h.auth.IssueStepUpToken(user, auth.Sid(c), time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not complete step-up"})
		return
	}
	h.recordCtx(c, "auth.step_up", "user", user.ID, nil)
	c.JSON(http.StatusOK, tokenResponse{AccessToken: tok, ExpiresIn: int(time.Until(exp).Seconds())})
}

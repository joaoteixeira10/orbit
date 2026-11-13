package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Context keys for the authenticated principal.
const (
	ctxUserID = "auth.user_id"
	ctxEff    = "auth.effective"
	ctxOwner  = "auth.owner"
	ctxSid    = "auth.sid"
	ctxImp    = "auth.impersonator"
	ctxStepUp = "auth.step_up_at"
	ctxViaKey = "auth.via_bearer_key" // the request came with an API token
)

// ViaAPIToken reports whether the request is authenticated by an API token
// (not a signed-in session).
func ViaAPIToken(c *gin.Context) bool {
	v, _ := c.Get(ctxViaKey)
	b, _ := v.(bool)
	return b
}

// Required wraps a handler chain, rejecting requests without a valid token,
// loading the user's roles, and resolving the effective privilege set.
func (m *Manager) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := bearer(c)
		if raw == "" {
			abort(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		// Long-lived API tokens take a separate (non-interactive) path.
		if strings.HasPrefix(raw, APITokenPrefix) {
			m.requireAPIToken(c, raw)
			return
		}
		claims, err := m.ParseAccessToken(raw)
		if err != nil {
			abort(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		uid := claims.Subject
		if uid == "" {
			abort(c, http.StatusUnauthorized, "invalid token subject")
			return
		}

		// Load the user with roles+grants. Rejects disabled/deleted accounts
		// immediately (a still-valid token stops working the moment the account
		// is disabled), and resolves the effective privilege set.
		var u models.User
		if err := m.db.Preload("Roles").First(&u, "id = ?", uid).Error; err != nil {
			abort(c, http.StatusUnauthorized, "account no longer exists")
			return
		}
		if u.Disabled {
			abort(c, http.StatusUnauthorized, "account disabled")
			return
		}
		// Reject tokens issued before the user's last token rotation.
		if claims.Version != u.TokenVersion {
			abort(c, http.StatusUnauthorized, "token has been rotated")
			return
		}
		// An ended session (signed out, revoked in Active sessions, reuse
		// detected) stops at once, not when its access token expires.
		if claims.Sid != "" && !m.sessionActive(claims.Sid) {
			abort(c, http.StatusUnauthorized, "session ended")
			return
		}
		// A user that must change their password is restricted to doing exactly
		// that (plus reading self / logging out) until it's done.
		if u.MustChangePassword && !passwordChangeAllowed(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "password_change_required"})
			return
		}
		// When TOTP is enforced, a local user without it set up is restricted to
		// enrolling (plus reading self / logging out). A registered passkey is
		// phishing-resistant MFA, so it also exempts the user. SSO accounts are
		// exempt entirely: their login path never checks a local factor, so MFA
		// is the identity provider's responsibility.
		if m.NeedsOTPSetup(&u) && !otpSetupAllowed(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "otp_setup_required"})
			return
		}

		c.Set(ctxUserID, uid)
		c.Set(ctxEff, rbac.Build(append(u.Roles, GroupRoles(m.db, uid)...)))
		c.Set(ctxOwner, u.Owner)
		c.Set(ctxSid, claims.Sid)
		c.Set(ctxImp, claims.Imp)
		c.Set(ctxStepUp, claims.StepUpAt)
		c.Next()
	}
}

// RequireGlobal ensures the principal holds a global privilege. Must run after
// Required().
func (m *Manager) RequireGlobal(priv string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !Effective(c).HasGlobal(priv) {
			abort(c, http.StatusForbidden, "insufficient privileges")
			return
		}
		c.Next()
	}
}

// RequireOwnerOrGlobal gates routes for an owner-only privilege: the instance
// owner always passes; otherwise the principal must hold the privilege explicitly
// (the superuser flag does NOT grant owner-only privileges — see rbac.HasGlobal).
// Used for branding / notifications / impersonation.
func (m *Manager) RequireOwnerOrGlobal(priv string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsOwner(c) || Effective(c).HasGlobal(priv) {
			c.Next()
			return
		}
		abort(c, http.StatusForbidden, "owner only")
	}
}

// RequireOwner gates owner-only ("god mode") routes. Must run after Required().
// Ownership is an identity, not a privilege, so it can't be self-granted.
func (m *Manager) RequireOwner() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsOwner(c) {
			abort(c, http.StatusForbidden, "owner only")
			return
		}
		c.Next()
	}
}

// IsOwner reports whether the authenticated principal is the instance owner.
func IsOwner(c *gin.Context) bool {
	v, _ := c.Get(ctxOwner)
	b, _ := v.(bool)
	return b
}

// Sid returns the caller's session id (RefreshToken.SessionID) from the access
// token, or "" for tokens issued before sessions existed.
func Sid(c *gin.Context) string {
	v, _ := c.Get(ctxSid)
	s, _ := v.(string)
	return s
}

// ImpersonatorID returns the real admin's id when the caller is acting through
// an impersonation token, or "" for an ordinary session.
func ImpersonatorID(c *gin.Context) string {
	v, _ := c.Get(ctxImp)
	id, _ := v.(string)
	return id
}

// StepUpAt returns the unix time (seconds) the caller last re-authenticated for
// a step-up action, or 0 if the token carries no step-up assertion.
func StepUpAt(c *gin.Context) int64 {
	v, _ := c.Get(ctxStepUp)
	t, _ := v.(int64)
	return t
}

// UserID returns the authenticated user's ID (UUID) from the context.
func UserID(c *gin.Context) string {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(string)
	return id
}

// Effective returns the resolved privilege set from the context.
func Effective(c *gin.Context) rbac.Effective {
	v, _ := c.Get(ctxEff)
	e, ok := v.(rbac.Effective)
	if !ok {
		return rbac.Effective{}
	}
	return e
}

// passwordChangeAllowed lists the endpoints a must-change-password user may
// still call: read self, change password, and log out.
func passwordChangeAllowed(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/me/password", "/api/v1/auth/logout":
		return true
	}
	return false
}

// otpSetupAllowed lists the endpoints a user pending OTP enrollment may call.
// The passkey-registration routes are included so an enforced user with neither
// OTP nor a passkey can still enroll a passkey to satisfy the exemption.
func otpSetupAllowed(path string) bool {
	switch path {
	case "/api/v1/auth/me", "/api/v1/auth/logout", "/api/v1/me/otp/setup", "/api/v1/me/otp/verify",
		"/api/v1/me/webauthn/register/begin", "/api/v1/me/webauthn/register/finish":
		return true
	}
	return false
}

// otpEnforced reports whether the admin requires TOTP for all users.
func (m *Manager) otpEnforced() bool {
	var s models.Setting
	if err := m.db.Where(&models.Setting{Key: "otp_enforced"}).First(&s).Error; err != nil {
		return false
	}
	return s.Value == "true"
}

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// sessionActive reports whether a session still has an unrevoked, unexpired
// refresh token (rotation keeps the SessionID; revocation marks them all).
func (m *Manager) sessionActive(sid string) bool {
	var n int64
	m.db.Model(&models.RefreshToken{}).Where("session_id = ? AND revoked = ? AND expires_at > ?", sid, false, time.Now()).Count(&n)
	return n > 0
}

// GroupRoles are the instance roles someone holds through their groups.
func GroupRoles(db *gorm.DB, userID string) []models.Role {
	var roles []models.Role
	db.Where("id IN (?)", db.Model(&models.GroupRole{}).Select("role_id").
		Where("group_id IN (?)", db.Model(&models.GroupMember{}).Select("group_id").Where("user_id = ?", userID))).Find(&roles)
	return roles
}

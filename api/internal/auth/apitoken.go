package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/rbac"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// APITokenPrefix marks a bearer as a long-lived API token (vs a JWT).
const APITokenPrefix = "orb_"

// NewAPITokenSecret mints a token: the plaintext (shown once), its sha256 hash
// (stored), and a non-secret display prefix.
func NewAPITokenSecret() (plaintext, hash, prefix string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	plaintext = APITokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	hash = hashToken(plaintext)
	if len(plaintext) >= 12 {
		prefix = plaintext[:12] // "orb_" + first 8 chars
	} else {
		prefix = plaintext
	}
	return
}

// authenticateAPIToken validates an API token bearer and returns its user.
func (m *Manager) authenticateAPIToken(raw string) (*models.User, *models.APIToken, error) {
	var t models.APIToken
	if err := m.db.Where("token_hash = ?", hashToken(raw)).First(&t).Error; err != nil {
		return nil, nil, ErrInvalidToken
	}
	if t.Revoked || (t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt)) {
		return nil, nil, ErrInvalidToken
	}
	var u models.User
	if err := m.db.Preload("Roles").First(&u, "id = ?", t.UserID).Error; err != nil {
		return nil, nil, ErrInvalidToken
	}
	// Best-effort, throttled last-used update.
	now := time.Now()
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) > time.Minute {
		m.db.Model(&t).Update("last_used_at", &now)
	}
	return &u, &t, nil
}

// requireAPIToken authorizes a request bearing an API token. Unlike the
// interactive JWT path, it skips the password-change / OTP-setup gates (those
// are for human logins); a disabled owner is still rejected.
func (m *Manager) requireAPIToken(c *gin.Context, raw string) {
	u, t, err := m.authenticateAPIToken(raw)
	if err != nil {
		abort(c, 401, "invalid or expired API token")
		return
	}
	if u.Disabled {
		abort(c, 401, "account disabled")
		return
	}
	// A read-only token reads, everywhere: any other method is refused here,
	// before a handler (workspace content checks it too, admin routes didn't).
	// MCP's POST is only an envelope: each of its tools calls the API with the
	// same token, and those calls are checked here.
	if t.ReadOnly && c.Request.URL.Path != "/mcp" {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			abort(c, 403, "this API token is read-only")
			return
		}
	}
	c.Set(ctxUserID, u.ID)
	c.Set(ctxViaKey, true)
	// The token's own scope narrows the owner's rights; it never widens them.
	c.Set(ctxEff, rbac.Build(append(u.Roles, GroupRoles(m.db, u.ID)...)).Narrow(rbac.DecodeStrings(t.ScopePrivileges), t.ReadOnly))
	c.Next()
}

// SweepExpiredAPITokens deletes expired tokens (called by the scheduler).
func SweepExpiredAPITokens(db *gorm.DB) (int64, error) {
	res := db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).Delete(&models.APIToken{})
	return res.RowsAffected, res.Error
}

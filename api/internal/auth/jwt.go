package auth

import (
	"errors"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

// AccessTokenTTL is the lifetime of a JWT access token. Kept short; clients
// refresh using the longer-lived, revocable refresh token.
const AccessTokenTTL = 15 * time.Minute

// Claims is the JWT payload for an access token. Authorization is resolved from
// the user's roles in the DB per request, so no role/privilege data is embedded.
// Version pins the user's TokenVersion so a rotate invalidates older tokens.
// Sid is the stable session id (RefreshToken.SessionID) this token was issued
// for, so the active-sessions UI can identify the caller's own session.
type Claims struct {
	Version uint   `json:"tv"`
	Sid     string `json:"sid,omitempty"`
	// Imp, when non-zero, marks this as an impersonation token: the Subject is
	// the impersonated user and Imp is the real admin's id (for the UI banner
	// and audit). Impersonation tokens are short-lived and have no refresh
	// token, so the session is naturally time-boxed to AccessTokenTTL.
	Imp string `json:"imp,omitempty"`
	// StepUpAt is the unix time (seconds) the user last re-authenticated for a
	// step-up-gated action. Zero on ordinary tokens; freshness is checked by
	// the step-up middleware.
	StepUpAt int64 `json:"sua,omitempty"`
	jwt.RegisteredClaims
}

// ErrInvalidToken is returned when a token fails validation.
var ErrInvalidToken = errors.New("invalid or expired token")

// IssueAccessToken signs a short-lived access token for the user. sid is the
// session id it belongs to (may be empty for tokens not tied to a session).
func (m *Manager) IssueAccessToken(u *models.User, sid string, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		Version: u.TokenVersion,
		Sid:     sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.jwtSecret)
	return signed, exp, err
}

// IssueStepUpToken signs a fresh access token for the user's session stamped
// with the current time as StepUpAt, so step-up-gated actions accept it within
// the step-up window. Same session (sid) and token version as the live token.
func (m *Manager) IssueStepUpToken(u *models.User, sid string, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		Version:  u.TokenVersion,
		Sid:      sid,
		StepUpAt: now.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.jwtSecret)
	return signed, exp, err
}

// IssueImpersonationToken signs a short-lived access token that authenticates
// as target but records impersonatorID in the imp claim. No refresh token is
// issued, so impersonation ends on its own within AccessTokenTTL.
func (m *Manager) IssueImpersonationToken(target *models.User, impersonatorID string, now time.Time) (string, time.Time, error) {
	exp := now.Add(AccessTokenTTL)
	claims := Claims{
		Version: target.TokenVersion,
		Imp:     impersonatorID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   target.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(m.jwtSecret)
	return signed, exp, err
}

// ParseAccessToken validates a token string and returns its claims.
func (m *Manager) ParseAccessToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.jwtSecret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

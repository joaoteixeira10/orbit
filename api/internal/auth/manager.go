package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

// RefreshTokenTTL is the lifetime of a refresh token.
const RefreshTokenTTL = 30 * 24 * time.Hour

// RefreshReuseGrace is how long a just-rotated refresh token may still be
// presented. It absorbs benign races (two tabs refreshing with the same cookie,
// a retried request) without breaking the session. Beyond it, presenting a
// rotated token is treated as token theft and revokes the whole session.
const RefreshReuseGrace = 30 * time.Second

var (
	// ErrInvalidCredentials is returned on a failed username/password login.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserDisabled is returned when a disabled account attempts to log in.
	ErrUserDisabled = errors.New("account disabled")
	// ErrInvalidRefresh is returned for an unknown/expired/revoked refresh token.
	ErrInvalidRefresh = errors.New("invalid refresh token")
	// ErrRefreshReuse is returned when an already-rotated refresh token is
	// replayed after the grace window: its whole session has been revoked. The
	// caller audits it; the client just sees an invalid refresh token.
	ErrRefreshReuse = errors.New("refresh token reuse detected")
	// ErrAccountLocked signals the lockout threshold was just crossed (the caller
	// audits the event). Mapped to the same generic 401 as ErrInvalidCredentials,
	// so it's never disclosed to the client.
	ErrAccountLocked = errors.New("account temporarily locked")
)

// Manager owns auth state: the JWT secret and DB handle.
type Manager struct {
	db               *gorm.DB
	jwtSecret        []byte
	lockoutThreshold int
	lockoutWindow    time.Duration
}

// NewManager constructs an auth Manager. lockoutThreshold <= 0 disables the
// per-account brute-force lockout.
func NewManager(db *gorm.DB, jwtSecret string, lockoutThreshold int, lockoutWindow time.Duration) *Manager {
	return &Manager{db: db, jwtSecret: []byte(jwtSecret), lockoutThreshold: lockoutThreshold, lockoutWindow: lockoutWindow}
}

// Authenticate verifies credentials and returns the user on success. The
// FailedLoginCount is NOT reset here — a correct password with a pending second
// factor must keep counting until RecordLoginSuccess — so call RecordLoginSuccess
// only once every factor passes, and RecordFailure on a failed second factor.
func (m *Manager) Authenticate(username, password string) (*models.User, error) {
	var u models.User
	if err := m.db.Where("username = ?", username).First(&u).Error; err != nil {
		// Constant-cost compare so timing doesn't reveal whether the user exists.
		DummyCompare(password)
		return nil, ErrInvalidCredentials
	}
	// Locked: reject without checking the password (no audit spam; the lock was
	// already audited when it was triggered). Still constant-cost.
	if m.locked(&u) {
		DummyCompare(password)
		return nil, ErrInvalidCredentials
	}
	if u.PasswordHash == "" || !CheckPassword(u.PasswordHash, password) {
		if m.RecordFailure(&u) {
			return &u, ErrAccountLocked // just locked — let the caller audit
		}
		return nil, ErrInvalidCredentials
	}
	if u.Disabled {
		return nil, ErrUserDisabled
	}
	return &u, nil
}

// IsLocked reports whether the account is within an active lockout window.
func (m *Manager) IsLocked(u *models.User) bool { return m.locked(u) }

// locked reports whether the account is within an active lockout window.
func (m *Manager) locked(u *models.User) bool {
	return u.LockedUntil != nil && time.Now().Before(*u.LockedUntil)
}

// RecordFailure increments the failed-attempt counter and, at the threshold,
// locks the account for the configured window (resetting the counter). Returns
// true when this call just triggered the lock. No-op when lockout is disabled.
func (m *Manager) RecordFailure(u *models.User) bool {
	if m.lockoutThreshold <= 0 {
		return false
	}
	u.FailedLoginCount++
	if u.FailedLoginCount >= m.lockoutThreshold {
		until := time.Now().Add(m.lockoutWindow)
		u.FailedLoginCount, u.LockedUntil = 0, &until
		m.db.Model(u).Updates(map[string]any{"failed_login_count": 0, "locked_until": until})
		return true
	}
	m.db.Model(u).Update("failed_login_count", u.FailedLoginCount)
	return false
}

// RecordLoginSuccess stamps the sign-in time and clears the failed-attempt
// counter + any lock after a fully successful login (all factors passed; also
// for SSO and passkey sign-ins).
func (m *Manager) RecordLoginSuccess(u *models.User) {
	now := time.Now()
	u.FailedLoginCount, u.LockedUntil, u.LastLoginAt = 0, nil, &now
	m.db.Model(u).Updates(map[string]any{"failed_login_count": 0, "locked_until": nil, "last_login_at": now})
}

// ClearFailures resets the failed-attempt counter after a correct password
// re-check (a signed-in user confirming it's them).
func (m *Manager) ClearFailures(u *models.User) {
	if u.FailedLoginCount != 0 {
		u.FailedLoginCount = 0
		m.db.Model(u).Update("failed_login_count", 0)
	}
}

// Unlock clears a lockout (admin action).
func (m *Manager) Unlock(userID string) error {
	return m.db.Model(&models.User{}).Where("id = ?", userID).
		Updates(map[string]any{"failed_login_count": 0, "locked_until": nil}).Error
}

// NeedsOTPSetup reports whether a local user must enroll a second factor
// before doing anything else: required for everyone (otp_enforced) or for this
// user (OTPRequired), and they have neither TOTP nor a passkey. SSO users are
// exempt — MFA is the identity provider's job.
func (m *Manager) NeedsOTPSetup(u *models.User) bool {
	return u.OIDCSubject == "" && !u.OTPEnabled && (u.OTPRequired || m.otpEnforced()) && !m.userHasPasskey(u.ID)
}

// newSessionID returns a random, stable session id.
func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IssueRefreshToken creates and stores a new refresh token for a fresh session,
// returning the plaintext token (shown once) and the new session id. ua/ip
// describe the device; pass "" when unknown.
func (m *Manager) IssueRefreshToken(userID string, ua, ip string, now time.Time) (token, sessionID string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sessionID = newSessionID()
	rt := models.RefreshToken{
		UserID:     userID,
		SessionID:  sessionID,
		TokenHash:  hashToken(token),
		UserAgent:  ua,
		IP:         ip,
		LastUsedAt: now,
		ExpiresAt:  now.Add(RefreshTokenTTL),
	}
	if err = m.db.Create(&rt).Error; err != nil {
		return "", "", err
	}
	return token, sessionID, nil
}

// Refresh validates a refresh token and rotates it, returning the user, a fresh
// refresh token, and the (stable) session id. The new row inherits the session
// id + user agent and updates IP/LastUsedAt; the old token is marked rotated.
//
// Reuse detection: a rotated token presented again within RefreshReuseGrace
// mints another token in the same session (a benign concurrent refresh). Past
// the grace window it means two parties hold the token, so the entire session
// is revoked and ErrRefreshReuse is returned together with the affected user.
// Tokens revoked by logout/admin action simply fail with ErrInvalidRefresh.
func (m *Manager) Refresh(token, ua, ip string, now time.Time) (*models.User, string, string, error) {
	var rt models.RefreshToken
	if err := m.db.Where("token_hash = ?", hashToken(token)).First(&rt).Error; err != nil {
		return nil, "", "", ErrInvalidRefresh
	}
	if rt.Revoked {
		if rt.RotatedAt == nil {
			return nil, "", "", ErrInvalidRefresh // logged out / revoked
		}
		if now.Sub(*rt.RotatedAt) > RefreshReuseGrace {
			var u models.User
			m.db.First(&u, "id = ?", rt.UserID)
			if rt.SessionID != "" {
				_ = m.RevokeSession(rt.UserID, rt.SessionID)
			}
			return &u, "", "", ErrRefreshReuse
		}
		// Within grace: fall through and mint a sibling token in the session —
		// unless the session itself was revoked meanwhile.
		if !m.sessionLive(rt.UserID, rt.SessionID, now) {
			return nil, "", "", ErrInvalidRefresh
		}
	}
	if now.After(rt.ExpiresAt) {
		return nil, "", "", ErrInvalidRefresh
	}

	var u models.User
	if err := m.db.First(&u, "id = ?", rt.UserID).Error; err != nil || u.Disabled {
		return nil, "", "", ErrInvalidRefresh
	}

	// Rotate: mark the used token spent (first rotation wins the timestamp),
	// then issue a new one in the same session.
	if err := m.db.Model(&models.RefreshToken{}).
		Where("id = ? AND revoked = ?", rt.ID, false).
		Updates(map[string]any{"revoked": true, "rotated_at": now}).Error; err != nil {
		return nil, "", "", err
	}
	sid := rt.SessionID
	if sid == "" {
		sid = newSessionID() // pre-feature token: adopt a session id on first refresh
	}
	if ua == "" {
		ua = rt.UserAgent // keep the device identity across refreshes
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, "", "", err
	}
	next := base64.RawURLEncoding.EncodeToString(raw)
	row := models.RefreshToken{
		UserID:     u.ID,
		SessionID:  sid,
		TokenHash:  hashToken(next),
		UserAgent:  ua,
		IP:         ip,
		LastUsedAt: now,
		ExpiresAt:  now.Add(RefreshTokenTTL),
	}
	if err := m.db.Create(&row).Error; err != nil {
		return nil, "", "", err
	}
	return &u, next, sid, nil
}

// sessionLive reports whether a session still has at least one unrevoked,
// unexpired token (i.e. it was not signed out).
func (m *Manager) sessionLive(userID, sessionID string, now time.Time) bool {
	if sessionID == "" {
		return false
	}
	var n int64
	m.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND session_id = ? AND revoked = ? AND expires_at > ?", userID, sessionID, false, now).
		Count(&n)
	return n > 0
}

// Session is a device/login session for the active-sessions UI.
type Session struct {
	ID         string    `json:"id"` // SessionID
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
	LastUsedAt time.Time `json:"last_used_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListSessions returns the user's live sessions (one per SessionID), newest
// activity first.
func (m *Manager) ListSessions(userID string, now time.Time) ([]Session, error) {
	var rows []models.RefreshToken
	if err := m.db.
		Where("user_id = ? AND revoked = ? AND expires_at > ?", userID, false, now).
		Order("last_used_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		if r.SessionID == "" || seen[r.SessionID] {
			continue
		}
		seen[r.SessionID] = true
		out = append(out, Session{
			ID: r.SessionID, UserAgent: r.UserAgent, IP: r.IP,
			LastUsedAt: r.LastUsedAt, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// RevokeSession revokes every refresh token in a session (scoped to the user).
func (m *Manager) RevokeSession(userID string, sessionID string) error {
	return m.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND session_id = ?", userID, sessionID).
		Update("revoked", true).Error
}

// RevokeOtherSessions revokes all of the user's sessions except keepSID.
func (m *Manager) RevokeOtherSessions(userID string, keepSID string) error {
	return m.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND session_id <> ?", userID, keepSID).
		Update("revoked", true).Error
}

// RevokeAllSessions revokes every active session for a user — an admin
// "force logout". The next refresh fails; any live access token expires
// within its short TTL.
func (m *Manager) RevokeAllSessions(userID string) error {
	return m.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked = ?", userID, false).
		Update("revoked", true).Error
}

// RevokeRefreshToken signs out the session a refresh token belongs to (logout).
// The whole session is revoked — not just this row — so a sibling minted in the
// reuse-grace window, or a just-rotated predecessor, can't resurrect it.
func (m *Manager) RevokeRefreshToken(token string) error {
	var rt models.RefreshToken
	if err := m.db.Where("token_hash = ?", hashToken(token)).First(&rt).Error; err != nil {
		return nil // unknown token: nothing to revoke
	}
	if rt.SessionID != "" {
		return m.RevokeSession(rt.UserID, rt.SessionID)
	}
	return m.db.Model(&models.RefreshToken{}).Where("id = ?", rt.ID).Update("revoked", true).Error
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Package ephemeral is a DB-backed store for short-lived, single-use tokens
// (OIDC login state, WebAuthn challenges, SSO handoff codes, SSE tickets).
//
// It replaces per-process maps so a ceremony begun on one API replica can be
// completed on another, and an in-flight login survives a restart. Ids are
// random 256-bit values returned to the client; only their sha256 is stored.
// Take is single-use even under concurrency: the row is deleted and the value
// is honoured only by the caller whose DELETE affected it.
package ephemeral

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Kinds namespace the stored values; an id is only redeemable for its kind.
const (
	KindOIDCState = "oidc_state"
	KindWebAuthn  = "webauthn"
	KindSSOCode   = "sso_code"
	KindSSETicket = "sse_ticket" // must match events.ticketKind
	KindGitOAuth  = "git_oauth"  // a "Sign in with Gitea" in progress
)

// Store persists ephemeral tokens in the shared database.
type Store struct {
	db  *gorm.DB
	now func() time.Time // injectable for tests
}

// New builds a Store over db.
func New(db *gorm.DB) *Store { return &Store{db: db, now: time.Now} }

// Put stores value (JSON-encoded) under a fresh random id for ttl and returns
// the id to hand to the client.
func (s *Store) Put(kind string, value any, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	row := models.EphemeralToken{KeyHash: hash(id), Kind: kind, Data: string(data), ExpiresAt: s.now().Add(ttl)}
	if err := s.db.Create(&row).Error; err != nil {
		return "", err
	}
	return id, nil
}

// Take redeems id: it returns false for an unknown, expired, wrong-kind or
// already-redeemed id, and otherwise decodes the value into out. The token is
// consumed either way (an expired row is deleted too).
func (s *Store) Take(kind, id string, out any) (bool, error) {
	if id == "" {
		return false, nil
	}
	h := hash(id)
	var row models.EphemeralToken
	err := s.db.Where(&models.EphemeralToken{KeyHash: h, Kind: kind}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Single-use: only the caller whose delete removed the row may use it.
	res := s.db.Where(&models.EphemeralToken{KeyHash: h, Kind: kind}).Delete(&models.EphemeralToken{})
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected != 1 || s.now().After(row.ExpiresAt) {
		return false, nil
	}
	if out != nil {
		if err := json.Unmarshal([]byte(row.Data), out); err != nil {
			return false, err
		}
	}
	return true, nil
}

// Sweep deletes expired tokens (called by the scheduler).
func (s *Store) Sweep() (int64, error) {
	res := s.db.Where("expires_at < ?", s.now()).Delete(&models.EphemeralToken{})
	return res.RowsAffected, res.Error
}

func hash(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

package auth

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/config"
	"github.com/joaoteixeira10/orbit/api/internal/ephemeral"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

// ErrWebAuthnDisabled is returned when passkeys are not configured (no RP ID).
var ErrWebAuthnDisabled = errors.New("webauthn is not enabled")

// ErrChallengeExpired is returned when a ceremony's session id is unknown/expired.
var ErrChallengeExpired = errors.New("challenge expired or not found")

// challengeTTL bounds how long a begin→finish ceremony may take.
const challengeTTL = 3 * time.Minute

// WebAuthn wraps the go-webauthn engine plus a short-lived challenge store. The
// store bridges the begin/finish round-trip for the pre-auth ceremonies
// (passwordless login and the password-login second factor) where there is no
// JWT context yet — the client gets an opaque session id from *begin and
// returns it on *finish. Challenges live in the shared ephemeral store, so a
// ceremony may begin and finish on different replicas.
type WebAuthn struct {
	wa *webauthn.WebAuthn
	db *gorm.DB
	ch *ephemeral.Store
}

// NewWebAuthn builds the service. If no RP ID is configured it returns a
// disabled service whose handlers report 501 (mirrors the OIDC scaffolding).
func NewWebAuthn(db *gorm.DB, cfg *config.Config) (*WebAuthn, error) {
	if cfg.WebAuthnRPID == "" || len(cfg.WebAuthnRPOrigins) == 0 {
		return &WebAuthn{db: db, ch: ephemeral.New(db)}, nil
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnRPDisplayName,
		RPOrigins:     cfg.WebAuthnRPOrigins,
	})
	if err != nil {
		return nil, err
	}
	return &WebAuthn{wa: wa, db: db, ch: ephemeral.New(db)}, nil
}

// Enabled reports whether passkeys are configured.
func (w *WebAuthn) Enabled() bool { return w != nil && w.wa != nil }

// UserHasPasskey reports whether the user has at least one registered passkey.
// Exported so both the auth middleware and the API handlers can call it.
func UserHasPasskey(db *gorm.DB, uid string) bool {
	var n int64
	db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", uid).Limit(1).Count(&n)
	return n > 0
}

// userHasPasskey is the Manager-side helper used by the Required() middleware.
func (m *Manager) userHasPasskey(uid string) bool { return UserHasPasskey(m.db, uid) }

// ---- ceremonies -----------------------------------------------------------

// BeginRegistration starts enrolling a new passkey for an authenticated user.
func (w *WebAuthn) BeginRegistration(u *models.User) (*protocol.PublicKeyCredentialCreationOptions, string, error) {
	if !w.Enabled() {
		return nil, "", ErrWebAuthnDisabled
	}
	if err := w.ensureUserHandle(u); err != nil {
		return nil, "", err
	}
	wu := w.adapt(u)
	exclusions := make([]protocol.CredentialDescriptor, 0, len(wu.creds))
	for _, c := range wu.creds {
		exclusions = append(exclusions, c.Descriptor())
	}
	opts, session, err := w.wa.BeginRegistration(wu,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
		webauthn.WithExclusions(exclusions),
	)
	if err != nil {
		return nil, "", err
	}
	sid, err := w.putChallenge(challengeEntry{Data: *session, UserID: u.ID})
	if err != nil {
		return nil, "", err
	}
	return &opts.Response, sid, nil
}

// FinishRegistration verifies the attestation and persists the new credential.
// The raw attestation JSON must be the request body.
func (w *WebAuthn) FinishRegistration(u *models.User, name, sid string, r *http.Request) (*models.WebAuthnCredential, error) {
	if !w.Enabled() {
		return nil, ErrWebAuthnDisabled
	}
	entry, ok := w.takeChallenge(sid)
	// The registration challenge is bound to the user who began it.
	if !ok || entry.UserID != u.ID {
		return nil, ErrChallengeExpired
	}
	wu := w.adapt(u)
	cred, err := w.wa.FinishRegistration(wu, entry.Data, r)
	if err != nil {
		return nil, err
	}
	row := fromWebauthnCredential(cred, u.ID)
	row.Name = name
	if err := w.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// BeginLogin starts a non-discoverable assertion for a known user (the second
// factor after a successful password check).
func (w *WebAuthn) BeginLogin(u *models.User) (*protocol.PublicKeyCredentialRequestOptions, string, error) {
	if !w.Enabled() {
		return nil, "", ErrWebAuthnDisabled
	}
	wu := w.adapt(u)
	opts, session, err := w.wa.BeginLogin(wu)
	if err != nil {
		return nil, "", err
	}
	sid, err := w.putChallenge(challengeEntry{Data: *session, UserID: u.ID})
	if err != nil {
		return nil, "", err
	}
	return &opts.Response, sid, nil
}

// BeginDiscoverableLogin starts a passwordless assertion (no user known yet).
func (w *WebAuthn) BeginDiscoverableLogin() (*protocol.PublicKeyCredentialRequestOptions, string, error) {
	if !w.Enabled() {
		return nil, "", ErrWebAuthnDisabled
	}
	opts, session, err := w.wa.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	sid, err := w.putChallenge(challengeEntry{Data: *session, Discoverable: true})
	if err != nil {
		return nil, "", err
	}
	return &opts.Response, sid, nil
}

// FinishLogin completes either ceremony (discoverable or 2FA) keyed by the
// session id, returns the authenticated user, and updates the credential's
// sign count / backup state / last-used. The raw assertion JSON is the body.
func (w *WebAuthn) FinishLogin(sid string, r *http.Request) (*models.User, error) {
	if !w.Enabled() {
		return nil, ErrWebAuthnDisabled
	}
	entry, ok := w.takeChallenge(sid)
	if !ok {
		return nil, ErrChallengeExpired
	}

	var user models.User
	var cred *webauthn.Credential
	var err error

	if entry.Discoverable {
		cred, err = w.wa.FinishDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
			var u models.User
			if e := w.db.Where("web_authn_user_handle = ?", userHandle).First(&u).Error; e != nil {
				return nil, e
			}
			if u.Disabled {
				return nil, ErrUserDisabled
			}
			user = u
			return w.adapt(&u), nil
		}, entry.Data, r)
	} else {
		if e := w.db.First(&user, "id = ?", entry.UserID).Error; e != nil {
			return nil, e
		}
		if user.Disabled {
			return nil, ErrUserDisabled
		}
		cred, err = w.wa.FinishLogin(w.adapt(&user), entry.Data, r)
	}
	if err != nil {
		return nil, err
	}

	w.touchCredential(cred)
	return &user, nil
}

// touchCredential persists the post-assertion state of the matched credential.
func (w *WebAuthn) touchCredential(cred *webauthn.Credential) {
	now := time.Now()
	w.db.Model(&models.WebAuthnCredential{}).
		Where("credential_id = ?", cred.ID).
		Updates(map[string]any{
			"sign_count":    cred.Authenticator.SignCount,
			"clone_warning": cred.Authenticator.CloneWarning,
			"backup_state":  cred.Flags.BackupState,
			"last_used_at":  &now,
		})
}

// ---- helpers --------------------------------------------------------------

// adapt loads the user's stored credentials and wraps them in a webauthn.User.
func (w *WebAuthn) adapt(u *models.User) *webauthnUser {
	var rows []models.WebAuthnCredential
	w.db.Where("user_id = ?", u.ID).Find(&rows)
	creds := make([]webauthn.Credential, 0, len(rows))
	for i := range rows {
		creds = append(creds, toWebauthnCredential(rows[i]))
	}
	return &webauthnUser{u: u, creds: creds}
}

// ensureUserHandle lazily assigns a random opaque WebAuthn user handle.
func (w *WebAuthn) ensureUserHandle(u *models.User) error {
	if len(u.WebAuthnUserHandle) > 0 {
		return nil
	}
	handle := make([]byte, 32)
	if _, err := rand.Read(handle); err != nil {
		return err
	}
	if err := w.db.Model(u).Update("web_authn_user_handle", handle).Error; err != nil {
		return err
	}
	u.WebAuthnUserHandle = handle
	return nil
}

// webauthnUser adapts a models.User to the webauthn.User interface without
// polluting the GORM model.
type webauthnUser struct {
	u     *models.User
	creds []webauthn.Credential
}

func (x *webauthnUser) WebAuthnID() []byte   { return x.u.WebAuthnUserHandle }
func (x *webauthnUser) WebAuthnName() string { return x.u.Username }
func (x *webauthnUser) WebAuthnDisplayName() string {
	if x.u.DisplayName != "" {
		return x.u.DisplayName
	}
	return x.u.Username
}
func (x *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return x.creds }

func toWebauthnCredential(row models.WebAuthnCredential) webauthn.Credential {
	var transports []protocol.AuthenticatorTransport
	if row.Transports != "" {
		_ = json.Unmarshal([]byte(row.Transports), &transports)
	}
	return webauthn.Credential{
		ID:              row.CredentialID,
		PublicKey:       row.PublicKey,
		AttestationType: row.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			UserPresent:    row.UserPresent,
			UserVerified:   row.UserVerified,
			BackupEligible: row.BackupEligible,
			BackupState:    row.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       row.AAGUID,
			SignCount:    row.SignCount,
			CloneWarning: row.CloneWarning,
		},
	}
}

func fromWebauthnCredential(cred *webauthn.Credential, userID string) models.WebAuthnCredential {
	transports, _ := json.Marshal(cred.Transport)
	return models.WebAuthnCredential{
		UserID:          userID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CloneWarning:    cred.Authenticator.CloneWarning,
		Transports:      string(transports),
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
		UserPresent:     cred.Flags.UserPresent,
		UserVerified:    cred.Flags.UserVerified,
	}
}

// ---- challenge store ------------------------------------------------------

// challengeEntry is the stored state of one begin→finish ceremony.
type challengeEntry struct {
	Data         webauthn.SessionData `json:"data"`
	UserID       string               `json:"user_id,omitempty"` // pinned user (registration / 2FA)
	Discoverable bool                 `json:"discoverable,omitempty"`
}

// putChallenge stores a ceremony under a fresh opaque, single-use id.
func (w *WebAuthn) putChallenge(e challengeEntry) (string, error) {
	return w.ch.Put(ephemeral.KindWebAuthn, e, challengeTTL)
}

// takeChallenge redeems a ceremony id, rejecting unknown/expired/used ids.
func (w *WebAuthn) takeChallenge(sid string) (challengeEntry, bool) {
	var e challengeEntry
	ok, err := w.ch.Take(ephemeral.KindWebAuthn, sid, &e)
	return e, ok && err == nil
}

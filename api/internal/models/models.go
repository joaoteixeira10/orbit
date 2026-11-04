// Package models defines the GORM entities of Orbit: the accounts side (users,
// roles, sessions, API tokens, passkeys, settings, the audit log) here, and the
// planner side (workspaces and their content) in planner.go.
//
// Schema is created with GORM AutoMigrate so a single struct definition is
// portable across sqlite / mysql / postgres without dialect-specific SQL.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base is embedded in every entity: a UUIDv7 surrogate key + timestamps.
// UUIDv7 is time-ordered and lexically sortable, so ORDER BY id keeps insertion
// order (newest-first lists, owner backfill, and the audit hash chain rely on it).
type Base struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate assigns a UUIDv7 id when one isn't already set. Embedded in every
// model, so all entities get UUID primary keys + generation for free.
func (b *Base) BeforeCreate(*gorm.DB) error {
	if b.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		b.ID = id.String()
	}
	return nil
}

// ---- Auth -----------------------------------------------------------------

// RoleName identifies the built-in role presets. Custom roles are full Role
// entities (see below).
type RoleName string

const (
	RoleAdmin  RoleName = "admin"
	RoleMember RoleName = "member"
	// RoleNone has no instance privileges; it's the default for just-in-time
	// SSO provisioning. Such a user can't create workspaces until an admin
	// makes them a member, but can still be invited into someone's workspace.
	RoleNone RoleName = "none"
)

// User is a local or OIDC-mapped account.
type User struct {
	Base
	Username     string `gorm:"uniqueIndex;size:190;not null" json:"username"`
	DisplayName  string `gorm:"size:190" json:"display_name"`
	Email        string `gorm:"size:190" json:"email"`
	AvatarURL    string `gorm:"type:text" json:"avatar_url"` // data URL
	PasswordHash string `gorm:"size:255" json:"-"`           // bcrypt; empty for pure-OIDC users
	Disabled     bool   `gorm:"not null;default:false" json:"disabled"`
	// Owner marks the single instance owner ("god mode") — set on the bootstrap
	// admin and transferable. Owner-only powers are tied to this identity, not to
	// any RBAC privilege, so they can't be self-granted. Surfaced via /auth/me's
	// is_owner, never in user lists.
	Owner       bool   `gorm:"not null;default:false" json:"-"`
	OIDCSubject string `gorm:"size:255;index" json:"-"` // sub claim, when SSO-provisioned
	// OIDC is a derived flag (set by AfterFind): true for SSO-provisioned
	// accounts, whose identity fields are managed by the identity provider.
	OIDC bool `gorm:"-" json:"oidc"`
	// LockedUntilView (derived) is LockedUntil while a lockout is active, for
	// the admin's user list; nil otherwise.
	LockedUntilView *time.Time `gorm:"-" json:"locked_until,omitempty"`
	// HasPasskey (set by the user list) reports a registered passkey, which
	// counts as the second factor.
	HasPasskey bool `gorm:"-" json:"has_passkey"`
	// NeedsOTPSetup (set by the user list): two-factor is required for this
	// user (by the policy or per user) but not set up yet.
	NeedsOTPSetup bool `gorm:"-" json:"needs_otp_setup"`
	// TokenVersion invalidates previously-issued access tokens when bumped
	// (the "rotate token" action revokes a token shared to a shell).
	TokenVersion uint `gorm:"not null;default:1" json:"-"`
	// MustChangePassword forces a password change before any other action.
	MustChangePassword bool `gorm:"not null;default:false" json:"must_change_password"`
	// Brute-force lockout: FailedLoginCount accumulates consecutive failed
	// password/second-factor attempts; at the configured threshold the account is
	// locked until LockedUntil (a temporary timed lock, reset on a full success).
	FailedLoginCount int        `gorm:"not null;default:0" json:"-"`
	LockedUntil      *time.Time `json:"-"`
	// LastLoginAt is the last completed sign-in (password, SSO or passkey).
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	// TOTP two-factor: sealed secret + enabled flag + hashed recovery codes.
	OTPSecretEnc   []byte `json:"-"`
	OTPEnabled     bool   `gorm:"not null;default:false" json:"otp_enabled"`
	OTPBackupCodes string `gorm:"type:text" json:"-"` // JSON []string of sha256 hashes
	// OTPRequired makes a second factor mandatory for this one user, on top of
	// the instance-wide otp_enforced policy (an LB Admin addition to the PKI model).
	OTPRequired bool `gorm:"not null;default:false" json:"otp_required"`

	// WebAuthnUserHandle is an opaque, random per-user handle (NOT the DB id)
	// used as the WebAuthn user id and as the lookup key for discoverable
	// (passwordless) logins. Generated lazily on first passkey registration.
	WebAuthnUserHandle []byte `gorm:"size:255;uniqueIndex" json:"-"`

	// Roles are the assigned RBAC roles; effective access is their union.
	Roles []Role    `gorm:"many2many:user_roles;" json:"roles,omitempty"`
	Prefs UserPrefs `gorm:"constraint:OnDelete:CASCADE" json:"prefs"`
}

// AfterFind derives the OIDC and locked flags.
func (u *User) AfterFind(*gorm.DB) error {
	u.OIDC = u.OIDCSubject != ""
	if u.LockedUntil != nil && time.Now().Before(*u.LockedUntil) {
		u.LockedUntilView = u.LockedUntil
	}
	return nil
}

// Role is a named bundle of instance-level privileges (managing users, roles,
// the audit log, creating workspaces). Privilege lists are stored as JSON text
// for DB portability. Built-in roles are immutable; the admin built-in is a
// superuser. What a user may do inside a workspace is not a role: it is their
// membership of that workspace (see WorkspaceMember).
type Role struct {
	Base
	Name             string `gorm:"uniqueIndex;size:64;not null" json:"name"`
	Description      string `gorm:"size:255" json:"description"`
	Builtin          bool   `gorm:"not null;default:false" json:"builtin"`
	Superuser        bool   `gorm:"not null;default:false" json:"superuser"`
	GlobalPrivileges string `gorm:"type:text" json:"-"` // JSON []string
}

// Theme is the per-user colour scheme.
type Theme string

const (
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
	ThemeSystem Theme = "system"
)

// UserPrefs are server-side per-user UI settings (so they follow the user
// across devices).
type UserPrefs struct {
	Base
	UserID   string `gorm:"uniqueIndex;size:36;not null" json:"user_id"`
	Timezone string `gorm:"size:64;not null;default:local" json:"timezone"` // "local" = device/browser zone
	// DeviceTZ is the zone the browser last reported, used for reminders
	// while Timezone is "local".
	DeviceTZ string `gorm:"size:64" json:"device_tz"`
	Theme    Theme  `gorm:"size:10;not null;default:system" json:"theme"`
	// AccentColor is a personal #rrggbb accent that overrides the org accent for
	// this user only (empty = follow the org/developer-theme accent).
	AccentColor string `gorm:"size:9" json:"accent_color"`
	// WeekStart is the first day of the week in calendars: 1 = Monday, 0 = Sunday.
	WeekStart int `gorm:"not null;default:1" json:"week_start"`
	// TimeFormat is "24h" or "12h".
	TimeFormat string `gorm:"size:4;not null;default:24h" json:"time_format"`
	// Notifications: master toggle for reminders (tasks due, events starting)
	// + chosen channel keys (JSON []string), restricted to the channels the
	// admin has enabled.
	NotifyEvents   bool   `gorm:"not null;default:false" json:"notify_events"`
	NotifyChannels string `gorm:"type:text" json:"-"` // JSON []string; exposed via DTO
	// ChannelDetails: the shared channels (chats, topics, webhooks — places
	// the owner set up, which others may read) get the notification's
	// text. Off (the default), they get only a ping; email, which is
	// personal, always has the text.
	ChannelDetails bool `gorm:"not null;default:false" json:"channel_details"`
	// NotifyPrefs is, per kind of notification (reminder, assigned,
	// workspace, activity, digest), where it goes besides the inbox: JSON
	// {"<kind>": {"push": bool, "channels": bool}}; missing kinds take the
	// defaults (package inbox).
	NotifyPrefs string `gorm:"type:text" json:"-"`
	// The daily summary: on or off, at "HH:MM" in the user's zone; DigestSent
	// is the local date it was last sent.
	DigestEnabled bool   `gorm:"not null;default:false" json:"digest_enabled"`
	DigestTime    string `gorm:"size:5;not null;default:08:00" json:"digest_time"`
	DigestSent    string `gorm:"size:10" json:"-"`
	// Locale is the interface language (en, pt), for text sent from the
	// server: push notifications, channels, the summary.
	Locale string `gorm:"size:8" json:"locale"`
	// PageEditor is how this user edits pages: "visual" (blocks, the default)
	// or "markdown" (the source). Pages are Markdown either way.
	PageEditor string `gorm:"size:10;not null;default:visual" json:"page_editor"`
	// DevTheme is this user's own developer theme, when the owner lets each
	// person choose: "" = as the instance, "none", or a theme id.
	DevTheme string `gorm:"size:10" json:"dev_theme"`
	// RemoteImages is whether pages show images from other sites at once
	// ("always") or only after a click ("ask", the default): such an image
	// tells its site who reads the page, and when.
	RemoteImages string `gorm:"size:8" json:"remote_images"`
	// SketchColors is the person's own palette in Quick Draw (JSON
	// []string of #rrggbb).
	SketchColors JSONText `gorm:"type:text" json:"sketch_colors"`
	// EditorThemes keeps an editor light or dark whatever Orbit's theme
	// (JSON {"sheets"|"diagrams"|"whiteboards": "light"|"dark"}; absent =
	// follow Orbit's).
	EditorThemes JSONText `gorm:"type:text" json:"editor_themes"`
	// CalendarColors are this person's own colours for calendars (JSON
	// {key: "#rrggbb"}; keys: a workspace id, "list:<id>", "sub:<id>",
	// "holidays"); absent = the calendar's own colour.
	CalendarColors JSONText `gorm:"type:text" json:"calendar_colors"`
	// FocusLengths are this person's own lengths for the focus timer, in
	// minutes (JSON []int), besides 15, 25 and 50.
	FocusLengths JSONText `gorm:"type:text" json:"focus_lengths"`
	// Quiet hours, "HH:MM" to "HH:MM" in the user's zone ("" = none): nothing
	// reaches devices or channels then (it waits in the inbox, and one
	// summary goes when they end) — reminders too, unless QuietReminders.
	QuietFrom      string `gorm:"size:5" json:"quiet_from"`
	QuietTo        string `gorm:"size:5" json:"quiet_to"`
	QuietReminders bool   `gorm:"not null;default:false" json:"quiet_reminders"`
	// HolidayEffectsOff: no falling snow, bats, confetti… in a holiday theme
	// (its colours and art stay — the owner's).
	HolidayEffectsOff bool `gorm:"not null;default:false" json:"-"`
}

// RefreshToken allows revocable refresh of short-lived JWT access tokens.
// SessionID is a stable id for a device/login session that survives token
// rotation (each refresh mints a new row sharing the SessionID), so the active
// sessions UI can show one entry per device. UserAgent/IP/LastUsedAt describe it.
type RefreshToken struct {
	Base
	UserID     string    `gorm:"index;size:36;not null" json:"user_id"`
	SessionID  string    `gorm:"index;size:36" json:"-"`                // stable across rotations
	TokenHash  string    `gorm:"uniqueIndex;size:64;not null" json:"-"` // sha256 hex of the token
	UserAgent  string    `gorm:"type:text" json:"-"`
	IP         string    `gorm:"size:64" json:"-"`
	LastUsedAt time.Time `json:"-"`
	ExpiresAt  time.Time `gorm:"index;not null" json:"expires_at"`
	Revoked    bool      `gorm:"not null;default:false" json:"revoked"`
	// RotatedAt is set when this token was spent by a refresh (and replaced by a
	// successor). Distinguishes "rotated" from "revoked by logout/admin", so a
	// replay of a rotated token beyond the grace window is detected as reuse.
	RotatedAt *time.Time `json:"-"`
}

// EphemeralToken is short-lived, single-use pre-auth state shared by every API
// replica: OIDC login state (nonce + PKCE verifier), WebAuthn ceremony
// challenges, SSO one-time handoff codes, and SSE stream tickets. Only a sha256
// of the opaque id is stored; Kind namespaces the value so an id minted for one
// purpose can't be redeemed for another. Rows are deleted on redemption and
// swept once expired.
type EphemeralToken struct {
	KeyHash   string    `gorm:"primaryKey;size:64"`
	Kind      string    `gorm:"size:32;not null"`
	Data      string    `gorm:"type:text"` // JSON payload
	ExpiresAt time.Time `gorm:"index;not null"`
}

// OAuthClient is an app that signs people in to Orbit's API with OAuth
// (registered by itself, RFC 7591): an MCP client such as Claude or ChatGPT.
// What it gets is an API token of the person who allowed it.
type OAuthClient struct {
	Base
	Name         string `gorm:"size:200" json:"client_name"`
	RedirectURIs string `gorm:"type:text" json:"-"` // JSON []string
}

// APIToken is a long-lived, revocable bearer token for programmatic access
// (scripts, shortcuts). It carries the owning user's access; only the sha256 hash is
// stored. Prefix is a non-secret display hint.
type APIToken struct {
	Base
	UserID     string     `gorm:"index;size:36;not null" json:"user_id"`
	Name       string     `gorm:"size:128" json:"name"`
	Prefix     string     `gorm:"size:24" json:"prefix"`
	TokenHash  string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Revoked    bool       `gorm:"not null;default:false" json:"-"`
	// Optional narrowing: the token can use only these instance privileges
	// (JSON []string; empty = all of the owner's). ReadOnly limits it to reading
	// workspace content. It never grants more than the owner has.
	ScopePrivileges string `gorm:"type:text" json:"-"`
	ReadOnly        bool   `gorm:"not null;default:false" json:"read_only"`
	// Decoded scope for API responses (set in AfterFind / by the handler).
	Privileges []string `gorm:"-" json:"privileges,omitempty"`
	// Username of the token's owner (admin list only).
	Username string `gorm:"-" json:"username,omitempty"`
}

// AfterFind decodes the token's scope for API responses.
func (t *APIToken) AfterFind(*gorm.DB) error {
	_ = json.Unmarshal([]byte(orEmptyList(t.ScopePrivileges)), &t.Privileges)
	return nil
}

func orEmptyList(s string) string {
	if s == "" {
		return "[]"
	}
	return s
}

// WebAuthnCredential is a registered passkey / FIDO2 authenticator for a user.
// Public keys are not secret, so (unlike the TOTP secret) nothing here is
// sealed. Byte columns carry no explicit SQL type so GORM picks each dialect's
// own (blob / longblob / bytea); the uniquely-indexed CredentialID gets a size
// so MySQL uses an indexable varbinary(255).
type WebAuthnCredential struct {
	Base
	UserID          string     `gorm:"index;size:36;not null" json:"user_id"`
	Name            string     `gorm:"size:128" json:"name"` // user-given label
	CredentialID    []byte     `gorm:"size:255;uniqueIndex" json:"-"`
	PublicKey       []byte     `json:"-"` // COSE public key
	AttestationType string     `gorm:"size:32" json:"-"`
	AAGUID          []byte     `json:"-"`
	SignCount       uint32     `gorm:"not null;default:0" json:"-"`
	CloneWarning    bool       `gorm:"not null;default:false" json:"-"`
	Transports      string     `gorm:"type:text" json:"-"` // JSON []string of AuthenticatorTransport
	BackupEligible  bool       `gorm:"not null;default:false" json:"backup_eligible"`
	BackupState     bool       `gorm:"not null;default:false" json:"backed_up"`
	UserPresent     bool       `gorm:"not null;default:false" json:"-"`
	UserVerified    bool       `gorm:"not null;default:false" json:"-"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty"`
}

// Setting is a global key/value (JSON) store: branding (logo, favicon,
// app_name, accent), defaults, OIDC config, etc.
type Setting struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"` // JSON-encoded
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditLog records every mutating action for traceability.
type AuditLog struct {
	Base
	UserID string `gorm:"index;size:36" json:"user_id"`
	// ImpersonatorID is the real admin's id when the action was performed via
	// impersonation (UserID is then the impersonated user). Nil otherwise.
	ImpersonatorID *string `gorm:"index;size:36" json:"impersonator_id,omitempty"`
	Action         string  `gorm:"size:64;not null" json:"action"`
	TargetType     string  `gorm:"size:64" json:"target_type"`
	TargetID       string  `gorm:"size:64" json:"target_id"`
	IP             string  `gorm:"size:64" json:"ip"`
	Detail         string  `gorm:"type:text" json:"detail"` // JSON
	// Hash chain for tamper-evidence: Hash = sha256(PrevHash || canonical(entry)).
	// PrevHash links to the previous entry's Hash; a break is detectable via
	// GET /audit/verify. Set by the recorder under a serialized insert.
	PrevHash string `gorm:"size:64" json:"-"`
	Hash     string `gorm:"size:64;index" json:"-"`
}

// AllModels is the ordered AutoMigrate list.
func AllModels() []any {
	return []any{
		&Role{},
		&User{},
		&UserPrefs{},
		&RefreshToken{},
		&APIToken{},
		&WebAuthnCredential{},
		&Setting{},
		&AuditLog{},
		&EphemeralToken{},
		&Workspace{},
		&WorkspaceMember{},
		&OAuthClient{},
		&Page{},
		&PageVersion{},
		&ShareLink{},
		&PageChange{},
		&Form{},
		&PageFavorite{},
		&TaskList{},
		&Task{},
		&Note{},
		&Notification{},
		&PushSubscription{},
		&GitAccount{},
		&GitLink{},
		&Upload{},
		&UploadBlob{},
		&Blob{},
		&UploadText{},
		&CollabUpdate{},
		&CollabDoc{},
		&WorkspaceInvite{},
		&Course{},
		&CourseSlot{},
		&Grade{},
		&CourseGoal{},
		&TaskComment{},
		&TaskEvent{},
		&PersonalChannel{},
		&Activity{},
		&Event{},
		&CalendarSubscription{},
		&ExternalEvent{},
		&Drawing{},
		&Habit{},
		&HabitCheck{},
		&FocusSession{},
		&Goal{},
		&SmartList{},
		&Sheet{},
		&Diagram{},
		&Folder{},
		&TaskBlock{},
		&JournalEntry{},
		&Bookmark{},
		&BookmarkImage{},
		&CalDAVLink{},
		&CalDAVItem{},
		&CalendarFeed{},
	}
}

// JSONText is JSON kept as text in a column, sent as the JSON it holds (so
// the signed-in person's preferences come with their values, not a string);
// empty or not JSON goes out as null.
type JSONText string

// MarshalJSON writes the JSON held, or null.
func (j JSONText) MarshalJSON() ([]byte, error) {
	if j == "" || !json.Valid([]byte(j)) {
		return []byte("null"), nil
	}
	return []byte(j), nil
}

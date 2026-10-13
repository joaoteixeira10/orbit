package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/ephemeral"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/safedial"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const oidcSettingKey = "oidc_config"

var (
	// ErrOIDCDisabled is returned when SSO is not configured/enabled.
	ErrOIDCDisabled = errors.New("OIDC is not enabled")
	// ErrOIDCState is returned for an unknown/expired login state (CSRF guard).
	ErrOIDCState = errors.New("OIDC state expired or invalid")
)

// OIDCSettings is the admin-managed SSO configuration, persisted as JSON in the
// settings table. The client secret is stored sealed (never in plaintext at
// rest, never returned to the UI).
type OIDCSettings struct {
	Enabled            bool   `json:"enabled"`
	Issuer             string `json:"issuer"`
	ClientID           string `json:"client_id"`
	ClientSecretSealed []byte `json:"client_secret_sealed,omitempty"`
	RedirectURL        string `json:"redirect_url"`
	Scopes             string `json:"scopes"`       // extra space-separated scopes (besides openid/profile/email)
	RoleClaim          string `json:"role_claim"`   // ID-token claim mapped to RBAC role names
	DefaultRole        string `json:"default_role"` // role for new / unmapped users (defaults to the no-access "none" role)
	// AllowAdmin lets the identity provider grant admin (superuser) roles;
	// off, a claim naming `admin` (or an admin default role) is ignored.
	AllowAdmin bool `json:"allow_admin"`
}

// oidcStateTTL bounds how long a login may sit at the IdP before the callback.
const oidcStateTTL = 10 * time.Minute

// OIDC owns SSO state: settings persistence, a cached provider, and the
// short-lived login state (nonce + PKCE verifier keyed by the state param),
// kept in the shared ephemeral store so any replica can handle the callback.
type OIDC struct {
	db     *gorm.DB
	sealer *keystore.Sealer
	states *ephemeral.Store

	mu        sync.Mutex
	cachedIss string
	provider  *oidc.Provider
}

// NewOIDC builds the SSO service.
func NewOIDC(db *gorm.DB, sealer *keystore.Sealer) *OIDC {
	return &OIDC{db: db, sealer: sealer, states: ephemeral.New(db)}
}

// Load reads the stored settings (zero-value if unset).
func (o *OIDC) Load() OIDCSettings {
	var row models.Setting
	var s OIDCSettings
	if err := o.db.Where(&models.Setting{Key: oidcSettingKey}).First(&row).Error; err == nil {
		_ = json.Unmarshal([]byte(row.Value), &s)
	}
	return s
}

// Save persists the settings.
func (o *OIDC) Save(s OIDCSettings) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return o.db.Save(&models.Setting{Key: oidcSettingKey, Value: string(b), UpdatedAt: time.Now()}).Error
}

// Enabled reports whether SSO is configured and turned on.
func (o *OIDC) Enabled() bool {
	s := o.Load()
	return s.Enabled && s.Issuer != "" && s.ClientID != ""
}

// SealSecret seals a plaintext client secret for storage.
func (o *OIDC) SealSecret(plain string) ([]byte, error) { return o.sealer.Seal([]byte(plain)) }

func (o *OIDC) clientSecret(s OIDCSettings) (string, error) {
	if len(s.ClientSecretSealed) == 0 {
		return "", nil
	}
	b, err := o.sealer.Unseal(s.ClientSecretSealed)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// oidcGuard binds an SSRF-guarded HTTP client to the context so go-oidc +
// oauth2 (discovery, JWKS fetch, token exchange) can't be pointed at
// loopback/link-local hosts or redirected to the cloud-metadata endpoint.
func oidcGuard(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, safedial.GuardedClient(15*time.Second))
}

// Provider discovers and caches the OIDC provider for an issuer (re-discovers
// if the configured issuer changes).
func (o *OIDC) discover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider != nil && o.cachedIss == issuer {
		return o.provider, nil
	}
	p, err := oidc.NewProvider(oidcGuard(ctx), issuer)
	if err != nil {
		return nil, err
	}
	o.provider, o.cachedIss = p, issuer
	return p, nil
}

// TestDiscovery verifies the issuer is reachable and a valid OIDC provider.
func (o *OIDC) TestDiscovery(ctx context.Context, issuer string) error {
	_, err := oidc.NewProvider(oidcGuard(ctx), issuer)
	return err
}

func (o *OIDC) oauth2Config(s OIDCSettings, p *oidc.Provider) (*oauth2.Config, error) {
	secret, err := o.clientSecret(s)
	if err != nil {
		return nil, err
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	scopes = append(scopes, strings.Fields(s.Scopes)...)
	return &oauth2.Config{
		ClientID:     s.ClientID,
		ClientSecret: secret,
		RedirectURL:  s.RedirectURL,
		Endpoint:     p.Endpoint(),
		Scopes:       scopes,
	}, nil
}

// BeginLogin returns the IdP authorization URL the browser should be sent to.
func (o *OIDC) BeginLogin(ctx context.Context) (string, error) {
	s := o.Load()
	if !s.Enabled || s.Issuer == "" {
		return "", ErrOIDCDisabled
	}
	p, err := o.discover(ctx, s.Issuer)
	if err != nil {
		return "", err
	}
	cfg, err := o.oauth2Config(s, p)
	if err != nil {
		return "", err
	}
	nonce, verifier := randToken(), oauth2.GenerateVerifier()
	state, err := o.states.Put(ephemeral.KindOIDCState, oidcState{Nonce: nonce, Verifier: verifier}, oidcStateTTL)
	if err != nil {
		return "", err
	}
	return cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// OIDCUserInfo is the verified identity returned from a callback.
type OIDCUserInfo struct {
	Subject  string
	Email    string
	Name     string
	Username string
	Roles    []string
}

// Finish validates the callback (state, code), verifies the ID token (nonce +
// signature), and returns the user's identity + mapped role claim values.
func (o *OIDC) Finish(ctx context.Context, state, code string) (*OIDCUserInfo, error) {
	var st oidcState
	if ok, err := o.states.Take(ephemeral.KindOIDCState, state, &st); err != nil || !ok {
		return nil, ErrOIDCState
	}
	// Guard the token-exchange + JWKS-verify outbound calls (covers oauth2 too).
	ctx = oidcGuard(ctx)
	s := o.Load()
	p, err := o.discover(ctx, s.Issuer)
	if err != nil {
		return nil, err
	}
	cfg, err := o.oauth2Config(s, p)
	if err != nil {
		return nil, err
	}
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return nil, err
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, errors.New("no id_token in token response")
	}
	idt, err := p.Verifier(&oidc.Config{ClientID: s.ClientID}).Verify(ctx, rawID)
	if err != nil {
		return nil, err
	}
	if idt.Nonce != st.Nonce {
		return nil, errors.New("OIDC nonce mismatch")
	}
	var claims map[string]any
	_ = idt.Claims(&claims)

	info := &OIDCUserInfo{Subject: idt.Subject}
	info.Email, _ = claims["email"].(string)
	info.Name, _ = claims["name"].(string)
	info.Username, _ = claims["preferred_username"].(string)
	if info.Username == "" {
		info.Username = info.Email
	}
	if info.Username == "" {
		info.Username = idt.Subject
	}
	if s.RoleClaim != "" {
		info.Roles = extractRoleClaim(claims[s.RoleClaim])
	}
	return info, nil
}

// extractRoleClaim coerces a role claim that may be a string or a list.
func extractRoleClaim(v any) []string {
	switch t := v.(type) {
	case string:
		return strings.Fields(t)
	case []any:
		var out []string
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

func randToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// oidcState is the pre-auth login state stored under the OAuth state param.
type oidcState struct {
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
}

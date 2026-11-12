package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
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

const (
	oidcSettingKey    = "oidc_config"    // the one provider, before there could be several
	oidcProvidersKey  = "oidc_providers" // the providers
	DefaultProviderID = "default"        // the first one (its people's subjects kept as they were)
	GoogleIssuer      = "https://accounts.google.com"
)

var (
	// ErrOIDCDisabled is returned when SSO is not configured/enabled.
	ErrOIDCDisabled = errors.New("OIDC is not enabled")
	// ErrOIDCState is returned for an unknown/expired login state (CSRF guard).
	ErrOIDCState = errors.New("OIDC state expired or invalid")
	// ErrOIDCDomain: the account's email isn't of a domain the provider allows.
	ErrOIDCDomain = errors.New("this account's email domain isn't allowed here")
)

// OIDCSettings is one identity provider (OpenID Connect; Google is one with
// its issuer set), persisted with the others as JSON in the settings table.
// The client secret is stored sealed (never in plaintext at rest, never
// returned to the UI).
type OIDCSettings struct {
	ID                 string `json:"id"`   // in URLs and subjects: [a-z0-9-]
	Name               string `json:"name"` // on the button: "Sign in with <Name>"
	Kind               string `json:"kind"` // oidc | google
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
	// GroupClaim: an ID-token claim naming the person's groups there; each
	// sign-in puts them in exactly those groups here (made as needed).
	GroupClaim string `json:"group_claim"`
	// Domains: only accounts with a verified email of these domains sign in
	// ("" any). For Google, its hd claim must match too.
	Domains []string `json:"domains"`
}

// ValidProviderID: what a provider's id may be.
var ValidProviderID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// oidcStateTTL bounds how long a login may sit at the IdP before the callback.
const oidcStateTTL = 10 * time.Minute

// OIDC owns SSO state: the providers, their discovered endpoints (cached by
// issuer), and the short-lived login state (nonce + PKCE verifier + which
// provider, keyed by the state param), kept in the shared ephemeral store so
// any replica can handle the callback.
type OIDC struct {
	db     *gorm.DB
	sealer *keystore.Sealer
	states *ephemeral.Store

	mu        sync.Mutex
	providers map[string]*oidc.Provider // by issuer
}

// NewOIDC builds the SSO service.
func NewOIDC(db *gorm.DB, sealer *keystore.Sealer) *OIDC {
	return &OIDC{db: db, sealer: sealer, states: ephemeral.New(db), providers: map[string]*oidc.Provider{}}
}

// Providers reads the stored providers — the single one from before, as the
// default provider, when there's no list yet.
func (o *OIDC) Providers() []OIDCSettings {
	var row models.Setting
	var list []OIDCSettings
	if o.db.Where(&models.Setting{Key: oidcProvidersKey}).Limit(1).Find(&row).RowsAffected == 1 {
		_ = json.Unmarshal([]byte(row.Value), &list)
		return list
	}
	if o.db.Where(&models.Setting{Key: oidcSettingKey}).Limit(1).Find(&row).RowsAffected == 1 {
		var s OIDCSettings
		if json.Unmarshal([]byte(row.Value), &s) == nil && (s.Issuer != "" || s.ClientID != "") {
			s.ID, s.Name, s.Kind = DefaultProviderID, "SSO", "oidc"
			list = append(list, s)
		}
	}
	return list
}

// SaveProviders persists the providers.
func (o *OIDC) SaveProviders(list []OIDCSettings) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return o.db.Save(&models.Setting{Key: oidcProvidersKey, Value: string(b), UpdatedAt: time.Now()}).Error
}

// Provider finds one by id.
func (o *OIDC) Provider(id string) (OIDCSettings, bool) {
	for _, p := range o.Providers() {
		if p.ID == id {
			return p, true
		}
	}
	return OIDCSettings{}, false
}

// Usable: turned on, and enough set to sign in with.
func (s OIDCSettings) Usable() bool { return s.Enabled && s.Issuer != "" && s.ClientID != "" }

// Enabled reports whether any provider can be signed in with.
func (o *OIDC) Enabled() bool { return len(o.Usable()) > 0 }

// Usable lists the providers that can be signed in with, in their order.
func (o *OIDC) Usable() []OIDCSettings {
	var out []OIDCSettings
	for _, p := range o.Providers() {
		if p.Usable() {
			out = append(out, p)
		}
	}
	return out
}

// Load is the first usable provider (else the first): the one the old,
// single-provider settings and the login page's auto-start mean.
func (o *OIDC) Load() OIDCSettings {
	if u := o.Usable(); len(u) > 0 {
		return u[0]
	}
	if all := o.Providers(); len(all) > 0 {
		return all[0]
	}
	return OIDCSettings{}
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

// OIDCClient is the HTTP client for discovery, JWKS and the token exchange:
// SSRF-guarded, so they can't be pointed at loopback / link-local hosts or
// redirected to the cloud-metadata endpoint. Tests swap it for a fake IdP's.
var OIDCClient = func() *http.Client { return safedial.GuardedClient(15 * time.Second) }

func oidcGuard(ctx context.Context) context.Context {
	return oidc.ClientContext(ctx, OIDCClient())
}

// discover finds (and caches) the OIDC provider of an issuer.
func (o *OIDC) discover(ctx context.Context, issuer string) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if p := o.providers[issuer]; p != nil {
		return p, nil
	}
	p, err := oidc.NewProvider(oidcGuard(ctx), issuer)
	if err != nil {
		return nil, err
	}
	o.providers[issuer] = p
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

// BeginLogin returns the IdP authorization URL the browser should be sent to,
// for a provider ("" the first usable one).
func (o *OIDC) BeginLogin(ctx context.Context, providerID string) (string, error) {
	s := o.Load()
	if providerID != "" {
		s, _ = o.Provider(providerID)
	}
	if !s.Usable() {
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
	state, err := o.states.Put(ephemeral.KindOIDCState, oidcState{Nonce: nonce, Verifier: verifier, Provider: s.ID}, oidcStateTTL)
	if err != nil {
		return "", err
	}
	opts := []oauth2.AuthCodeOption{oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)}
	// Google picks the account of the domain (when there's one) at once.
	if s.Kind == "google" && len(s.Domains) == 1 {
		opts = append(opts, oauth2.SetAuthURLParam("hd", s.Domains[0]))
	}
	return cfg.AuthCodeURL(state, opts...), nil
}

// OIDCUserInfo is the verified identity returned from a callback.
type OIDCUserInfo struct {
	Provider string // which one
	Subject  string // as kept on the account: the default provider's as is, another's "<id>|<sub>"
	Email    string
	Name     string
	Username string
	Roles    []string
	Groups   []string // nil: the provider sends none (GroupClaim unset)
}

// Finish validates the callback (state, code), verifies the ID token (nonce +
// signature), checks the email's domain when the provider limits it, and
// returns the user's identity, role and group claim values.
func (o *OIDC) Finish(ctx context.Context, state, code string) (*OIDCUserInfo, error) {
	var st oidcState
	if ok, err := o.states.Take(ephemeral.KindOIDCState, state, &st); err != nil || !ok {
		return nil, ErrOIDCState
	}
	// Guard the token-exchange + JWKS-verify outbound calls (covers oauth2 too).
	ctx = oidcGuard(ctx)
	s := o.Load()
	if st.Provider != "" {
		var ok bool
		if s, ok = o.Provider(st.Provider); !ok || !s.Usable() {
			return nil, ErrOIDCDisabled
		}
	}
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

	info := &OIDCUserInfo{Provider: s.ID, Subject: idt.Subject}
	if s.ID != DefaultProviderID && s.ID != "" {
		info.Subject = s.ID + "|" + idt.Subject
	}
	info.Email, _ = claims["email"].(string)
	info.Name, _ = claims["name"].(string)
	info.Username, _ = claims["preferred_username"].(string)
	if !domainAllowed(s, info.Email, claims) {
		return nil, ErrOIDCDomain
	}
	if info.Username == "" {
		info.Username = info.Email
	}
	if info.Username == "" {
		info.Username = idt.Subject
	}
	if s.RoleClaim != "" {
		info.Roles = extractRoleClaim(claims[s.RoleClaim])
	}
	if s.GroupClaim != "" {
		info.Groups = extractRoleClaim(claims[s.GroupClaim])
		if info.Groups == nil {
			info.Groups = []string{} // none there: none here either
		}
	}
	return info, nil
}

// domainAllowed: with domains set, the email is verified and of one of them
// (and Google's hosted-domain claim agrees).
func domainAllowed(s OIDCSettings, email string, claims map[string]any) bool {
	if len(s.Domains) == 0 {
		return true
	}
	if v, _ := claims["email_verified"].(bool); !v {
		return false
	}
	_, domain, ok := strings.Cut(strings.ToLower(email), "@")
	if !ok || !slices.ContainsFunc(s.Domains, func(d string) bool { return strings.EqualFold(strings.TrimSpace(d), domain) }) {
		return false
	}
	if s.Kind == "google" {
		hd, _ := claims["hd"].(string)
		return strings.EqualFold(hd, domain)
	}
	return true
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
	Provider string `json:"provider,omitempty"`
}

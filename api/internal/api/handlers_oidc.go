package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/ephemeral"
	"github.com/joaoteixeira10/orbit/api/internal/metrics"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/planner"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const oidcSecretPlaceholder = "********"

// ssoCodeTTL bounds the window between the SSO callback redirect and the SPA
// exchanging its one-time code.
const ssoCodeTTL = 60 * time.Second

// ssoHandoff is what a one-time SSO code redeems for: the session the callback
// already opened (its refresh cookie was set on the callback response). The
// access token is minted at exchange time, so no credential sits in the store
// and tokens never travel in the redirect URL.
type ssoHandoff struct {
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
}

func ssoFail(c *gin.Context, reason string) { c.Redirect(http.StatusFound, "/login?sso_error="+reason) }

// oidcLogin redirects the browser to the configured identity provider.
//
//	@Summary	Begin OIDC login
//	@Tags		auth
//	@Param		provider	query	string	false	"Which provider (the first one if not given)"
//	@Router		/auth/oidc/login [get]
func (h *Handlers) oidcLogin(c *gin.Context) {
	url, err := h.oidc.BeginLogin(c.Request.Context(), c.Query("provider"))
	if err != nil {
		if errors.Is(err, auth.ErrOIDCDisabled) {
			ssoFail(c, "disabled")
			return
		}
		h.log.Warn("oidc begin failed", zap.Error(err))
		ssoFail(c, "unavailable")
		return
	}
	c.Redirect(http.StatusFound, url)
}

// oidcCallback completes the auth-code flow, provisions the user, and hands the
// SPA a one-time code to exchange for tokens.
//
//	@Summary	OIDC callback
//	@Tags		auth
//	@Router		/auth/oidc/callback [get]
func (h *Handlers) oidcCallback(c *gin.Context) {
	if e := c.Query("error"); e != "" {
		ssoFail(c, "idp_error")
		return
	}
	info, err := h.oidc.Finish(c.Request.Context(), c.Query("state"), c.Query("code"))
	if errors.Is(err, auth.ErrOIDCDomain) {
		ssoFail(c, "domain")
		return
	}
	if err != nil {
		h.log.Warn("oidc callback failed", zap.Error(err))
		ssoFail(c, "auth_failed")
		return
	}
	h.ssoComplete(c, info, "auth.oidc_login")
}

// ssoComplete finishes any SSO sign-in (OpenID Connect or SAML) once the
// provider vouched for the person: their account made or found, a session,
// and a one-time code the login page exchanges for tokens.
func (h *Handlers) ssoComplete(c *gin.Context, info *auth.OIDCUserInfo, action string) {
	user, err := h.provisionOIDCUser(info)
	if err != nil {
		h.log.Warn("sso provisioning failed", zap.Error(err))
		ssoFail(c, "provisioning")
		return
	}
	sid, err := h.startSession(c, &user, time.Now())
	if err != nil {
		ssoFail(c, "token")
		return
	}
	code, err := h.ephemeral.Put(ephemeral.KindSSOCode, ssoHandoff{UserID: user.ID, SessionID: sid}, ssoCodeTTL)
	if err != nil {
		ssoFail(c, "token")
		return
	}
	h.auth.RecordLoginSuccess(&user)
	h.audit.Record(user.ID, action, "user", user.ID, c.ClientIP(), gin.H{"provider": info.Provider})
	c.Redirect(http.StatusFound, "/login?sso="+code)
}

type oidcExchangeRequest struct {
	Code string `json:"code" binding:"required"`
}

// oidcExchange swaps the one-time SSO code for a token pair.
//
//	@Summary	Exchange an SSO code for tokens
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		oidcExchangeRequest	true	"One-time code"
//	@Success	200		{object}	tokenResponse
//	@Failure	401		{object}	map[string]string
//	@Router		/auth/oidc/exchange [post]
func (h *Handlers) oidcExchange(c *gin.Context) {
	var req oidcExchangeRequest
	if !bindJSON(c, &req) {
		return
	}
	var hand ssoHandoff
	var user models.User
	ok, err := h.ephemeral.Take(ephemeral.KindSSOCode, req.Code, &hand)
	if err == nil && ok {
		err = h.db.First(&user, "id = ?", hand.UserID).Error
	}
	if err != nil || !ok || user.Disabled {
		metrics.Login("oidc", "failure")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired SSO code"})
		return
	}
	resp, err := h.accessResponse(&user, hand.SessionID, time.Now(), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not issue token"})
		return
	}
	metrics.Login("oidc", "success")
	c.JSON(http.StatusOK, resp)
}

// provisionOIDCUser looks up (by OIDC subject) or just-in-time provisions the
// user, syncing profile + roles from the IdP.
func (h *Handlers) provisionOIDCUser(info *auth.OIDCUserInfo) (models.User, error) {
	if info.Subject == "" {
		return models.User{}, errors.New("provider returned no subject claim")
	}
	s, ok := h.oidc.Provider(info.Provider)
	if !ok {
		s = h.oidc.Load()
	}
	roles := h.mapOIDCRoles(info.Roles, s.DefaultRole, s.AllowAdmin)

	// Use a struct condition so GORM maps the column name (OIDCSubject →
	// o_id_c_subject) instead of a hard-coded literal.
	var u models.User
	err := h.db.Where(&models.User{OIDCSubject: info.Subject}).First(&u).Error
	// An account SCIM made before its first sign-in: found by email or username
	// (only those: never someone's own account).
	if errors.Is(err, gorm.ErrRecordNotFound) && (info.Email != "" || info.Username != "") {
		var made models.User
		if h.db.Where("scim_managed = ? AND (o_id_c_subject = '' OR o_id_c_subject IS NULL) AND ((? <> '' AND LOWER(email) = LOWER(?)) OR LOWER(username) = LOWER(?))",
			true, info.Email, info.Email, info.Username).Limit(1).Find(&made).RowsAffected == 1 {
			if h.db.Model(&made).Update("o_id_c_subject", info.Subject).Error == nil {
				u, err = made, nil
				u.OIDCSubject = info.Subject
			}
		}
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = models.User{
			Username:    uniqueUsername(h.db, info.Username, info.Subject),
			OIDCSubject: info.Subject,
			Email:       info.Email,
			DisplayName: info.Name,
		}
		if err := h.db.Create(&u).Error; err != nil {
			return u, err
		}
		_ = h.db.Model(&u).Association("Roles").Replace(roles)
	case err != nil:
		return u, err
	default:
		if u.Disabled {
			return u, errors.New("account is disabled")
		}
		h.db.Model(&u).Updates(map[string]any{"email": info.Email, "display_name": info.Name})
		if s.RoleClaim != "" { // IdP groups drive access — re-sync on each login
			_ = h.db.Model(&u).Association("Roles").Replace(roles)
		}
	}
	if u.Disabled {
		return u, errors.New("account is disabled")
	}
	if info.Groups != nil {
		h.syncIdPGroups(u.ID, s, info.Groups)
	}
	h.db.Preload("Roles").Preload("Prefs").First(&u, "id = ?", u.ID)
	return u, nil
}

// syncIdPGroups puts someone in exactly the groups their identity provider
// names (made here as needed, marked as the provider's), and out of the
// provider's others; their workspaces follow. A change that would leave a
// workspace without an owner is skipped (logged): they keep the groups
// they had.
func (h *Handlers) syncIdPGroups(uid string, p auth.OIDCSettings, names []string) {
	prefix := p.ID + ":"
	want := map[string]bool{}
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" && len(n) <= 120 {
			want[n] = true
		}
	}
	var mine []models.Group // the provider's groups they're in now
	h.db.Where("source = ? AND external_id LIKE ? AND id IN (?)", "oidc", prefix+"%",
		h.db.Model(&models.GroupMember{}).Select("group_id").Where("user_id = ?", uid)).Find(&mine)
	var wss []string
	var leave, join []string
	for _, g := range mine {
		if !want[strings.TrimPrefix(g.ExternalID, prefix)] {
			leave = append(leave, g.ID)
			wss = append(wss, planner.GroupWorkspaces(h.db, g.ID)...)
		}
		delete(want, strings.TrimPrefix(g.ExternalID, prefix))
	}
	for n := range want {
		var g models.Group
		if h.db.Where("source = ? AND external_id = ?", "oidc", prefix+n).Limit(1).Find(&g).RowsAffected == 0 {
			g = models.Group{Name: n, Source: "oidc", ExternalID: prefix + n}
			var taken int64
			h.db.Model(&models.Group{}).Where("name = ?", n).Count(&taken)
			if taken > 0 { // a group of that name made here: this one says where it's from
				g.Name = n + " (" + p.Name + ")"
			}
			if err := h.db.Create(&g).Error; err != nil {
				h.log.Warn("sso group", zap.String("group", n), zap.Error(err))
				continue
			}
		}
		join = append(join, g.ID)
		wss = append(wss, planner.GroupWorkspaces(h.db, g.ID)...)
	}
	if len(leave)+len(join) == 0 {
		return
	}
	changed, err := h.regroup(wss, func(tx *gorm.DB) error {
		if len(leave) > 0 {
			if err := tx.Where("user_id = ? AND group_id IN ?", uid, leave).Delete(&models.GroupMember{}).Error; err != nil {
				return err
			}
		}
		for _, gid := range join {
			if err := tx.Create(&models.GroupMember{GroupID: gid, UserID: uid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		h.log.Warn("sso groups not changed", zap.String("user", uid), zap.Error(err))
		return
	}
	h.regrouped(append(changed, uid))
}

// mapOIDCRoles resolves IdP role-claim values to RBAC roles, falling back to the
// configured default role (or the no-access "none" role).
func (h *Handlers) mapOIDCRoles(claimRoles []string, defaultRole string, allowAdmin bool) []models.Role {
	var roles []models.Role
	if len(claimRoles) > 0 {
		h.db.Where("name IN ?", claimRoles).Find(&roles)
	}
	// Admin (superuser) roles come from the identity provider only when the
	// owner allowed it; otherwise a group called "admin" can't make an admin.
	keep := roles[:0]
	for _, r := range roles {
		if r.Superuser && !allowAdmin {
			continue
		}
		keep = append(keep, r)
	}
	roles = keep
	if len(roles) == 0 {
		name := defaultRole
		if name == "" {
			name = string(models.RoleNone)
		}
		var r models.Role
		if h.db.Where("name = ?", name).First(&r).Error == nil && (!r.Superuser || allowAdmin) {
			roles = []models.Role{r}
		} else {
			var none models.Role // a fresh row: GORM would add r's id to the query
			if h.db.Where("name = ?", string(models.RoleNone)).First(&none).Error == nil {
				roles = []models.Role{none}
			}
		}
	}
	return roles
}

// uniqueUsername returns want if free, else a subject-suffixed variant.
func uniqueUsername(db *gorm.DB, want, subject string) string {
	if want == "" {
		want = "sso-user"
	}
	var n int64
	db.Model(&models.User{}).Where("username = ?", want).Count(&n)
	if n == 0 {
		return want
	}
	suffix := subject
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return want + "-" + suffix
}

// ---- admin config ---------------------------------------------------------

type oidcConfigDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"` // oidc | google | saml
	Enabled      bool     `json:"enabled"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"` // write-only; placeholder/empty = unchanged
	RedirectURL  string   `json:"redirect_url"`
	Scopes       string   `json:"scopes"`
	RoleClaim    string   `json:"role_claim"`
	DefaultRole  string   `json:"default_role"`
	AllowAdmin   bool     `json:"allow_admin"`  // the IdP may grant admin (superuser) roles
	GroupClaim   string   `json:"group_claim"`  // the IdP's groups, kept as Orbit groups
	Domains      []string `json:"domains"`      // only verified emails of these domains
	MetadataXML  string   `json:"metadata_xml"` // SAML: the IdP's metadata, pasted (else from Issuer, its URL)
	HasSecret    bool     `json:"has_secret"`   // response only
}

func toOIDCDTO(s auth.OIDCSettings) oidcConfigDTO {
	dto := oidcConfigDTO{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Enabled: s.Enabled, Issuer: s.Issuer, ClientID: s.ClientID,
		RedirectURL: s.RedirectURL, Scopes: s.Scopes, RoleClaim: s.RoleClaim, DefaultRole: s.DefaultRole,
		AllowAdmin: s.AllowAdmin, GroupClaim: s.GroupClaim, Domains: s.Domains, MetadataXML: s.MetadataXML, HasSecret: len(s.ClientSecretSealed) > 0,
	}
	if dto.Domains == nil {
		dto.Domains = []string{}
	}
	if dto.HasSecret {
		dto.ClientSecret = oidcSecretPlaceholder
	}
	return dto
}

// listOIDCProviders lists the sign-in providers (without their secrets).
//
//	@Summary	SSO providers
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{array}	oidcConfigDTO
//	@Router		/auth/oidc/providers [get]
func (h *Handlers) listOIDCProviders(c *gin.Context) {
	out := []oidcConfigDTO{}
	for _, p := range h.oidc.Providers() {
		out = append(out, toOIDCDTO(p))
	}
	c.JSON(http.StatusOK, out)
}

// getOIDCConfig returns the first provider's settings (the API from before
// there could be several).
//
//	@Summary	Get OIDC config (the first provider)
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	oidcConfigDTO
//	@Router		/auth/oidc/config [get]
func (h *Handlers) getOIDCConfig(c *gin.Context) {
	s, ok := h.oidc.Provider(auth.DefaultProviderID)
	if !ok {
		s = auth.OIDCSettings{ID: auth.DefaultProviderID, Name: "SSO", Kind: "oidc"}
	}
	c.JSON(http.StatusOK, toOIDCDTO(s))
}

// updateOIDCConfig saves the default provider (the API from before).
//
//	@Summary	Update OIDC config (the default provider)
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		oidcConfigDTO	true	"OIDC settings"
//	@Success	200		{object}	oidcConfigDTO
//	@Router		/auth/oidc/config [put]
func (h *Handlers) updateOIDCConfig(c *gin.Context) {
	c.Params = append(c.Params, gin.Param{Key: "id", Value: auth.DefaultProviderID})
	h.saveOIDCProvider(c)
}

// saveOIDCProvider adds or changes a sign-in provider (by its id); the client
// secret is sealed and kept when the UI leaves it as the placeholder.
//
//	@Summary	Add or change an SSO provider
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string			true	"Provider id (a-z, 0-9, -)"
//	@Param		body	body		oidcConfigDTO	true	"Its settings"
//	@Success	200		{object}	oidcConfigDTO
//	@Router		/auth/oidc/providers/{id} [put]
func (h *Handlers) saveOIDCProvider(c *gin.Context) {
	id := c.Param("id")
	if !auth.ValidProviderID.MatchString(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a provider's id: lowercase letters, digits and -"})
		return
	}
	var dto oidcConfigDTO
	if !bindJSON(c, &dto) {
		return
	}
	list := h.oidc.Providers()
	at := -1
	for i, p := range list {
		if p.ID == id {
			at = i
		}
	}
	var cur auth.OIDCSettings
	if at >= 0 {
		cur = list[at]
	}
	kind := dto.Kind
	if kind != "google" && kind != "saml" {
		kind = "oidc"
	}
	if len(dto.MetadataXML) > 1<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the metadata is too large (1 MB at most)"})
		return
	}
	name := strings.TrimSpace(dto.Name)
	if name == "" {
		name = map[string]string{"google": "Google", "oidc": "SSO", "saml": "SAML"}[kind]
	}
	issuer := strings.TrimSpace(dto.Issuer)
	if kind == "google" {
		issuer = auth.GoogleIssuer
	}
	var domains []string
	for _, d := range dto.Domains {
		if d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@")); d != "" {
			domains = append(domains, d)
		}
	}
	s := auth.OIDCSettings{
		ID: id, Name: name, Kind: kind, Enabled: dto.Enabled, Issuer: issuer, ClientID: strings.TrimSpace(dto.ClientID),
		RedirectURL: strings.TrimSpace(dto.RedirectURL), Scopes: dto.Scopes,
		RoleClaim: strings.TrimSpace(dto.RoleClaim), DefaultRole: strings.TrimSpace(dto.DefaultRole),
		AllowAdmin: dto.AllowAdmin, GroupClaim: strings.TrimSpace(dto.GroupClaim), Domains: domains,
		ClientSecretSealed: cur.ClientSecretSealed,
	}
	if kind == "saml" {
		s.MetadataXML = strings.TrimSpace(dto.MetadataXML)
		s.ClientID, s.ClientSecretSealed, s.Scopes = "", nil, ""
	}
	if dto.ClientSecret != "" && dto.ClientSecret != oidcSecretPlaceholder {
		sealed, err := h.oidc.SealSecret(dto.ClientSecret)
		if err != nil {
			h.internalError(c, err, "could not store client secret")
			return
		}
		s.ClientSecretSealed = sealed
	}
	if at >= 0 {
		list[at] = s
	} else {
		list = append(list, s)
	}
	if err := h.oidc.SaveProviders(list); err != nil {
		h.internalError(c, err, "could not save the SSO provider")
		return
	}
	if !h.oidc.Enabled() { // a disabled method can't keep its auto-login on
		_ = h.setBoolSetting(settingAutoSSO, false)
	}
	h.recordCtx(c, "auth.oidc_config", "settings", "oidc:"+id, gin.H{"name": name, "kind": kind, "enabled": s.Enabled})
	h.securityAlert(c, "config_change", "SSO/OIDC configuration changed")
	c.JSON(http.StatusOK, toOIDCDTO(s))
}

// deleteOIDCProvider removes a sign-in provider. Its people's accounts stay
// (signing in another way, if they have one).
//
//	@Summary	Remove an SSO provider
//	@Tags		auth
//	@Security	BearerAuth
//	@Param		id	path	string	true	"Provider id"
//	@Success	204
//	@Router		/auth/oidc/providers/{id} [delete]
func (h *Handlers) deleteOIDCProvider(c *gin.Context) {
	id := c.Param("id")
	list := h.oidc.Providers()
	kept := list[:0]
	for _, p := range list {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(list) {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such provider"})
		return
	}
	if err := h.oidc.SaveProviders(kept); err != nil {
		h.internalError(c, err, "could not remove the SSO provider")
		return
	}
	if !h.oidc.Enabled() {
		_ = h.setBoolSetting(settingAutoSSO, false)
	}
	h.recordCtx(c, "auth.oidc_config", "settings", "oidc:"+id, gin.H{"removed": true})
	h.securityAlert(c, "config_change", "SSO/OIDC configuration changed")
	c.Status(http.StatusNoContent)
}

type oidcTestRequest struct {
	Issuer string `json:"issuer" binding:"required"`
}

// testOIDCDiscovery checks that an issuer is a reachable OIDC provider.
//
//	@Summary	Test OIDC discovery
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body	oidcTestRequest	true	"Issuer URL"
//	@Success	200	{object}	map[string]string
//	@Failure	400	{object}	map[string]string
//	@Router		/auth/oidc/test [post]
func (h *Handlers) testOIDCDiscovery(c *gin.Context) {
	var req oidcTestRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.oidc.TestDiscovery(c.Request.Context(), strings.TrimSpace(req.Issuer)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "discovery failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

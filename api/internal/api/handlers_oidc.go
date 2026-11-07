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
//	@Router		/auth/oidc/login [get]
func (h *Handlers) oidcLogin(c *gin.Context) {
	url, err := h.oidc.BeginLogin(c.Request.Context())
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
	if err != nil {
		h.log.Warn("oidc callback failed", zap.Error(err))
		ssoFail(c, "auth_failed")
		return
	}
	user, err := h.provisionOIDCUser(info)
	if err != nil {
		h.log.Warn("oidc provisioning failed", zap.Error(err))
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
	h.audit.Record(user.ID, "auth.oidc_login", "user", user.ID, c.ClientIP(), nil)
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
	s := h.oidc.Load()
	roles := h.mapOIDCRoles(info.Roles, s.DefaultRole, s.AllowAdmin)

	// Use a struct condition so GORM maps the column name (OIDCSubject →
	// o_id_c_subject) instead of a hard-coded literal.
	var u models.User
	err := h.db.Where(&models.User{OIDCSubject: info.Subject}).First(&u).Error
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
	h.db.Preload("Roles").Preload("Prefs").First(&u, "id = ?", u.ID)
	return u, nil
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
	Enabled      bool   `json:"enabled"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"` // write-only; placeholder/empty = unchanged
	RedirectURL  string `json:"redirect_url"`
	Scopes       string `json:"scopes"`
	RoleClaim    string `json:"role_claim"`
	DefaultRole  string `json:"default_role"`
	AllowAdmin   bool   `json:"allow_admin"` // the IdP may grant admin (superuser) roles
	HasSecret    bool   `json:"has_secret"`  // response only
}

// getOIDCConfig returns the SSO settings (without the client secret).
//
//	@Summary	Get OIDC config
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	oidcConfigDTO
//	@Router		/auth/oidc/config [get]
func (h *Handlers) getOIDCConfig(c *gin.Context) {
	s := h.oidc.Load()
	dto := oidcConfigDTO{
		Enabled: s.Enabled, Issuer: s.Issuer, ClientID: s.ClientID,
		RedirectURL: s.RedirectURL, Scopes: s.Scopes, RoleClaim: s.RoleClaim,
		DefaultRole: s.DefaultRole, AllowAdmin: s.AllowAdmin, HasSecret: len(s.ClientSecretSealed) > 0,
	}
	if dto.HasSecret {
		dto.ClientSecret = oidcSecretPlaceholder
	}
	c.JSON(http.StatusOK, dto)
}

// updateOIDCConfig saves the SSO settings; the client secret is sealed and
// preserved when the UI leaves it as the placeholder.
//
//	@Summary	Update OIDC config
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		oidcConfigDTO	true	"OIDC settings"
//	@Success	200		{object}	oidcConfigDTO
//	@Router		/auth/oidc/config [put]
func (h *Handlers) updateOIDCConfig(c *gin.Context) {
	var dto oidcConfigDTO
	if !bindJSON(c, &dto) {
		return
	}
	cur := h.oidc.Load()
	s := auth.OIDCSettings{
		Enabled: dto.Enabled, Issuer: strings.TrimSpace(dto.Issuer), ClientID: strings.TrimSpace(dto.ClientID),
		RedirectURL: strings.TrimSpace(dto.RedirectURL), Scopes: dto.Scopes,
		RoleClaim: strings.TrimSpace(dto.RoleClaim), DefaultRole: strings.TrimSpace(dto.DefaultRole),
		AllowAdmin: dto.AllowAdmin, ClientSecretSealed: cur.ClientSecretSealed,
	}
	if dto.ClientSecret != "" && dto.ClientSecret != oidcSecretPlaceholder {
		sealed, err := h.oidc.SealSecret(dto.ClientSecret)
		if err != nil {
			h.internalError(c, err, "could not store client secret")
			return
		}
		s.ClientSecretSealed = sealed
	}
	if err := h.oidc.Save(s); err != nil {
		h.internalError(c, err, "could not save OIDC config")
		return
	}
	if !h.oidc.Enabled() { // a disabled method can't keep its auto-login on
		_ = h.setBoolSetting(settingAutoSSO, false)
	}
	h.recordCtx(c, "auth.oidc_config", "settings", "oidc", nil)
	h.securityAlert(c, "config_change", "SSO/OIDC configuration changed")
	h.getOIDCConfig(c)
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

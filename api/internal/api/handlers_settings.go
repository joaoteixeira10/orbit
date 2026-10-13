package api

import (
	"net/http"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

// Branding setting keys.
const (
	keyAppName        = "app_name"
	keyAccentColor    = "accent_color"
	keyLogoDataURL    = "logo_data_url"
	keyFaviconDataURL = "favicon_data_url"
	keyShowAppName    = "show_app_name"
	keyShowLogo       = "show_logo"
	keyLoginMessage   = "login_message"

	// Developer themes: a closed set of curated themes (accent + UBonito proxy
	// crest + "UBonito" wordmark); per asset, the theme's or the org's own.
	keyDevThemesEnabled = "developer_themes_enabled"
	keyDevTheme         = "developer_theme"       // blue | gold | red | ink
	keyAssetSrcLogo     = "asset_source_logo"     // theme | keep
	keyAssetSrcWordmark = "asset_source_wordmark" // theme | keep
	keyAssetSrcFavicon  = "asset_source_favicon"  // theme | keep
)

// validDevTheme / validAssetSource clamp stored values to the closed sets.
func validDevTheme(s string) string {
	switch s {
	case "blue", "gold", "red", "ink":
		return s
	}
	return "blue"
}

func validAssetSource(s string) string {
	if s == "keep" {
		return "keep"
	}
	return "theme"
}

// brandingResponse is the public theming payload the SPA loads at boot.
type brandingResponse struct {
	AppName             string `json:"app_name"`
	AccentColor         string `json:"accent_color"`
	LogoDataURL         string `json:"logo_data_url"`
	FaviconDataURL      string `json:"favicon_data_url"`
	ShowAppName         bool   `json:"show_app_name"`
	ShowLogo            bool   `json:"show_logo"` // show the sidebar/login logo mark
	LoginMessage        string `json:"login_message"`
	OIDCEnabled         bool   `json:"oidc_enabled"`         // SPA shows the "Sign in with SSO" button
	HideLocalLogin      bool   `json:"hide_local_login"`     // hide the password form on /login (when SSO is available)
	PasswordlessEnabled bool   `json:"passwordless_enabled"` // SPA shows the "Sign in with a passkey" button
	AutoSSO             bool   `json:"auto_sso"`             // login page auto-starts SSO (unless ?local=1)

	DeveloperThemesEnabled bool   `json:"developer_themes_enabled"`
	DeveloperTheme         string `json:"developer_theme"`       // blue | gold | red | ink
	AssetSourceLogo        string `json:"asset_source_logo"`     // theme | keep
	AssetSourceWordmark    string `json:"asset_source_wordmark"` // theme | keep
	AssetSourceFavicon     string `json:"asset_source_favicon"`  // theme | keep
}

type updateSettingsRequest struct {
	AppName        *string `json:"app_name"`
	AccentColor    *string `json:"accent_color"`
	LogoDataURL    *string `json:"logo_data_url"`
	FaviconDataURL *string `json:"favicon_data_url"`
	ShowAppName    *bool   `json:"show_app_name"`
	ShowLogo       *bool   `json:"show_logo"`
	LoginMessage   *string `json:"login_message"`

	DeveloperThemesEnabled *bool   `json:"developer_themes_enabled"`
	DeveloperTheme         *string `json:"developer_theme"`
	AssetSourceLogo        *string `json:"asset_source_logo"`
	AssetSourceWordmark    *string `json:"asset_source_wordmark"`
	AssetSourceFavicon     *string `json:"asset_source_favicon"`
}

// getBranding returns public branding (no auth) so the SPA can theme at boot.
//
//	@Summary	Get branding
//	@Tags		settings
//	@Produce	json
//	@Success	200	{object}	brandingResponse
//	@Router		/branding [get]
func (h *Handlers) getBranding(c *gin.Context) {
	m := h.loadSettings()
	appName := m[keyAppName]
	if appName == "" {
		appName = "Orbit"
	}
	// Show the app name + logo mark unless explicitly disabled.
	showAppName := m[keyShowAppName] != "false"
	showLogo := m[keyShowLogo] != "false"
	c.JSON(http.StatusOK, brandingResponse{
		AppName:             appName,
		AccentColor:         m[keyAccentColor],
		LogoDataURL:         m[keyLogoDataURL],
		FaviconDataURL:      m[keyFaviconDataURL],
		ShowAppName:         showAppName,
		ShowLogo:            showLogo,
		LoginMessage:        m[keyLoginMessage],
		OIDCEnabled:         h.oidc != nil && h.oidc.Enabled(),
		HideLocalLogin:      h.hideLocalLogin(),
		PasswordlessEnabled: h.passwordlessEnabled(),
		AutoSSO:             h.autoSSO(),

		DeveloperThemesEnabled: m[keyDevThemesEnabled] == "true",
		DeveloperTheme:         validDevTheme(m[keyDevTheme]),
		AssetSourceLogo:        validAssetSource(m[keyAssetSrcLogo]),
		AssetSourceWordmark:    validAssetSource(m[keyAssetSrcWordmark]),
		AssetSourceFavicon:     validAssetSource(m[keyAssetSrcFavicon]),
	})
}

// updateSettings patches branding (admin only).
//
//	@Summary	Update settings/branding
//	@Tags		settings
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		updateSettingsRequest	true	"Branding"
//	@Success	200		{object}	brandingResponse
//	@Router		/settings [put]
func (h *Handlers) updateSettings(c *gin.Context) {
	var req updateSettingsRequest
	if !bindJSON(c, &req) {
		return
	}
	// Validate the free-form fields before persisting.
	if req.AccentColor != nil && !validHexColor(*req.AccentColor) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accent_color must be a #rrggbb hex colour"})
		return
	}
	if req.LogoDataURL != nil && !validImageDataURL(*req.LogoDataURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo must be an image data URL"})
		return
	}
	if req.FaviconDataURL != nil && !validImageDataURL(*req.FaviconDataURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "favicon must be an image data URL"})
		return
	}
	if req.AppName != nil {
		h.setSetting(keyAppName, *req.AppName)
	}
	if req.AccentColor != nil {
		h.setSetting(keyAccentColor, *req.AccentColor)
	}
	if req.LogoDataURL != nil {
		h.setSetting(keyLogoDataURL, *req.LogoDataURL)
	}
	if req.FaviconDataURL != nil {
		h.setSetting(keyFaviconDataURL, *req.FaviconDataURL)
	}
	if req.ShowAppName != nil {
		v := "true"
		if !*req.ShowAppName {
			v = "false"
		}
		h.setSetting(keyShowAppName, v)
	}
	if req.ShowLogo != nil {
		v := "true"
		if !*req.ShowLogo {
			v = "false"
		}
		h.setSetting(keyShowLogo, v)
	}
	if req.LoginMessage != nil {
		h.setSetting(keyLoginMessage, *req.LoginMessage)
	}
	if req.DeveloperThemesEnabled != nil {
		v := "false"
		if *req.DeveloperThemesEnabled {
			v = "true"
		}
		h.setSetting(keyDevThemesEnabled, v)
	}
	if req.DeveloperTheme != nil {
		h.setSetting(keyDevTheme, validDevTheme(*req.DeveloperTheme))
	}
	if req.AssetSourceLogo != nil {
		h.setSetting(keyAssetSrcLogo, validAssetSource(*req.AssetSourceLogo))
	}
	if req.AssetSourceWordmark != nil {
		h.setSetting(keyAssetSrcWordmark, validAssetSource(*req.AssetSourceWordmark))
	}
	if req.AssetSourceFavicon != nil {
		h.setSetting(keyAssetSrcFavicon, validAssetSource(*req.AssetSourceFavicon))
	}
	h.recordCtx(c, "settings.update", "settings", "branding", nil)
	h.getBranding(c)
}

func (h *Handlers) loadSettings() map[string]string {
	var rows []models.Setting
	h.db.Find(&rows)
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	return m
}

func (h *Handlers) setSetting(key, value string) {
	h.db.Save(&models.Setting{Key: key, Value: value, UpdatedAt: time.Now()})
}

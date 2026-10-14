package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestDeveloperThemes: off by default; the owner turns them on and picks a
// theme and, per asset, the theme's or their own; stored values are clamped
// to the closed sets; the public branding carries them (the sign-in page
// themes itself before sign-in).
func TestDeveloperThemes(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	var b brandingResponse
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.DeveloperThemesEnabled || b.DeveloperTheme != "blue" || b.AssetSourceLogo != "theme" || b.AssetSourceWordmark != "theme" || b.AssetSourceFavicon != "theme" {
		t.Fatalf("defaults: %+v", b)
	}
	owner := loginTok(t, r, "admin", "adminpass")
	body := `{"developer_themes_enabled":true,"developer_theme":"ink","asset_source_logo":"keep","asset_source_favicon":"bogus"}`
	if w := authdo(r, "PUT", "/api/v1/settings", body, owner); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if !b.DeveloperThemesEnabled || b.DeveloperTheme != "ink" || b.AssetSourceLogo != "keep" || b.AssetSourceFavicon != "theme" {
		t.Errorf("after save: %+v", b)
	}
	authdo(r, "PUT", "/api/v1/settings", `{"developer_theme":"purple"}`, owner)
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.DeveloperTheme != "blue" {
		t.Errorf("an unknown theme is clamped to blue: %q", b.DeveloperTheme)
	}
}

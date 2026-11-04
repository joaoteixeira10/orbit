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

// TestDeveloperThemesGreenAndPersonal: Forest Green is a theme; the owner can
// let each person choose, and a person's choice is a checked preference.
func TestDeveloperThemesGreenAndPersonal(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	authdo(r, "PUT", "/api/v1/settings", `{"developer_theme":"green","developer_themes_personal":true}`, owner)
	var b brandingResponse
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.DeveloperTheme != "green" || !b.DeveloperThemesPersonal {
		t.Fatalf("branding: %+v", b)
	}
	for _, v := range []string{"", "none", "gold", "green"} {
		if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"dev_theme":"`+v+`"}`, owner); w.Code != http.StatusOK {
			t.Errorf("dev_theme %q: %d %s", v, w.Code, w.Body.String())
		}
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"dev_theme":"purple"}`, owner); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown theme was accepted: %d", w.Code)
	}
}

// TestHolidayTheme: off by default; the owner picks auto or one holiday, in
// the public branding for everyone; anything else is refused.
func TestHolidayTheme(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	var b brandingResponse
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.HolidayTheme != "off" {
		t.Fatalf("default: %q", b.HolidayTheme)
	}
	owner := loginTok(t, r, "admin", "adminpass")
	for _, v := range []string{"auto", "christmas", "lunar", "off"} {
		if w := authdo(r, "PUT", "/api/v1/settings", `{"holiday_theme":"`+v+`"}`, owner); w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", v, w.Code, w.Body.String())
		}
		_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
		if b.HolidayTheme != v {
			t.Errorf("after %s: %q", v, b.HolidayTheme)
		}
	}
	if w := authdo(r, "PUT", "/api/v1/settings", `{"holiday_theme":"diwali"}`, owner); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown holiday was accepted: %d", w.Code)
	}
}

// TestHolidayEffects: each person's effects are on until they turn them off.
func TestHolidayEffects(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p prefsResponse
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if !p.HolidayEffects {
		t.Fatal("effects are on by default")
	}
	authdo(r, "PUT", "/api/v1/me/prefs", `{"holiday_effects":false}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.HolidayEffects {
		t.Error("effects stayed on")
	}
	authdo(r, "PUT", "/api/v1/me/prefs", `{"holiday_effects":true}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if !p.HolidayEffects {
		t.Error("effects stayed off")
	}
}

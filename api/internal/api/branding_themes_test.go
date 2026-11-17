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
	// The signed-in user carries it too (the app reads it from there on a reload).
	var me struct {
		User struct {
			Prefs struct {
				HolidayEffects bool `json:"holiday_effects"`
			} `json:"prefs"`
		} `json:"user"`
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/auth/me", "", owner).Body.Bytes(), &me)
	if me.User.Prefs.HolidayEffects {
		t.Error("me: effects still on")
	}
	authdo(r, "PUT", "/api/v1/me/prefs", `{"holiday_effects":true}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if !p.HolidayEffects {
		t.Error("effects stayed off")
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/auth/me", "", owner).Body.Bytes(), &me)
	if !me.User.Prefs.HolidayEffects {
		t.Error("me: effects still off")
	}
}

// TestSeasonTheme: off by default; the owner picks auto or one season, in
// the public branding for everyone; anything else is refused.
func TestSeasonTheme(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	var b brandingResponse
	_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
	if b.SeasonTheme != "off" {
		t.Fatalf("default: %q", b.SeasonTheme)
	}
	owner := loginTok(t, r, "admin", "adminpass")
	for _, v := range []string{"auto", "spring", "summer", "autumn", "winter", "off"} {
		if w := authdo(r, "PUT", "/api/v1/settings", `{"season_theme":"`+v+`"}`, owner); w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", v, w.Code, w.Body.String())
		}
		_ = json.Unmarshal(do(r, "GET", "/api/v1/branding", "").Body.Bytes(), &b)
		if b.SeasonTheme != v {
			t.Errorf("after %s: %q", v, b.SeasonTheme)
		}
	}
	if w := authdo(r, "PUT", "/api/v1/settings", `{"season_theme":"monsoon"}`, owner); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown season was accepted: %d", w.Code)
	}
}

// TestGlass: solid (0) by default; each person sets 0–100, and it follows
// them (the signed-in user carries it); out of range is refused.
func TestGlass(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p prefsResponse
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.Glass != 0 {
		t.Fatalf("default: %d", p.Glass)
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"glass":65}`, owner); w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	var me struct {
		User struct {
			Prefs struct {
				Glass int `json:"glass"`
			} `json:"prefs"`
		} `json:"user"`
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/auth/me", "", owner).Body.Bytes(), &me)
	if me.User.Prefs.Glass != 65 {
		t.Errorf("me: %d", me.User.Prefs.Glass)
	}
	for _, bad := range []string{`{"glass":101}`, `{"glass":-1}`} {
		if w := authdo(r, "PUT", "/api/v1/me/prefs", bad, owner); w.Code != http.StatusBadRequest {
			t.Errorf("%s accepted: %d", bad, w.Code)
		}
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"glass":0}`, owner); w.Code != http.StatusOK {
		t.Fatalf("back to solid: %d", w.Code)
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.Glass != 0 {
		t.Errorf("solid again: %d", p.Glass)
	}
}

// The app's animations: on by default, off and on again, per person.
func TestAnimations(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var p prefsResponse
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if !p.Animations {
		t.Fatal("animations should be on by default")
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"animations":false}`, owner); w.Code != http.StatusOK {
		t.Fatalf("off: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.Animations {
		t.Error("still on after turning off")
	}
	// Another setting leaves it as it is.
	authdo(r, "PUT", "/api/v1/me/prefs", `{"glass":40}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.Animations {
		t.Error("turned back on by another setting")
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"animations":true}`, owner); w.Code != http.StatusOK {
		t.Fatalf("on: %d", w.Code)
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if !p.Animations {
		t.Error("not on again")
	}
}

// TestPersonalHolidayThemes: the owner lets each person choose; a person
// picks their own holiday and season themes ("" = as the instance), and
// nothing outside the closed sets is kept.
func TestPersonalHolidayThemes(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	var b struct {
		Holidays bool `json:"holiday_themes_personal"`
		Seasons  bool `json:"season_themes_personal"`
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/branding", "", owner).Body.Bytes(), &b)
	if b.Holidays || b.Seasons {
		t.Fatal("each person's choice should be off by default")
	}
	// Each on its own.
	if w := authdo(r, "PUT", "/api/v1/settings", `{"season_themes_personal":true}`, owner); w.Code != http.StatusOK {
		t.Fatalf("allow seasons: %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/branding", "", owner).Body.Bytes(), &b)
	if b.Holidays || !b.Seasons {
		t.Fatalf("seasons only: %+v", b)
	}
	authdo(r, "PUT", "/api/v1/settings", `{"holiday_themes_personal":true}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/branding", "", owner).Body.Bytes(), &b)
	if !b.Holidays || !b.Seasons {
		t.Fatalf("both: %+v", b)
	}
	if w := authdo(r, "PUT", "/api/v1/me/prefs", `{"holiday_theme":"halloween","season_theme":"off"}`, owner); w.Code != http.StatusOK {
		t.Fatalf("pick: %d %s", w.Code, w.Body.String())
	}
	var p prefsResponse
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.HolidayTheme != "halloween" || p.SeasonTheme != "off" {
		t.Errorf("kept: %q %q", p.HolidayTheme, p.SeasonTheme)
	}
	for _, body := range []string{`{"holiday_theme":"diwali"}`, `{"season_theme":"monsoon"}`} {
		if w := authdo(r, "PUT", "/api/v1/me/prefs", body, owner); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	authdo(r, "PUT", "/api/v1/me/prefs", `{"holiday_theme":"","season_theme":""}`, owner)
	_ = json.Unmarshal(authdo(r, "GET", "/api/v1/me/prefs", "", owner).Body.Bytes(), &p)
	if p.HolidayTheme != "" || p.SeasonTheme != "" {
		t.Errorf("back to the instance's: %q %q", p.HolidayTheme, p.SeasonTheme)
	}
}

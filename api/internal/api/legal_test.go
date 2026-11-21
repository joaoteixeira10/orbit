package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLegalPages: the privacy policy is always there (the owner's text, or
// "" for the built-in one) and the terms only once written; both are public,
// and only an owner changes them.
func TestLegalPages(t *testing.T) {
	r, _ := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	public := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		return w
	}
	var doc struct {
		AppName string `json:"app_name"`
		Text    string `json:"text"`
	}
	var b struct {
		HasTerms bool `json:"has_terms"`
	}
	w := public("/api/v1/legal/privacy")
	expect(t, w, http.StatusOK, "privacy, built in")
	decode(t, w, &doc)
	if doc.Text != "" || doc.AppName == "" {
		t.Errorf("built-in privacy: %#v", doc)
	}
	expect(t, public("/api/v1/legal/terms"), http.StatusNotFound, "no terms yet")
	expect(t, public("/api/v1/legal/other"), http.StatusNotFound, "unknown")

	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"privacy_policy":"  # Ours\n","terms_of_service":"Be kind."}`, owner), http.StatusOK, "save")
	w = public("/api/v1/legal/privacy")
	decode(t, w, &doc)
	if doc.Text != "# Ours" {
		t.Errorf("own privacy: %q", doc.Text)
	}
	expect(t, public("/api/v1/legal/terms"), http.StatusOK, "terms written")
	decode(t, public("/api/v1/branding"), &b)
	if !b.HasTerms {
		t.Error("branding should say there are terms")
	}
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"terms_of_service":" "}`, owner), http.StatusOK, "clear terms")
	expect(t, public("/api/v1/legal/terms"), http.StatusNotFound, "terms gone")
}

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// TestSites: a workspace's owners publish some of its pages as a website —
// those pages and what's under them, never a restricted, locked or trashed
// one — open to anyone, searchable, listed by search engines only when
// they say so, and on a domain of its own serving only the site.
func TestSites(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	ws := createWS(t, r, owner, `{"name":"Équipe Docs","kind":"work","starter":false,"modules":["home","pages"]}`)
	base := "/api/v1/workspaces/" + ws.ID
	page := func(body string) string {
		w := authdo(r, "POST", base+"/pages", body, owner)
		expect(t, w, http.StatusCreated, "page")
		var p struct {
			ID string `json:"id"`
		}
		decode(t, w, &p)
		return p.ID
	}
	guide := page(`{"title":"Guide","content":"Welcome to the guide."}`)
	install := page(`{"title":"Install","content":"Run the installer with docker compose.","parent_id":"` + guide + `"}`)
	secret := page(`{"title":"Salaries","content":"Private numbers.","parent_id":"` + guide + `"}`)
	under := page(`{"title":"Under salaries","content":"Also private.","parent_id":"` + secret + `"}`)
	internal := page(`{"title":"Internal","content":"Not published."}`)
	db.Model(&models.Page{}).Where("id = ?", secret).Update("restricted", true)

	public := func(method, path, host string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(method, path, nil)
		if host != "" {
			req.Host = host
		}
		r.ServeHTTP(w, req)
		return w
	}

	var got struct {
		Site          *siteResponse `json:"site"`
		SuggestedSlug string        `json:"suggested_slug"`
	}
	decode(t, authdo(r, "GET", base+"/site", "", owner), &got)
	if got.Site != nil || got.SuggestedSlug != "equipe-docs" {
		t.Fatalf("before: %+v", got)
	}
	expect(t, authdo(r, "PUT", base+"/site", `{"slug":"Bad Name!"}`, owner), http.StatusBadRequest, "bad name")
	expect(t, authdo(r, "PUT", base+"/site", `{"page_ids":["nope"]}`, owner), http.StatusBadRequest, "not its page")
	w := authdo(r, "PUT", base+"/site", `{"published":true,"page_ids":["`+guide+`"]}`, owner)
	expect(t, w, http.StatusOK, "publish")
	var s siteResponse
	decode(t, w, &s)
	if s.Slug != "equipe-docs" || s.Path != "/site/equipe-docs" || !s.Published || s.Indexed {
		t.Fatalf("site: %+v", s)
	}

	// Anyone: the picked page and what's under it, not the restricted one
	// nor what's under that, nor the page that wasn't picked.
	w = public("GET", "/api/v1/site/equipe-docs", "")
	expect(t, w, http.StatusOK, "open")
	if w.Header().Get("X-Robots-Tag") == "" {
		t.Error("not listed: search engines should be told")
	}
	var open struct {
		Title string      `json:"title"`
		Home  string      `json:"home"`
		Pages []guestPage `json:"pages"`
	}
	decode(t, w, &open)
	titles := []string{}
	for _, p := range open.Pages {
		titles = append(titles, p.Title)
	}
	if open.Title != "Équipe Docs" || open.Home != guide || strings.Join(titles, ",") != "Guide,Install" {
		t.Fatalf("open: %q home %s pages %v", open.Title, open.Home, titles)
	}
	expect(t, public("GET", "/api/v1/site/equipe-docs/pages/"+install, ""), http.StatusOK, "a page under it")
	for _, id := range []string{secret, under, internal} {
		expect(t, public("GET", "/api/v1/site/equipe-docs/pages/"+id, ""), http.StatusNotFound, "not on the site")
	}
	var hits []siteHit
	decode(t, public("GET", "/api/v1/site/equipe-docs/search?q=docker", ""), &hits)
	if len(hits) != 1 || hits[0].ID != install || !strings.Contains(hits[0].Snippet, "docker") {
		t.Fatalf("search: %+v", hits)
	}
	decode(t, public("GET", "/api/v1/site/equipe-docs/search?q=private", ""), &hits)
	if len(hits) != 0 {
		t.Fatalf("search found what isn't on the site: %+v", hits)
	}
	expect(t, public("GET", "/site/equipe-docs/sitemap.xml", ""), http.StatusNotFound, "no sitemap unless listed")

	// Listed: a sitemap.
	expect(t, authdo(r, "PUT", base+"/site", `{"indexed":true}`, owner), http.StatusOK, "listed")
	w = public("GET", "/site/equipe-docs/sitemap.xml", "")
	expect(t, w, http.StatusOK, "sitemap")
	if body := w.Body.String(); !strings.Contains(body, "/site/equipe-docs/p/"+install) || strings.Contains(body, secret) {
		t.Fatalf("sitemap: %s", body)
	}

	// Its own domain: only the site there.
	own := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", base+"/site", strings.NewReader(`{"domain":"orbit.example.org"}`))
	req.Host = "orbit.example.org"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+owner)
	r.ServeHTTP(own, req)
	expect(t, own, http.StatusBadRequest, "Orbit's own host")
	expect(t, authdo(r, "PUT", base+"/site", `{"domain":"https://Docs.Example.org/"}`, owner), http.StatusOK, "domain")
	decode(t, authdo(r, "GET", base+"/site", "", owner), &got)
	if got.Site == nil || got.Site.Domain != "docs.example.org" {
		t.Fatalf("domain kept as: %+v", got.Site)
	}
	expect(t, public("GET", "/api/v1/site/equipe-docs", "docs.example.org"), http.StatusOK, "the site on its domain")
	expect(t, public("GET", "/api/v1/branding", "docs.example.org:443"), http.StatusOK, "branding there")
	expect(t, public("GET", "/api/v1/workspaces", "docs.example.org"), http.StatusNotFound, "nothing else of Orbit there")
	expect(t, public("POST", "/api/v1/auth/login", "docs.example.org"), http.StatusMethodNotAllowed, "no signing in there")
	w = public("GET", "/robots.txt", "docs.example.org")
	if !strings.Contains(w.Body.String(), "Sitemap: https://docs.example.org/sitemap.xml") {
		t.Errorf("robots: %s", w.Body.String())
	}
	w = public("GET", "/sitemap.xml", "docs.example.org")
	if !strings.Contains(w.Body.String(), "https://docs.example.org/p/"+install) {
		t.Errorf("sitemap on its domain: %s", w.Body.String())
	}

	// Another workspace can't take its name or domain.
	other := createWS(t, r, owner, `{"name":"Other","kind":"work","starter":false}`)
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+other.ID+"/site", `{"slug":"equipe-docs"}`, owner), http.StatusBadRequest, "name taken")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+other.ID+"/site", `{"domain":"docs.example.org"}`, owner), http.StatusBadRequest, "domain taken")

	// Unpublished, or taken down: nothing there.
	expect(t, authdo(r, "PUT", base+"/site", `{"published":false}`, owner), http.StatusOK, "unpublish")
	expect(t, public("GET", "/api/v1/site/equipe-docs", ""), http.StatusNotFound, "unpublished")
	expect(t, public("GET", "/api/v1/workspaces", "docs.example.org"), http.StatusUnauthorized, "the domain is no site's now")
	expect(t, authdo(r, "DELETE", base+"/site", "", owner), http.StatusNoContent, "take down")
	decode(t, authdo(r, "GET", base+"/site", "", owner), &got)
	if got.Site != nil {
		t.Error("still there")
	}

}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Équipe Docs":             "equipe-docs",
		"  Hello, World!! ":       "hello-world",
		"日本":                      "",
		strings.Repeat("ab ", 30): "ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab-ab",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

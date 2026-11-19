package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type appResp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Allowed bool   `json:"allowed"`
}

// TestAppsModule: apps only on the owner's allowed sites (each checked, the
// CSP lets exactly them be framed); editors add, rename, order and remove
// them, viewers only look; a site taken off the list leaves its apps marked.
func TestAppsModule(t *testing.T) {
	r, owner, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Ops","kind":"work"}`)
	add := func(tok, body string) *httptest.ResponseRecorder {
		return authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/apps", body, tok)
	}
	expect(t, add(alice, `{"url":"https://gitea.example.org/"}`), http.StatusBadRequest, "no site allowed yet")

	for _, bad := range []string{"ftp://x.example", "https://x.example/path", "https://user@x.example", "https://x.example?a=1", "https://*.1.2.3.4", "nope"} {
		expect(t, authdo(r, "PUT", "/api/v1/settings", `{"embed_sites":["`+bad+`"]}`, owner), http.StatusBadRequest, bad)
	}
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"embed_sites":["https://gitea.example.org"]}`, alice), http.StatusForbidden, "not the owner")
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"embed_sites":["https://Gitea.example.org:443/","http://grafana.lan:3000","https://*.example.net",""]}`, owner), http.StatusOK, "allow")
	var sites embedSitesResponse
	decode(t, authdo(r, "GET", "/api/v1/embed-sites", "", bob), &sites)
	if strings.Join(sites.Sites, " ") != "https://gitea.example.org http://grafana.lan:3000 https://*.example.net" {
		t.Fatalf("sites: %v", sites.Sites)
	}
	if csp := do(r, "GET", "/api/v1/branding", "").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self' https://embed.diagrams.net https://gitea.example.org http://grafana.lan:3000 https://*.example.net;") {
		t.Fatalf("csp: %s", csp)
	}

	var gitea, wiki appResp
	w := add(alice, `{"url":"https://gitea.example.org/org/repo"}`)
	expect(t, w, http.StatusCreated, "gitea")
	decode(t, w, &gitea)
	if gitea.Name != "gitea.example.org" || !gitea.Allowed {
		t.Fatalf("gitea: %+v", gitea)
	}
	decode(t, add(alice, `{"name":"Wiki","url":"https://wiki.example.net/"}`), &wiki)
	expect(t, add(alice, `{"url":"http://grafana.lan:3000/d/abc"}`), http.StatusCreated, "grafana, its port")
	for _, off := range []string{"https://evil.example.com/", "http://gitea.example.org/", "http://grafana.lan/", "https://example.net/", "javascript:alert(1)"} {
		expect(t, add(alice, `{"url":"`+off+`"}`), http.StatusBadRequest, off)
	}

	expect(t, add(bob, `{"url":"https://gitea.example.org/"}`), http.StatusNotFound, "not a member")
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"viewer"}`, alice), http.StatusCreated, "bob views")
	expect(t, add(bob, `{"url":"https://gitea.example.org/"}`), http.StatusForbidden, "a viewer adds")
	expect(t, authdo(r, "DELETE", "/api/v1/apps/"+gitea.ID, "", bob), http.StatusForbidden, "a viewer removes")

	expect(t, authdo(r, "PUT", "/api/v1/apps/"+gitea.ID, `{"name":"Gitea","url":"https://evil.example.com"}`, alice), http.StatusBadRequest, "moved off the sites")
	expect(t, authdo(r, "PUT", "/api/v1/apps/"+gitea.ID, `{"name":"Gitea"}`, alice), http.StatusOK, "rename")
	expect(t, authdo(r, "PUT", "/api/v1/workspaces/"+ws.ID+"/apps/order", `{"ids":["`+wiki.ID+`","`+gitea.ID+`"]}`, alice), http.StatusNoContent, "order")
	var list []appResp
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/apps", "", bob), &list)
	if len(list) != 3 || list[0].Name != "Wiki" || list[1].Name != "Gitea" {
		t.Fatalf("list: %+v", list)
	}

	// The owner takes the wiki's site off: its app stays, marked.
	expect(t, authdo(r, "PUT", "/api/v1/settings", `{"embed_sites":["https://gitea.example.org","http://grafana.lan:3000"]}`, owner), http.StatusOK, "narrow")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/apps", "", alice), &list)
	if list[0].Allowed || !list[1].Allowed {
		t.Fatalf("after narrowing: %+v", list)
	}
	var chk appCheckResponse
	decode(t, authdo(r, "GET", "/api/v1/apps/"+wiki.ID+"/check", "", alice), &chk)
	if chk.Frameable != "no" {
		t.Fatalf("check off the list: %+v", chk)
	}

	expect(t, authdo(r, "DELETE", "/api/v1/apps/"+wiki.ID, "", alice), http.StatusNoContent, "remove")
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/apps", "", alice), &list)
	if len(list) != 2 {
		t.Fatalf("after remove: %+v", list)
	}
	// A workspace's archive carries them, in order.
	w = authdo(r, "GET", "/api/v1/workspaces/"+ws.ID+"/transfer", "", alice)
	expect(t, w, http.StatusOK, "export")
	w = importWS(r, owner, w.Body.Bytes(), false)
	expect(t, w, http.StatusCreated, "import")
	var got importResp
	decode(t, w, &got)
	decode(t, authdo(r, "GET", "/api/v1/workspaces/"+got.Workspace.ID+"/apps", "", owner), &list)
	if len(list) != 2 || list[0].Name != "Gitea" || list[0].ID == gitea.ID || !strings.HasPrefix(list[1].URL, "http://grafana.lan:3000") {
		t.Fatalf("imported: %+v", list)
	}
}

// TestFramingAllowed reads the headers as a browser does for a frame on
// https://orbit.example.org.
func TestFramingAllowed(t *testing.T) {
	at, _ := url.Parse("https://gitea.example.org/x")
	orbit := "https://orbit.example.org"
	cases := []struct {
		name string
		hd   map[string]string
		want bool
	}{
		{"nothing", nil, true},
		{"XFO deny", map[string]string{"X-Frame-Options": "DENY"}, false},
		{"XFO sameorigin (Gitea's default)", map[string]string{"X-Frame-Options": "SAMEORIGIN"}, false},
		{"frame-ancestors none", map[string]string{"Content-Security-Policy": "default-src 'self'; frame-ancestors 'none'"}, false},
		{"frame-ancestors self", map[string]string{"Content-Security-Policy": "frame-ancestors 'self'"}, false},
		{"frame-ancestors Orbit", map[string]string{"Content-Security-Policy": "frame-ancestors 'self' https://orbit.example.org"}, true},
		{"frame-ancestors Orbit, path", map[string]string{"Content-Security-Policy": "frame-ancestors https://orbit.example.org/"}, true},
		{"frame-ancestors subdomains", map[string]string{"Content-Security-Policy": "frame-ancestors *.example.org"}, true},
		{"frame-ancestors scheme", map[string]string{"Content-Security-Policy": "frame-ancestors https:"}, true},
		{"frame-ancestors star", map[string]string{"Content-Security-Policy": "frame-ancestors *"}, true},
		{"frame-ancestors other", map[string]string{"Content-Security-Policy": "frame-ancestors https://other.example.com"}, false},
		{"frame-ancestors wins over XFO", map[string]string{"Content-Security-Policy": "frame-ancestors https://orbit.example.org", "X-Frame-Options": "DENY"}, true},
		{"report-only doesn't count", map[string]string{"Content-Security-Policy-Report-Only": "frame-ancestors 'none'"}, true},
	}
	for _, c := range cases {
		hd := http.Header{}
		for k, v := range c.hd {
			hd.Set(k, v)
		}
		if got := framingAllowed(hd, at, orbit); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestProbeFrame asks real servers: a refusal by headers, one that allows
// Orbit, and one that sends you off the allowed sites.
func TestProbeFrame(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer other.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/deny":
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		case "/orbit":
			w.Header().Set("Content-Security-Policy", "frame-ancestors https://orbit.example.org")
		case "/login":
			http.Redirect(w, r, "/orbit", http.StatusFound)
		case "/away":
			http.Redirect(w, r, other.URL+"/", http.StatusFound)
		}
	}))
	defer site.Close()
	prev := embedSitesNow.Load()
	defer embedSitesNow.Store(prev)
	sites := []string{cleanEmbedSite(site.URL)}
	embedSitesNow.Store(&sites)

	orbit := "https://orbit.example.org"
	ctx := context.Background()
	for path, want := range map[string]string{"/deny": "no", "/orbit": "yes", "/login": "yes", "/plain": "yes", "/away": "no"} {
		got := probeFrame(ctx, site.URL+path, orbit)
		if got.Frameable != want {
			t.Errorf("%s: %+v, want %s", path, got, want)
		}
		if path == "/away" && (got.Why != "elsewhere" || got.Site != other.URL) {
			t.Errorf("/away: %+v", got)
		}
	}
	if got := probeFrame(ctx, "http://127.0.0.1:1/", orbit); got.Frameable != "unknown" {
		t.Errorf("unreachable: %+v", got)
	}
	b, _ := json.Marshal(probeFrame(ctx, site.URL+"/deny", orbit))
	if !strings.Contains(string(b), `"why":"headers"`) {
		t.Errorf("deny says why: %s", b)
	}
}

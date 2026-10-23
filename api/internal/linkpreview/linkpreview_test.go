package linkpreview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

func TestFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/article", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Fallback</title>
<meta property="og:title" content="How to bake bread">
<meta name="description" content="Plain description">
<meta property="og:description" content="Flour,   water, salt.">
<meta property="og:site_name" content="Kitchen">
<meta property="og:image" content="/pic.png"></head><body>…</body></html>`))
	})
	mux.HandleFunc("/plain", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title> Just a title </title><meta name="description" content="Desc"></head></html>`))
	})
	mux.HandleFunc("/pic.png", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(png) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", maxImage+10))) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	f := &Fetcher{Client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}

	p, err := f.Fetch(context.Background(), srv.URL+"/old")
	if err != nil {
		t.Fatal(err)
	}
	if p.URL != srv.URL+"/article" || p.Title != "How to bake bread" || p.Description != "Flour, water, salt." || p.SiteName != "Kitchen" || p.ImageURL != srv.URL+"/pic.png" {
		t.Fatalf("preview: %+v", p)
	}
	p, _ = f.Fetch(context.Background(), srv.URL+"/plain")
	if p.Title != "Just a title" || p.Description != "Desc" {
		t.Fatalf("plain: %+v", p)
	}
	img, err := f.FetchImage(context.Background(), srv.URL+"/pic.png")
	if err != nil || img.Type != "image/png" {
		t.Fatalf("image: %v %v", img.Type, err)
	}
	if _, err := f.FetchImage(context.Background(), srv.URL+"/big"); err == nil {
		t.Error("a too large picture")
	}
	if _, err := Normalize("javascript:alert(1)"); err == nil {
		t.Error("javascript: accepted")
	}
	if u, err := Normalize("example.org/a#top"); err != nil || u != "https://example.org/a" {
		t.Errorf("normalize: %q %v", u, err)
	}
	// The real client refuses loopback.
	if _, err := New().Fetch(context.Background(), srv.URL+"/plain"); err == nil {
		t.Error("loopback fetched with the guarded client")
	}
}

package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/linkpreview"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

type bookmarkResp struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	SiteName  string   `json:"site_name"`
	Tags      []string `json:"tags"`
	ReadLater bool     `json:"read_later"`
	ReadAt    *string  `json:"read_at"`
	HasImage  bool     `json:"has_image"`
}

// TestBookmarks: a link is kept with its preview and picture (served from
// here); tags, read later, read; not a web address is refused; deleting
// takes the picture.
func TestBookmarks(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pic.png" {
			_, _ = w.Write(tinyPNG)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><meta property="og:title" content="Sourdough guide"><meta property="og:site_name" content="Kitchen"><meta property="og:image" content="/pic.png"></head></html>`))
	}))
	defer site.Close()
	testLinks = &linkpreview.Fetcher{Client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	defer func() { testLinks = nil }()

	r, db, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Reading","kind":"personal"}`)
	base := "/api/v1/workspaces/" + ws.ID + "/bookmarks"
	expect(t, authdo(r, "POST", base, `{"url":"javascript:alert(1)"}`, alice), http.StatusBadRequest, "not a web address")

	var b bookmarkResp
	w := authdo(r, "POST", base, `{"url":"`+site.URL+`/bread","tags":["food"," food ","Baking"],"read_later":true}`, alice)
	expect(t, w, http.StatusCreated, "add")
	decode(t, w, &b)
	if b.Title != "Sourdough guide" || b.SiteName != "Kitchen" || !b.HasImage || !b.ReadLater || len(b.Tags) != 2 {
		t.Fatalf("bookmark: %+v", b)
	}
	w = authdo(r, "GET", "/api/v1/bookmarks/"+b.ID+"/image", "", alice)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), tinyPNG) || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("picture: %d", w.Code)
	}
	if w := authdo(r, "GET", "/api/v1/bookmarks/"+b.ID+"/image", "", bob); w.Code == http.StatusOK {
		t.Error("an outsider sees the picture")
	}
	decode(t, authdo(r, "PUT", "/api/v1/bookmarks/"+b.ID, `{"read":true}`, alice), &b)
	if b.ReadAt == nil {
		t.Fatal("not read")
	}
	var list []bookmarkResp
	decode(t, authdo(r, "GET", base, "", alice), &list)
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	expect(t, authdo(r, "DELETE", "/api/v1/bookmarks/"+b.ID, "", alice), http.StatusNoContent, "delete")
	var n int64
	db.Model(&models.BookmarkImage{}).Count(&n)
	if n != 0 {
		t.Errorf("pictures left: %d", n)
	}
}

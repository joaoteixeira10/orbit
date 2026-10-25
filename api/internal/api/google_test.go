package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGoogleExport(t *testing.T) {
	id := "1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	for link, want := range map[string]string{
		"https://docs.google.com/spreadsheets/d/" + id + "/edit#gid=0":        "https://docs.google.com/spreadsheets/d/" + id + "/export?format=xlsx",
		"https://docs.google.com/spreadsheets/d/" + id:                        "https://docs.google.com/spreadsheets/d/" + id + "/export?format=xlsx",
		" https://DOCS.google.com/spreadsheets/d/" + id + "/edit?usp=sharing": "https://docs.google.com/spreadsheets/d/" + id + "/export?format=xlsx",
		"https://docs.google.com/spreadsheets/d/e/2PACX-" + id + "/pubhtml":   "https://docs.google.com/spreadsheets/d/e/2PACX-" + id + "/pub?output=xlsx",
		"https://docs.google.com/document/d/" + id + "/edit":                  "",
		"https://evil.example/spreadsheets/d/" + id:                           "",
		"https://docs.google.com.evil.example/spreadsheets/d/" + id:           "",
		"not a link": "",
	} {
		if got := googleExport(link); got != want {
			t.Errorf("googleExport(%q) = %q, want %q", link, got, want)
		}
	}
}

// TestGoogleSheet: the download follows Google's redirect, hands over the
// file; a spreadsheet not shared, a link that isn't one, a redirect away
// from Google and a viewer are refused.
func TestGoogleSheet(t *testing.T) {
	r, _, _, alice, bob := plannerSetupDB(t)
	ws := createWS(t, r, alice, `{"name":"Money","kind":"personal","starter":false,"modules":["home","sheets"]}`)
	shared := "1SharedSharedSharedSharedShared"
	private := "1PrivatePrivatePrivatePrivate00"
	away := "1AwayAwayAwayAwayAwayAwayAway00"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		switch q.URL.Path {
		case "/spreadsheets/d/" + shared + "/export":
			http.Redirect(w, q, "/download/"+shared, http.StatusTemporaryRedirect)
		case "/download/" + shared:
			w.Header().Set("Content-Disposition", `attachment; filename="Budget.xlsx"; filename*=UTF-8''Or%C3%A7amento%202026.xlsx`)
			_, _ = w.Write([]byte("PK\x03\x04 the workbook"))
		case "/spreadsheets/d/" + private + "/export":
			_, _ = w.Write([]byte("<html>Sign in</html>"))
		case "/spreadsheets/d/" + away + "/export":
			http.Redirect(w, q, "https://evil.example/x", http.StatusFound)
		default:
			http.NotFound(w, q)
		}
	}))
	defer srv.Close()
	host, _ := url.Parse(srv.URL)
	oldDocs, oldClient, oldHost := googleDocs, googleClient, googleHost
	t.Cleanup(func() { googleDocs, googleClient, googleHost = oldDocs, oldClient, oldHost })
	googleDocs = srv.URL
	googleClient = func() *http.Client {
		c := srv.Client()
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return c
	}
	googleHost = func(u *url.URL) bool { return u.Host == host.Host }

	path := "/api/v1/workspaces/" + ws.ID + "/google-sheet"
	body := func(id string) string { return `{"url":"https://docs.google.com/spreadsheets/d/` + id + `/edit"}` }
	w := authdo(r, "POST", path, body(shared), alice)
	expect(t, w, http.StatusOK, "shared")
	if !strings.HasPrefix(w.Body.String(), "PK") || !strings.Contains(w.Header().Get("Content-Type"), "spreadsheetml") {
		t.Errorf("download: %q %q", w.Header().Get("Content-Type"), w.Body.String())
	}
	if cd := w.Header().Get("Content-Disposition"); cd != "attachment; filename*=UTF-8''Or%C3%A7amento%202026.xlsx" {
		t.Errorf("name: %q", cd)
	}
	expect(t, authdo(r, "POST", path, body(private), alice), http.StatusUnprocessableEntity, "not shared")
	expect(t, authdo(r, "POST", path, body("1MissingMissingMissingMissing00"), alice), http.StatusUnprocessableEntity, "missing")
	expect(t, authdo(r, "POST", path, body(away), alice), http.StatusBadGateway, "redirect away")
	expect(t, authdo(r, "POST", path, `{"url":"https://example.com/x.xlsx"}`, alice), http.StatusBadRequest, "not Google")
	expect(t, authdo(r, "POST", path, body(shared), bob), http.StatusNotFound, "not a member")
}

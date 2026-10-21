package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestCalDAVHandlers: owners set it up; the address must be a web one; the
// server is reached through the guarded dialer (no loopback); nothing linked
// reads as null.
func TestCalDAVHandlers(t *testing.T) {
	r, _, alice, bob := plannerSetup(t)
	ws := createWS(t, r, alice, `{"name":"Work","kind":"work"}`)
	base := "/api/v1/workspaces/" + ws.ID + "/caldav"
	expect(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/members", `{"user":"bob","role":"editor"}`, alice), http.StatusCreated, "bob in")

	w := authdo(r, "GET", base, "", bob)
	expect(t, w, http.StatusOK, "get")
	if !strings.Contains(w.Body.String(), `"link":null`) {
		t.Errorf("no link: %s", w.Body.String())
	}
	acct := `{"url":"https://caldav.example.org","username":"alice","password":"x"}`
	if w := authdo(r, "POST", base+"/calendars", acct, bob); w.Code == http.StatusOK || w.Code == http.StatusBadGateway {
		t.Errorf("an editor sets it up: %d", w.Code)
	}
	expect(t, authdo(r, "POST", base+"/calendars", `{"url":"ftp://x","username":"a","password":"b"}`, alice), http.StatusBadRequest, "not a web address")
	w = authdo(r, "POST", base+"/calendars", `{"url":"http://127.0.0.1:9/dav","username":"a","password":"b"}`, alice)
	expect(t, w, http.StatusBadGateway, "loopback refused")
	expect(t, authdo(r, "POST", base+"/sync", "", alice), http.StatusNotFound, "nothing to sync")
	expect(t, authdo(r, "DELETE", base, "", bob), http.StatusForbidden, "an editor unlinks")
}

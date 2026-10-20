package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/notify"
)

// TestPersonalChannels: a person adds their own Slack webhook (it must take a
// test message first), sees only a hint of it, tests it, removes it; others
// can't see or touch it; the owner's flag turns it all off.
func TestPersonalChannels(t *testing.T) {
	var mu sync.Mutex
	var got []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m["text"].(string))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	notify.UsePersonalClient(srv.Client())

	r, admin, alice, bob := plannerSetup(t)
	expect(t, authdo(r, "POST", "/api/v1/me/channels", `{"kind":"slack","webhook_url":"http://insecure.example.org"}`, alice), http.StatusBadRequest, "http refused")
	w := authdo(r, "POST", "/api/v1/me/channels", `{"kind":"slack","name":"My Slack","webhook_url":"`+srv.URL+`/hook"}`, alice)
	expect(t, w, http.StatusCreated, "add")
	var ch struct {
		ID   string `json:"id"`
		Hint string `json:"hint"`
	}
	decode(t, w, &ch)
	if ch.Hint != "127.0.0.1" || len(got) != 1 {
		t.Fatalf("added: %+v, sent %v", ch, got)
	}
	// No secret comes back.
	w = authdo(r, "GET", "/api/v1/me/channels", "", alice)
	if body := w.Body.String(); strings.Contains(body, "/hook") || strings.Contains(body, "webhook_url") {
		t.Fatalf("secret in the list: %s", body)
	}
	expect(t, authdo(r, "POST", "/api/v1/me/channels/"+ch.ID+"/test", "", alice), http.StatusNoContent, "test")
	expect(t, authdo(r, "POST", "/api/v1/me/channels/"+ch.ID+"/test", "", bob), http.StatusNotFound, "bob tests alice's")
	expect(t, authdo(r, "DELETE", "/api/v1/me/channels/"+ch.ID, "", bob), http.StatusNotFound, "bob removes alice's")

	expect(t, authdo(r, "PUT", "/api/v1/features", `{"personal_channels":false}`, admin), http.StatusOK, "flag off")
	expect(t, authdo(r, "GET", "/api/v1/me/channels", "", alice), http.StatusNotFound, "off")
	expect(t, authdo(r, "PUT", "/api/v1/features", `{"personal_channels":true}`, admin), http.StatusOK, "flag on")
	expect(t, authdo(r, "DELETE", "/api/v1/me/channels/"+ch.ID, "", alice), http.StatusNoContent, "remove")
}

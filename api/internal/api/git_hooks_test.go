package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// TestGitHookEndpoint: turning instant updates on without a git account
// hands over the address and secret; the hook then takes only posts signed
// with that secret, and the list shows when the last one came.
func TestGitHookEndpoint(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"alice","password":"alicepassword"}`)
	alice := loginTok(t, r, "alice", "alicepassword")
	ws := createWS(t, r, alice, `{"name":"Dev","kind":"work"}`)
	var list struct {
		ID string `json:"id"`
	}
	decode(t, authdo(r, "POST", "/api/v1/workspaces/"+ws.ID+"/lists", `{"name":"Issues"}`, alice), &list)
	link := models.GitLink{ListID: list.ID, WorkspaceID: ws.ID, Host: "https://git.example.org", Owner: "o", Repo: "r", Mode: "issues", LinkedBy: meID(t, r, alice)}
	db.Create(&link)

	w := authdo(r, "POST", "/api/v1/lists/"+list.ID+"/git/hook", "", alice)
	expect(t, w, http.StatusOK, "hook on")
	var setup struct {
		Auto   bool   `json:"auto"`
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	decode(t, w, &setup)
	if setup.Auto || setup.Secret == "" || setup.URL == "" {
		t.Fatalf("setup: %+v", setup)
	}
	post := func(sig string) int {
		body := []byte(`{"action":"opened"}`)
		req, _ := http.NewRequest("POST", "/api/v1/git/hook/"+link.ID, bytes.NewReader(body))
		req.Header.Set("X-Gitea-Event", "issues")
		if sig != "" {
			mac := hmac.New(sha256.New, []byte(sig))
			mac.Write(body)
			req.Header.Set("X-Gitea-Signature", hex.EncodeToString(mac.Sum(nil)))
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post(""); code != http.StatusUnauthorized {
		t.Errorf("unsigned: %d", code)
	}
	if code := post("guess"); code != http.StatusUnauthorized {
		t.Errorf("wrong secret: %d", code)
	}
	if code := post(setup.Secret); code != http.StatusAccepted {
		t.Errorf("signed: %d", code)
	}
	db.First(&link, "id = ?", link.ID)
	if link.HookAt == nil {
		t.Error("the post wasn't noted")
	}
	expect(t, authdo(r, "DELETE", "/api/v1/lists/"+list.ID+"/git/hook", "", alice), http.StatusNoContent, "hook off")
	if code := post(setup.Secret); code != http.StatusUnauthorized {
		t.Errorf("after turning off: %d", code)
	}
}

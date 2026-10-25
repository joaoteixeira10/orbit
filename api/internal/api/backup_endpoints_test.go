package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestBackupEndpoints: owner only; the export is an Orbit archive; the
// schedule is validated; "run now" writes to the directory.
func TestBackupEndpoints(t *testing.T) {
	r, db := newTestRouterDB(t, false)
	owner := loginTok(t, r, "admin", "adminpass")
	if db.Name() == "postgres" {
		// Postgres is backed up with its own tools: said so, plainly.
		w := authdo(r, "POST", "/api/v1/backup/export", "", owner)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "pg_dump") {
			t.Fatalf("export on postgres: %d %s", w.Code, w.Body.String())
		}
		var v struct {
			Supported bool `json:"supported"`
		}
		decode(t, authdo(r, "GET", "/api/v1/backup/schedule", "", owner), &v)
		if v.Supported {
			t.Error("supported on postgres")
		}
		return
	}
	createUserVia(t, r, owner, `{"username":"bob","password":"bobpassword1","role_ids":[]}`)
	bob := loginTok(t, r, "bob", "bobpassword1")
	if w := authdo(r, "POST", "/api/v1/backup/export", "", bob); w.Code != http.StatusForbidden {
		t.Errorf("not the owner: %d", w.Code)
	}
	w := authdo(r, "POST", "/api/v1/backup/export", `{"passphrase":"correct horse battery"}`, owner)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"format": "orbit-backup"`) || !strings.Contains(w.Header().Get("Content-Disposition"), "orbit-backup-passphrase-") {
		t.Fatalf("export: %d %s", w.Code, w.Header())
	}
	if w := authdo(r, "PUT", "/api/v1/backup/schedule", `{"enabled":true,"interval_hours":24,"dir":"backups"}`, owner); w.Code != http.StatusBadRequest {
		t.Errorf("relative dir: %d", w.Code)
	}
	dir := t.TempDir()
	if w := authdo(r, "PUT", "/api/v1/backup/schedule", `{"enabled":true,"interval_hours":24,"dir":"`+dir+`","keep":3}`, owner); w.Code != http.StatusOK {
		t.Fatalf("schedule: %d %s", w.Code, w.Body.String())
	}
	w = authdo(r, "POST", "/api/v1/backup/run", "", owner)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) || !strings.Contains(w.Body.String(), dir) {
		t.Errorf("run now: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(authdo(r, "GET", "/api/v1/backup/schedule", "", owner).Body.String(), `"last":{"at"`) {
		t.Error("the last run shows")
	}
}

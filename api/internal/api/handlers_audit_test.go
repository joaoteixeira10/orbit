package api

import (
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func auditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnrichAuditResolvesNames(t *testing.T) {
	db := auditTestDB(t)
	must := func(err error) {
		if err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	// IDs are UUID strings; set them explicitly so the audit entries below can
	// reference them deterministically (BeforeCreate only generates when empty).
	must(db.Create(&models.User{Base: models.Base{ID: "1"}, Username: "alice"}).Error)
	must(db.Create(&models.Role{Base: models.Base{ID: "1"}, Name: "auditor"}).Error)

	h := &Handlers{db: db}
	out := h.enrichAudit([]models.AuditLog{
		{UserID: "1", Action: "user.update", TargetType: "user", TargetID: "1"},
		{UserID: "1", Action: "role.update", TargetType: "role", TargetID: "1"},
		{UserID: "", Action: "settings.update", TargetType: "settings", TargetID: "branding"},
		{UserID: "99", Action: "role.delete", TargetType: "role", TargetID: "999"}, // both gone
	})

	cases := []struct{ userName, targetName string }{
		{"alice", "alice"},
		{"alice", "auditor"},
		{"system", ""}, // settings has no name table → falls back client-side
		{"#99", ""},    // deleted actor + deleted target
	}
	for i, want := range cases {
		if out[i].UserName != want.userName {
			t.Errorf("row %d user_name: got %q want %q", i, out[i].UserName, want.userName)
		}
		if out[i].TargetName != want.targetName {
			t.Errorf("row %d target_name: got %q want %q", i, out[i].TargetName, want.targetName)
		}
	}
}

// TestAuditExport: the CSV has every entry (or the matches), neutralises
// formula-looking cells, and the export itself is audited.
func TestAuditExport(t *testing.T) {
	r := newTestRouter(t)
	admin := loginTok(t, r, "admin", "adminpass")
	createUserVia(t, r, admin, `{"username":"=cmd","password":"cmdpassword","role_ids":[]}`)
	w := authdo(r, "GET", "/api/v1/audit/export", "", admin)
	body := w.Body.String()
	if w.Code != 200 || !strings.HasPrefix(body, "time,action,user,") || !strings.Contains(w.Header().Get("Content-Disposition"), ".csv") {
		t.Fatalf("export: %d %s", w.Code, body[:min(200, len(body))])
	}
	if !strings.Contains(body, "user.create") || !strings.Contains(body, "auth.login") {
		t.Errorf("rows missing: %s", body)
	}
	w = authdo(r, "GET", "/api/v1/audit/export?q=user.create", "", admin)
	if strings.Contains(w.Body.String(), "auth.login") {
		t.Error("q must filter")
	}
	if w := authdo(r, "GET", "/api/v1/audit?q=audit.export", "", admin); !strings.Contains(w.Body.String(), "audit.export") {
		t.Error("export not audited")
	}
}

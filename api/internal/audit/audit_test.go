package audit

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func memDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := g.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return g
}

func TestRecordPersistsEntry(t *testing.T) {
	g := memDB(t)
	r := New(g, zap.NewNop())

	r.Record("7", "cert.issue", "certificate", "42", "10.0.0.5", map[string]any{"cn": "host.example.org"})

	var rows []models.AuditLog
	if err := g.Find(&rows).Error; err != nil || len(rows) != 1 {
		t.Fatalf("want 1 audit row, got %d (%v)", len(rows), err)
	}
	e := rows[0]
	if e.UserID != "7" || e.Action != "cert.issue" || e.TargetType != "certificate" || e.TargetID != "42" || e.IP != "10.0.0.5" {
		t.Errorf("audit fields wrong: %+v", e)
	}
	if e.Detail != `{"cn":"host.example.org"}` {
		t.Errorf("detail JSON = %q", e.Detail)
	}
}

func TestRecordNilDetail(t *testing.T) {
	g := memDB(t)
	New(g, zap.NewNop()).Record("", "auth.login", "user", "1", "::1", nil)
	var e models.AuditLog
	if err := g.First(&e).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if e.Detail != "" {
		t.Errorf("nil detail should store empty string, got %q", e.Detail)
	}
}

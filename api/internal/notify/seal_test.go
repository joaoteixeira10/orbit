package notify

import (
	"strings"
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TestConfigSealedAtRest: channel secrets are stored sealed, read back, and a
// plain configuration from before is sealed by SealStored.
func TestConfigSealedAtRest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	_ = db.AutoMigrate(models.AllModels()...)
	k, _ := keystore.New("notify-master-key-0123456789-abcdef", []byte("0123456789abcdef"))

	// Written by an older build: plain JSON.
	plain := New(db, zap.NewNop())
	var cfg Config
	cfg.Email.Password = "smtp-secret-pass"
	cfg.Webhook.Secret = "hook-signing-secret"
	if err := plain.Save(cfg); err != nil {
		t.Fatal(err)
	}
	svc := New(db, zap.NewNop()).UseSealer(k)
	if err := svc.SealStored(); err != nil {
		t.Fatal(err)
	}
	var row models.Setting
	db.Where(&models.Setting{Key: settingKey}).First(&row)
	if !strings.HasPrefix(row.Value, sealedPrefix) || strings.Contains(row.Value, "smtp-secret-pass") || strings.Contains(row.Value, "hook-signing-secret") {
		t.Fatalf("stored in plain text: %q", row.Value)
	}
	if got := svc.Load(); got.Email.Password != "smtp-secret-pass" || got.Webhook.Secret != "hook-signing-secret" {
		t.Errorf("round trip: %+v", got)
	}
}

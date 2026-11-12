package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestBackupBeforeMigrate: a copy on the first start of a new version only —
// not for a new database, not on the same version, never with PostgreSQL.
func TestBackupBeforeMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orbit.db")
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, why, _ := BackupBeforeMigrate(g, "sqlite", path, "0.6.0"); why != "a new database" {
		t.Fatalf("new database: %q", why)
	}
	if err := g.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	g.Create(&models.Setting{Key: "version_history", Value: `[{"version":"0.5.0"}]`})

	got, size, why, err := BackupBeforeMigrate(g, "sqlite", path, "0.6.0")
	if err != nil || why != "" || got != path+".before-0.6.0" || size == 0 {
		t.Fatalf("new version: %q %d %q %v", got, size, why, err)
	}
	c, err := gorm.Open(sqlite.Open(got), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var n int64
	c.Model(&models.Setting{}).Where("key = ?", "version_history").Count(&n)
	if n != 1 {
		t.Fatal("the copy hasn't the data")
	}
	// Again before it's recorded (a failed start): a second copy, not over the first.
	again, _, _, err := BackupBeforeMigrate(g, "sqlite", path, "0.6.0")
	if err != nil || !strings.HasPrefix(again, path+".before-0.6.0-") {
		t.Fatalf("second copy: %q %v", again, err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatal("the first copy went")
	}

	g.Model(&models.Setting{}).Where("key = ?", "version_history").Update("value", `[{"version":"0.5.0"},{"version":"0.6.0"}]`)
	if _, _, why, _ := BackupBeforeMigrate(g, "sqlite", path, "0.6.0"); why != "the same version as last time" {
		t.Fatalf("same version: %q", why)
	}
	if p, _, why, _ := BackupBeforeMigrate(g, "postgres", "", "0.7.0"); p != "" || !strings.Contains(why, "PostgreSQL") {
		t.Fatalf("postgres: %q %q", p, why)
	}
}

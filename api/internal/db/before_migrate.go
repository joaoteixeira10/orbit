package db

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

// BackupBeforeMigrate, with BACKUP_BEFORE_MIGRATE on, copies an SQLite
// database before a new version of Orbit changes it: on the first start of
// a version (the last one recorded in version_history differs), the file is
// copied — consistently, VACUUM INTO — beside itself as
// <db>.before-<version>, to go back to with the old version if the new one
// goes wrong. Migrations only go forward; this is the way back.
//
// Nothing is copied for a new database (no tables yet), on the same version,
// or with PostgreSQL (back that up with pg_dump or your provider's
// snapshots: skipped says so). Copies aren't removed: each is a full copy of
// the database, kept until you delete it.
func BackupBeforeMigrate(gdb *gorm.DB, driver, dbPath, current string) (path string, size int64, skipped string, err error) {
	if driver != "sqlite" {
		return "", 0, "PostgreSQL is backed up outside Orbit (pg_dump, your provider's snapshots)", nil
	}
	if !gdb.Migrator().HasTable(&models.Setting{}) {
		return "", 0, "a new database", nil
	}
	last := lastVersion(gdb)
	if last == current {
		return "", 0, "the same version as last time", nil
	}
	path = dbPath + ".before-" + safeVersion.ReplaceAllString(current, "_")
	if _, statErr := os.Stat(path); statErr == nil {
		path += "-" + time.Now().UTC().Format("20060102-150405")
	}
	if err := gdb.Exec("VACUUM INTO ?", path).Error; err != nil {
		return "", 0, "", fmt.Errorf("copy the database to %s: %w", path, err)
	}
	if fi, statErr := os.Stat(path); statErr == nil {
		size = fi.Size()
	}
	return path, size, "", nil
}

var safeVersion = regexp.MustCompile(`[^0-9A-Za-z.-]`)

// lastVersion is the version that last started on this database ("" none
// recorded: one from before the history was kept).
func lastVersion(gdb *gorm.DB) string {
	var row models.Setting
	if gdb.Where(&models.Setting{Key: "version_history"}).Limit(1).Find(&row).RowsAffected == 0 {
		return ""
	}
	var hist []struct {
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(row.Value), &hist) != nil || len(hist) == 0 {
		return ""
	}
	return hist[len(hist)-1].Version
}

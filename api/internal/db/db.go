// Package db opens the database for the configured driver and runs migrations.
//
// SQLite (pure Go, no CGO; one file) is the default; PostgreSQL is the other
// engine, for an instance whose database lives on its own server. The models
// stay portable (GORM AutoMigrate, no dialect-specific SQL); what only SQLite
// has — the FTS5 search index, VACUUM INTO backups — is used only there
// (search falls back to plain matching; Postgres is backed up with its own
// tools).
package db

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/planner"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/search"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open connects using cfg and returns a ready *gorm.DB.
func Open(cfg *config.Config) (*gorm.DB, error) {
	dialector, err := dialector(cfg)
	if err != nil {
		return nil, err
	}

	logLevel := gormlogger.Warn
	if cfg.GinMode != "release" {
		logLevel = gormlogger.Info
	}

	// IgnoreRecordNotFoundError: "record not found" is expected for optional
	// setting lookups (otp_enforced, oidc_config, …) and shouldn't be logged.
	gdb, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			gormlogger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true,
			},
		),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	return gdb, nil
}

func dialector(cfg *config.Config) (gorm.Dialector, error) {
	switch cfg.DBDriver {
	case "sqlite":
		// _pragma busy_timeout + foreign_keys for sane sqlite behavior.
		dsn := cfg.DBURL + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
		return sqlite.Open(dsn), nil
	case "postgres":
		// DB_URL is a connection string: postgres://user:pass@host:5432/orbit?sslmode=require
		return postgres.Open(cfg.DBURL), nil
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}
}

// Migrate creates/updates all tables.
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	if err := migrateAssignees(gdb); err != nil {
		return err
	}
	if err := migrateListFolders(gdb); err != nil {
		return err
	}
	// Members from before groups: their role is their own.
	if err := gdb.Exec("UPDATE workspace_members SET direct_role = role WHERE direct_role = '' AND NOT EXISTS (SELECT 1 FROM workspace_groups WHERE workspace_groups.workspace_id = workspace_members.workspace_id)").Error; err != nil {
		return fmt.Errorf("direct roles: %w", err)
	}
	// Files from before versions: each its own first.
	if err := gdb.Exec("UPDATE uploads SET lineage = id WHERE lineage IS NULL OR lineage = ''").Error; err != nil {
		return fmt.Errorf("file versions: %w", err)
	}
	// The full-text index (SQLite FTS5); search falls back to plain matching
	// without it.
	if !search.Setup(gdb) {
		log.Println("full-text search index unavailable; using plain search")
	}
	return nil
}

// migrateListFolders, once: every list gets its folder, and Files is on in
// every workspace with tasks, pages, sheets, diagrams or whiteboards (things
// that live in folders) — as new ones are made since.
func migrateListFolders(gdb *gorm.DB) error {
	const done = "lists_have_folders"
	var row models.Setting
	if gdb.Where(&models.Setting{Key: done}).Limit(1).Find(&row).RowsAffected == 1 {
		return nil
	}
	return gdb.Transaction(func(tx *gorm.DB) error {
		var wss []models.Workspace
		if err := tx.Find(&wss).Error; err != nil {
			return err
		}
		for _, ws := range wss {
			if err := planner.ListFolders(tx, ws.ID, ""); err != nil {
				return fmt.Errorf("list folders: %w", err)
			}
			var mods []string
			_ = json.Unmarshal([]byte(ws.Modules), &mods)
			if slices.Contains(mods, planner.ModFiles) || !slices.ContainsFunc(mods, func(m string) bool { return slices.Contains(planner.FilesWith, m) }) {
				continue
			}
			b, _ := json.Marshal(append(mods, planner.ModFiles))
			if err := tx.Model(&models.Workspace{}).Where("id = ?", ws.ID).Update("modules", string(b)).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.Setting{Key: done, Value: "true"}).Error
	})
}

// migrateAssignees moves a task's single assignee (the old assignee_id
// column) into its assignees list, once: the old value is cleared after.
func migrateAssignees(gdb *gorm.DB) error {
	if !gdb.Migrator().HasColumn(&models.Task{}, "assignee_id") {
		return nil
	}
	var rows []struct {
		ID         string
		AssigneeID string
	}
	if err := gdb.Table("tasks").Select("id, assignee_id").Where("assignee_id IS NOT NULL AND assignee_id <> ''").Scan(&rows).Error; err != nil {
		return fmt.Errorf("read assignees: %w", err)
	}
	return gdb.Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			var t models.Task
			t.SetAssignees([]string{r.AssigneeID})
			if err := tx.Table("tasks").Where("id = ?", r.ID).Updates(map[string]any{"assignees": t.Assignees, "assignee_id": nil}).Error; err != nil {
				return fmt.Errorf("move assignee: %w", err)
			}
		}
		return nil
	})
}

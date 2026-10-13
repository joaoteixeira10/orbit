// Package db opens the database for the configured driver and runs migrations.
//
// SQLite (pure Go, no CGO) is the only engine for now: an engine counts as
// supported only once it is smoke-tested (docs/BLUEPRINT-FIT.md). The models
// stay portable (GORM AutoMigrate, no dialect-specific SQL), so adding
// Postgres later is a driver + smoke test, not a rewrite.
package db

import (
	"fmt"
	"log"
	"os"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/config"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
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
	default:
		return nil, fmt.Errorf("unsupported driver %q", cfg.DBDriver)
	}
}

// Migrate creates/updates all tables.
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(models.AllModels()...); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}
	return nil
}

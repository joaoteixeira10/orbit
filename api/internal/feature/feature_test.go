package feature

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestDefaultsAndSet(t *testing.T) {
	db := testDB(t)

	// Defaults: every known flag present and off.
	all := All(db)
	if len(all) != len(Defaults) {
		t.Fatalf("All returned %d flags, want %d", len(all), len(Defaults))
	}
	if all[Sessions] {
		t.Errorf("sessions should default off")
	}
	if Enabled(db, Sessions) {
		t.Errorf("Enabled(sessions) should be false by default")
	}

	// Set persists.
	if err := Set(db, Sessions, true); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !Enabled(db, Sessions) {
		t.Errorf("sessions should be enabled after Set(true)")
	}

	// Set back off.
	if err := Set(db, Sessions, false); err != nil {
		t.Fatalf("set: %v", err)
	}
	if Enabled(db, Sessions) {
		t.Errorf("sessions should be off after Set(false)")
	}
}

func TestUnknownStoredFlagsIgnored(t *testing.T) {
	db := testDB(t)
	// Persist a stored map containing an unknown flag.
	if err := db.Save(&models.Setting{Key: "features", Value: `{"sessions":true,"bogus":true}`}).Error; err != nil {
		t.Fatal(err)
	}
	all := All(db)
	if !all[Sessions] {
		t.Errorf("known stored flag should be honoured")
	}
	if _, ok := all["bogus"]; ok {
		t.Errorf("unknown stored flag must not leak into All()")
	}
	if IsKnown("bogus") {
		t.Errorf("bogus should not be a known flag")
	}
}

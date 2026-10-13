package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TestBeforeCreateGeneratesV7 asserts the Base hook stamps a UUIDv7 on insert and
// that Order("id") returns rows in creation order — the property the audit hash
// chain and newest-first listings rely on.
func TestBeforeCreateGeneratesV7(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&Role{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	const n = 25
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		r := Role{Name: "role" + string(rune('a'+i))}
		if err := db.Create(&r).Error; err != nil {
			t.Fatalf("create: %v", err)
		}
		if r.ID == "" {
			t.Fatal("BeforeCreate should have set an ID")
		}
		u, err := uuid.Parse(r.ID)
		if err != nil {
			t.Fatalf("generated ID %q is not a UUID: %v", r.ID, err)
		}
		if u.Version() != 7 {
			t.Errorf("generated ID %q is UUID version %d, want 7", r.ID, u.Version())
		}
		ids = append(ids, r.ID)
	}

	// An explicit ID must be preserved (the hook only fills empties).
	fixed := Role{Base: Base{ID: "fixed-id"}, Name: "fixed"}
	if err := db.Create(&fixed).Error; err != nil {
		t.Fatalf("create fixed: %v", err)
	}
	if fixed.ID != "fixed-id" {
		t.Errorf("explicit ID overwritten: %q", fixed.ID)
	}

	// Order("id") returns rows in insertion order (UUIDv7 is time-ordered).
	var rows []Role
	if err := db.Where("id <> ?", "fixed-id").Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != n {
		t.Fatalf("got %d rows, want %d", len(rows), n)
	}
	for i := range rows {
		if rows[i].ID != ids[i] {
			t.Errorf("row %d out of creation order: got %q want %q", i, rows[i].ID, ids[i])
		}
	}
}

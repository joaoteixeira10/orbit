package audit

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newRec(t *testing.T) (*Recorder, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(db, zap.NewNop()), db
}

// TestAuditChainVerifies records a few entries and confirms the chain verifies,
// then that altering a stored entry is detected at that row.
func TestAuditChain(t *testing.T) {
	r, db := newRec(t)
	for i := 0; i < 5; i++ {
		r.Record("1", "test.action", "thing", "1", "127.0.0.1", nil)
	}

	if res := r.Verify(); !res.OK || res.Checked != 5 {
		t.Fatalf("fresh chain: %+v, want ok/5", res)
	}

	// Each entry links to the previous one's hash.
	var rows []models.AuditLog
	db.Order("id ASC").Find(&rows)
	for i := 1; i < len(rows); i++ {
		if rows[i].PrevHash != rows[i-1].Hash {
			t.Fatalf("entry %s prev_hash != entry %s hash", rows[i].ID, rows[i-1].ID)
		}
	}

	// Tamper with the middle entry's content (as a DB edit would).
	bad := rows[2]
	if err := db.Model(&models.AuditLog{}).Where("id = ?", bad.ID).Update("action", "tampered").Error; err != nil {
		t.Fatalf("tamper: %v", err)
	}
	res := r.Verify()
	if res.OK || res.BrokenAt != bad.ID {
		t.Errorf("after tamper: %+v, want broken_at=%s", res, bad.ID)
	}
}

// TestBackfillChain hashes pre-existing (un-chained) rows and then verifies.
func TestBackfillChain(t *testing.T) {
	r, db := newRec(t)
	// Insert rows directly (no hash), simulating a pre-upgrade log.
	for i := 0; i < 3; i++ {
		db.Create(&models.AuditLog{UserID: "1", Action: "legacy", TargetType: "x", TargetID: "1"})
	}
	if res := r.Verify(); res.OK {
		t.Fatal("un-chained rows should not verify before backfill")
	}
	if err := r.BackfillChain(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if res := r.Verify(); !res.OK || res.Checked != 3 {
		t.Fatalf("after backfill: %+v, want ok/3", res)
	}
	// New records continue the chain.
	r.Record("2", "after", "x", "2", "127.0.0.1", nil)
	if res := r.Verify(); !res.OK || res.Checked != 4 {
		t.Fatalf("after new record: %+v, want ok/4", res)
	}
}

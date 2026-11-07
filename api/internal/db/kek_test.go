package db

import (
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
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

// TestOpenSealerPersistsSalt: the first call stores a salt; a later call with
// the same master key reads it back and can unseal what the first sealed.
func TestOpenSealerPersistsSalt(t *testing.T) {
	g := memDB(t)
	s1, err := OpenSealer(g, "master-key-0123456789-abcdefghij")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	blob, _ := s1.Seal([]byte("totp-secret"))
	var row models.Setting
	if g.Where(&models.Setting{Key: settingKEKSalt}).First(&row).Error != nil || row.Value == "" {
		t.Fatal("kek_salt was not persisted")
	}
	s2, err := OpenSealer(g, "master-key-0123456789-abcdefghij")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if pt, err := s2.Unseal(blob); err != nil || string(pt) != "totp-secret" {
		t.Fatalf("reopened sealer should unseal: %v %q", err, pt)
	}
}

// TestOpenSealerWrongMasterKey: another master key can't read sealed data.
func TestOpenSealerWrongMasterKey(t *testing.T) {
	g := memDB(t)
	s1, _ := OpenSealer(g, "right-master-key-0123456789-abcd")
	blob, _ := s1.Seal([]byte("secret"))
	s2, _ := OpenSealer(g, "wrong-master-key-0123456789-abcd")
	if _, err := s2.Unseal(blob); err == nil {
		t.Fatal("a different master key must not unseal")
	}
}

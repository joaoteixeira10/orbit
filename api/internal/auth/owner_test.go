package auth

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func ownerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnsureOwnerBackfillsEarliestSuperuser(t *testing.T) {
	db := ownerTestDB(t)
	// Two users, neither owner. admin2 holds a superuser role.
	viewer := &models.User{Username: "viewer"} // not superuser
	db.Create(viewer)
	admin2 := &models.User{Username: "admin2"} // superuser
	db.Create(admin2)
	su := models.Role{Name: "admin", Superuser: true, Builtin: true}
	db.Create(&su)
	db.Exec("INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)", admin2.ID, su.ID)

	if err := EnsureOwner(db, zap.NewNop()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var u2 models.User
	db.First(&u2, "id = ?", admin2.ID)
	if !u2.Owner {
		t.Errorf("the superuser should have been made owner")
	}
	var u1 models.User
	db.First(&u1, "id = ?", viewer.ID)
	if u1.Owner {
		t.Errorf("non-superuser should not be owner")
	}
}

func TestEnsureOwnerNoopWhenOwnerExists(t *testing.T) {
	db := ownerTestDB(t)
	db.Create(&models.User{Username: "a", Owner: true}) // id 1
	db.Create(&models.User{Username: "b"})              // id 2

	if err := EnsureOwner(db, zap.NewNop()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var owners int64
	db.Model(&models.User{}).Where("owner = ?", true).Count(&owners)
	if owners != 1 {
		t.Errorf("owner count = %d, want exactly 1 (no-op)", owners)
	}
}

func TestEnsureOwnerFallbackEarliestUser(t *testing.T) {
	db := ownerTestDB(t)
	// No superuser roles at all → earliest user becomes owner.
	first := &models.User{Username: "first"}
	db.Create(first)
	db.Create(&models.User{Username: "second"})

	if err := EnsureOwner(db, zap.NewNop()); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	var u1 models.User
	db.First(&u1, "id = ?", first.ID)
	if !u1.Owner {
		t.Errorf("earliest user should be owner when there's no superuser")
	}
}

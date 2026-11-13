package rbac

import (
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func seedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Role{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func has(privs []string, p string) bool {
	for _, x := range privs {
		if x == p {
			return true
		}
	}
	return false
}

func TestSeedAdminExcludesOwnerOnly(t *testing.T) {
	db := seedTestDB(t)
	// Pre-existing admin with the OLD all-privileges list (incl. owner-only).
	old := append([]string{}, GlobalPrivileges...)
	db.Create(&models.Role{Name: string(models.RoleAdmin), Builtin: true, Superuser: true, GlobalPrivileges: EncodeStrings(old)})

	if err := SeedBuiltins(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var admin models.Role
	db.Where("name = ? AND builtin = ?", string(models.RoleAdmin), true).First(&admin)
	got := DecodeStrings(admin.GlobalPrivileges)
	for _, p := range OwnerOnlyGlobals {
		if has(got, p) {
			t.Errorf("reconciled admin must not hold owner-only privilege %q (got %v)", p, got)
		}
	}
	if !has(got, PUsersManage) || !has(got, PAuditRead) {
		t.Errorf("admin should keep ordinary globals, got %v", got)
	}
}

// TestSeedBuiltins: the three built-ins exist with the intended powers.
func TestSeedBuiltins(t *testing.T) {
	db := seedTestDB(t)
	if err := SeedBuiltins(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	load := func(name string) Effective {
		var r models.Role
		db.Where("name = ?", name).First(&r)
		return Build([]models.Role{r})
	}
	if a := load("admin"); !a.Superuser || !a.HasGlobal(PUsersManage) || a.HasGlobal(PSettingsBranding) {
		t.Error("admin: superuser without the owner-only privileges")
	}
	if m := load("member"); !m.HasGlobal(PWorkspacesCreate) || m.HasGlobal(PUsersManage) {
		t.Error("member: create workspaces, no user management")
	}
	if n := load("none"); n.HasGlobal(PWorkspacesCreate) || n.HasGlobal(PAuditRead) {
		t.Error("none: nothing")
	}
}

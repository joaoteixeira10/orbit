package auth

import (
	"testing"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/rbac"

	"go.uber.org/zap"
)

// TestBootstrapAdmin: the first admin is the owner, holds the built-in admin
// role, must change its password, and gets a temporary one when ADMIN_PASSWORD
// is empty. A second run changes nothing.
func TestBootstrapAdmin(t *testing.T) {
	db := testDB(t)
	if err := rbac.SeedBuiltins(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := Bootstrap(db, zap.NewNop(), "", ""); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	var u models.User
	if err := db.Preload("Roles").Where("username = ?", "admin").First(&u).Error; err != nil {
		t.Fatalf("admin not created: %v", err)
	}
	if !u.Owner || !u.MustChangePassword || u.PasswordHash == "" {
		t.Errorf("owner=%v must_change=%v hash set=%v", u.Owner, u.MustChangePassword, u.PasswordHash != "")
	}
	if len(u.Roles) != 1 || u.Roles[0].Name != string(models.RoleAdmin) || !u.Roles[0].Superuser {
		t.Errorf("roles = %+v, want the built-in admin", u.Roles)
	}
	if err := Bootstrap(db, zap.NewNop(), "other", "pw-0123456789"); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	var n int64
	db.Model(&models.User{}).Count(&n)
	if n != 1 {
		t.Errorf("second run created users: %d", n)
	}
}

// TestTempPassword: four groups of four from the look-alike-free alphabet.
func TestTempPassword(t *testing.T) {
	p := TempPassword()
	if len(p) != 19 || p[4] != '-' || p[9] != '-' || p[14] != '-' {
		t.Fatalf("unexpected shape %q", p)
	}
	for _, c := range p {
		if c != '-' && !containsRune(tempAlphabet, c) {
			t.Fatalf("%q has %q outside the alphabet", p, c)
		}
	}
	if TempPassword() == p {
		t.Error("two temporary passwords should differ")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

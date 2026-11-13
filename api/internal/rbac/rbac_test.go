package rbac

import (
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

func roleWith(privs []string) models.Role {
	return models.Role{Name: "r", GlobalPrivileges: EncodeStrings(privs)}
}

// TestSuperuserHasAllButOwnerOnly: the superuser flag implies every ordinary
// privilege but never the owner-gated ones.
func TestSuperuserHasAllButOwnerOnly(t *testing.T) {
	e := Build([]models.Role{{Superuser: true}})
	for _, p := range GlobalPrivileges {
		if got, want := e.HasGlobal(p), !IsOwnerOnlyGlobal(p); got != want {
			t.Errorf("superuser HasGlobal(%s) = %v, want %v", p, got, want)
		}
	}
}

// TestUnionOfRoles: effective privileges are the union of every role's.
func TestUnionOfRoles(t *testing.T) {
	e := Build([]models.Role{roleWith([]string{PAuditRead}), roleWith([]string{PWorkspacesCreate, PSettingsBranding})})
	if !e.HasGlobal(PAuditRead) || !e.HasGlobal(PWorkspacesCreate) || !e.HasGlobal(PSettingsBranding) || e.HasGlobal(PUsersManage) {
		t.Errorf("union wrong: %+v", e.Global)
	}
}

// TestNarrow: a token's privilege list and read-only flag only ever take away.
func TestNarrow(t *testing.T) {
	su := Build([]models.Role{{Superuser: true}})
	if n := su.Narrow(nil, false); !n.Superuser || n.ReadOnly {
		t.Error("no narrowing keeps everything")
	}
	if n := su.Narrow(nil, true); !n.Superuser || !n.ReadOnly {
		t.Error("read-only keeps privileges, adds ReadOnly")
	}
	n := su.Narrow([]string{PAuditRead, PSettingsBranding}, false)
	if n.Superuser || !n.HasGlobal(PAuditRead) || n.HasGlobal(PUsersManage) || n.HasGlobal(PSettingsBranding) {
		t.Errorf("narrowed set wrong: %+v", n)
	}
}

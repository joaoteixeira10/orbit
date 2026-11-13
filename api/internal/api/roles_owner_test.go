package api

import (
	"testing"

	"github.com/joaoteixeira10/orbit/api/internal/rbac"
)

func TestOwnerOnlySetAndEquality(t *testing.T) {
	in := []string{rbac.PUsersManage, rbac.PSettingsBranding, rbac.PUsersImpersonate, rbac.PAuditRead}
	got := ownerOnlySet(in)
	if len(got) != 2 || !got[rbac.PSettingsBranding] || !got[rbac.PUsersImpersonate] {
		t.Errorf("ownerOnlySet picked wrong privileges: %v", got)
	}
	if got[rbac.PUsersManage] || got[rbac.PAuditRead] {
		t.Error("ordinary privileges must not be flagged owner-only")
	}

	a := ownerOnlySet([]string{rbac.PSettingsBranding})
	b := ownerOnlySet([]string{rbac.PSettingsBranding})
	c := ownerOnlySet([]string{rbac.PSettingsBranding, rbac.PSettingsNotify})
	if !sameStringSet(a, b) {
		t.Error("identical owner-only sets should compare equal")
	}
	if sameStringSet(a, c) {
		t.Error("different owner-only sets should not compare equal")
	}
}

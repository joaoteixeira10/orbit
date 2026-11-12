// Package rbac defines the instance-level privilege catalog and resolves a
// user's roles into an effective, queryable permission set. A user's
// effective access is the union of all their roles.
//
// These privileges are about running the instance: managing accounts and
// roles, reading the audit log, creating workspaces. What someone may do
// inside a workspace is not here: it is their role in that workspace's
// membership (owner / editor / viewer), checked by the workspace handlers.
// Admins therefore can't read other people's planners.
package rbac

import (
	"encoding/json"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"
)

// Global privileges. Backup export/restore, security policy and SSO config
// are gated by auth.RequireOwner, not by RBAC.
const (
	PUsersManage      = "users.manage"
	PRolesManage      = "roles.manage"
	PAuditRead        = "audit.read"
	PWorkspacesCreate = "workspaces.create" // create workspaces (invited ones need nothing)
	// Owner-gated (see OwnerOnlyGlobals).
	PSettingsBranding = "settings.branding"      // branding (app name, accent, logo)
	PSettingsNotify   = "settings.notifications" // notification channels
	PUsersImpersonate = "users.impersonate"
)

// GlobalPrivileges is the catalog of privileges (for the role editor).
var GlobalPrivileges = []string{PUsersManage, PRolesManage, PAuditRead, PWorkspacesCreate, PSettingsBranding, PSettingsNotify, PUsersImpersonate}

// OwnerOnlyGlobals are privileges the instance owner holds by default and only
// the owner can delegate. The superuser flag does NOT grant them (so a built-in
// Admin doesn't implicitly get branding/notifications/impersonation); they're
// reachable only by the owner or by an owner-created custom role that lists them.
var OwnerOnlyGlobals = []string{PSettingsBranding, PSettingsNotify, PUsersImpersonate}

func setOf(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, p := range list {
		m[p] = true
	}
	return m
}

var (
	ownerOnlyGlobal = setOf(OwnerOnlyGlobals)
	validGlobal     = setOf(GlobalPrivileges)
)

// IsOwnerOnlyGlobal reports whether a privilege is owner-gated.
func IsOwnerOnlyGlobal(p string) bool { return ownerOnlyGlobal[p] }

// IsValidGlobal validates a privilege string.
func IsValidGlobal(p string) bool { return validGlobal[p] }

// Effective is a resolved permission set for a request. ReadOnly comes from
// a read-only API token: workspace content can be read but not changed.
type Effective struct {
	Superuser bool
	Global    map[string]bool
	ReadOnly  bool
}

// Build resolves a set of roles into an Effective set.
func Build(roles []models.Role) Effective {
	e := Effective{Global: map[string]bool{}}
	for _, r := range roles {
		if r.Superuser {
			e.Superuser = true
		}
		for _, p := range parseStrings(r.GlobalPrivileges) {
			e.Global[p] = true
		}
	}
	return e
}

// HasGlobal reports whether a global privilege is held. Owner-gated privileges
// are NOT granted by the superuser flag — they must be held explicitly (or the
// caller must be the instance owner, checked separately via RequireOwnerOrGlobal).
func (e Effective) HasGlobal(p string) bool {
	if ownerOnlyGlobal[p] {
		return e.Global[p]
	}
	return e.Superuser || e.Global[p]
}

// Narrow limits an Effective set, for an API token: only the privileges in
// privs (nil = all), and read-only workspace access when readOnly. The result
// never exceeds e. Owner-only globals are dropped when privs is set unless
// listed.
func (e Effective) Narrow(privs []string, readOnly bool) Effective {
	if len(privs) == 0 {
		e.ReadOnly = e.ReadOnly || readOnly
		return e
	}
	allow := setOf(privs)
	out := Effective{Global: map[string]bool{}, ReadOnly: e.ReadOnly || readOnly}
	for _, p := range GlobalPrivileges {
		if e.HasGlobal(p) && allow[p] {
			out.Global[p] = true
		}
	}
	return out
}

// ParseStrings reads a JSON list of strings (a role's privileges).
func ParseStrings(s string) []string { return parseStrings(s) }

func parseStrings(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// DecodeStrings parses a stored JSON string list (for API responses).
func DecodeStrings(s string) []string { return parseStrings(s) }

// EncodeStrings marshals a list for storage (privileges).
func EncodeStrings(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

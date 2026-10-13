package rbac

import (
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// SeedBuiltins creates the immutable built-in roles if they don't exist:
//   - admin  : superuser (users, roles, audit log, workspaces)
//   - member : creates and runs their own workspaces (the default for people
//     an admin adds)
//   - none   : no instance privileges (default for SSO-provisioned users); can
//     still be invited into someone else's workspace
//
// None of them gives access to anyone's workspace content: that comes only
// from workspace membership.
func SeedBuiltins(db *gorm.DB) error {
	builtins := []models.Role{
		{
			Name: string(models.RoleAdmin), Description: "Manage the instance: users, roles, audit log (superuser)",
			Builtin: true, Superuser: true,
			GlobalPrivileges: EncodeStrings(adminGlobals()),
		},
		{
			Name: string(models.RoleMember), Description: "Create and run their own workspaces",
			Builtin:          true,
			GlobalPrivileges: EncodeStrings([]string{PWorkspacesCreate}),
		},
		{
			Name: string(models.RoleNone), Description: "Only workspaces they are invited to (default for new SSO users)",
			Builtin:          true,
			GlobalPrivileges: EncodeStrings(nil),
		},
	}
	for i := range builtins {
		var count int64
		if err := db.Model(&models.Role{}).Where("name = ?", builtins[i].Name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&builtins[i]).Error; err != nil {
				return err
			}
		}
	}
	// Reconcile the built-in Admin's global privileges every boot so upgrades drop
	// the owner-only privileges (branding/notifications/impersonate) from Admin.
	return db.Model(&models.Role{}).Where("name = ? AND builtin = ?", string(models.RoleAdmin), true).
		Update("global_privileges", EncodeStrings(adminGlobals())).Error
}

// adminGlobals is the built-in Admin's explicit global-privilege list: every
// global privilege except the owner-only ones (Admin is superuser, so it gets
// the rest implicitly; the owner-only set is deliberately excluded).
func adminGlobals() []string {
	out := make([]string, 0, len(GlobalPrivileges))
	for _, p := range GlobalPrivileges {
		if !IsOwnerOnlyGlobal(p) {
			out = append(out, p)
		}
	}
	return out
}

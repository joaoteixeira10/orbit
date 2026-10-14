// Instance privilege identifiers (must match the backend rbac package) and
// their display metadata. What someone may do inside a workspace is their role
// there (owner / editor / viewer), not one of these.
import { Users, ShieldCheck, Settings, Bell, ClipboardList, VenetianMask, LayoutGrid, type LucideIcon } from 'lucide-vue-next'

export const P = {
  usersManage: 'users.manage',
  rolesManage: 'roles.manage',
  auditRead: 'audit.read',
  workspacesCreate: 'workspaces.create',
  settingsBranding: 'settings.branding',
  settingsNotifications: 'settings.notifications',
  usersImpersonate: 'users.impersonate',
} as const

export interface PrivilegeMeta {
  id: string
  label: string
  desc?: string
  icon: LucideIcon
  // ownerOnly privileges are held by the instance owner by default and only the
  // owner can grant them via a role (and the superuser flag does NOT grant them).
  ownerOnly?: boolean
}

// OWNER_ONLY_PRIVS must match rbac.OwnerOnlyGlobals on the backend.
export const OWNER_ONLY_PRIVS: string[] = [P.settingsBranding, P.settingsNotifications, P.usersImpersonate]
export function isOwnerOnlyPriv(p: string): boolean {
  return OWNER_ONLY_PRIVS.includes(p)
}

export const GLOBAL_PRIVILEGES: PrivilegeMeta[] = [
  { id: P.workspacesCreate, label: 'Create workspaces', desc: 'Start their own workspaces (invited ones need nothing)', icon: LayoutGrid },
  { id: P.usersManage, label: 'Manage users', desc: 'Create, disable, reset and sign out accounts', icon: Users },
  { id: P.rolesManage, label: 'Manage roles', desc: 'Create roles and choose their privileges', icon: ShieldCheck },
  { id: P.auditRead, label: 'View audit log', desc: 'Read all audit events', icon: ClipboardList },
  { id: P.usersImpersonate, label: 'Impersonate users', desc: 'Sign in as another user (not owner/admins)', icon: VenetianMask, ownerOnly: true },
  { id: P.settingsBranding, label: 'Manage branding', desc: 'App name, accent, logo', icon: Settings, ownerOnly: true },
  { id: P.settingsNotifications, label: 'Manage notifications', desc: 'Notification channels', icon: Bell, ownerOnly: true },
]

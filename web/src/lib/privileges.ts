// Instance privilege identifiers (must match the backend rbac package) and
// their display metadata. What someone may do inside a workspace is their role
// there (owner / editor / viewer), not one of these.
import { Users, ShieldCheck, Settings, Bell, ClipboardList, VenetianMask, LayoutGrid, type LucideIcon } from 'lucide-vue-next'
import { i18n } from '@/i18n'

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

// A privilege's name and description are looked up when read, so they follow
// the interface language (messages: priv.<id>.label / .desc).
function priv(id: string, key: string, icon: LucideIcon, ownerOnly = false): PrivilegeMeta {
  return {
    id,
    icon,
    ownerOnly,
    get label() {
      return i18n.global.t(`priv.${key}.label`)
    },
    get desc() {
      return i18n.global.t(`priv.${key}.desc`)
    },
  }
}

export const GLOBAL_PRIVILEGES: PrivilegeMeta[] = [
  priv(P.workspacesCreate, 'workspacesCreate', LayoutGrid),
  priv(P.usersManage, 'usersManage', Users),
  priv(P.rolesManage, 'rolesManage', ShieldCheck),
  priv(P.auditRead, 'auditRead', ClipboardList),
  priv(P.usersImpersonate, 'usersImpersonate', VenetianMask, true),
  priv(P.settingsBranding, 'settingsBranding', Settings, true),
  priv(P.settingsNotifications, 'settingsNotifications', Bell, true),
]

// roleDescription is a role's description; the built-in roles' (seeded in
// English by the server, and not editable) are shown in the interface language.
const BUILTIN_ROLES = ['admin', 'member', 'none']
export function roleDescription(r: { name: string; description?: string; builtin?: boolean }): string {
  return r.builtin && BUILTIN_ROLES.includes(r.name) ? i18n.global.t(`priv.role.${r.name}`) : (r.description ?? '')
}

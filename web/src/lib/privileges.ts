// Privilege identifiers (must match the backend rbac package) + display metadata.
import {
  Users,
  ShieldCheck,
  Settings,
  Bell,
  ClipboardList,
  Network,
  Eye,
  Power,
  PowerOff,
  Hourglass,
  Scale,
  RotateCcw,
  Activity,
  VenetianMask,
  CheckCheck,
  FileBadge,
  FileKey,
  type LucideIcon,
} from 'lucide-vue-next'

export const P = {
  usersManage: 'users.manage',
  rolesManage: 'roles.manage',
  auditRead: 'audit.read',
  proxiesManage: 'proxies.manage',
  certsRead: 'certs.read',
  certsManage: 'certs.manage',
  settingsBranding: 'settings.branding',
  settingsNotifications: 'settings.notifications',
  usersImpersonate: 'users.impersonate',
  serversRead: 'servers.read',
  serversEnable: 'servers.enable',
  serversDisable: 'servers.disable',
  serversDrain: 'servers.drain',
  serversWeight: 'servers.weight',
  maintApprove: 'maint.approve',
  statsReset: 'stats.reset',
  trafficTest: 'traffic.test',
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
  { id: P.usersManage, label: 'Manage users', desc: 'Create, disable, reset and sign out accounts', icon: Users },
  { id: P.rolesManage, label: 'Manage roles', desc: 'Create roles and set their scope', icon: ShieldCheck },
  { id: P.auditRead, label: 'View audit log', desc: 'Read all audit events', icon: ClipboardList },
  { id: P.proxiesManage, label: 'Manage proxies', desc: 'Add and edit proxies and their agent tokens', icon: Network },
  { id: P.certsRead, label: 'See certificates', desc: 'The certificates the proxies and agents serve, and their expiry', icon: FileBadge },
  { id: P.certsManage, label: 'Manage certificates', desc: 'Replace them (upload or PKI Management), automatic renewal', icon: FileKey },
  { id: P.usersImpersonate, label: 'Impersonate users', desc: 'Sign in as another user (not owner/admins)', icon: VenetianMask, ownerOnly: true },
  { id: P.settingsBranding, label: 'Manage branding', desc: 'App name, accent, logo', icon: Settings, ownerOnly: true },
  { id: P.settingsNotifications, label: 'Manage notifications', desc: 'Notification channels', icon: Bell, ownerOnly: true },
]

// Server-scoped: granted on every server, or on the servers matching the
// grant's scope rules.
export const SCOPED_PRIVILEGES: PrivilegeMeta[] = [
  { id: P.serversRead, label: 'See servers', icon: Eye },
  { id: P.serversEnable, label: 'Enable (ready)', icon: Power },
  { id: P.serversDisable, label: 'Disable (maintenance)', icon: PowerOff },
  { id: P.serversDrain, label: 'Drain (HAProxy)', icon: Hourglass },
  { id: P.serversWeight, label: 'Change weights', icon: Scale },
  { id: P.maintApprove, label: 'Approve maintenance requests', desc: 'Approve or deny requests for maintenance and extensions', icon: CheckCheck },
  { id: P.statsReset, label: 'Reset counters', desc: 'Needs every server of the proxy in scope', icon: RotateCcw },
  { id: P.trafficTest, label: 'Traffic test', icon: Activity },
]

// Scope rule keys (must match rbac.RuleKeys).
export const RULE_KEYS = ['env', 'machine', 'service', 'proxy', 'backend'] as const

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { onLiveEvent } from '@/lib/live'
import { useFlash } from '@/lib/highlight'
import { usersApi, apiTokensAdminApi, uiSettingsApi, rolesApi, settingsApi, notifyApi, securityApi, oidcApi, featuresApi, ownerApi, type CreateUserInput, type RoleInput } from '@/api'
import type { User, Role, NotifConfig, OIDCConfig, APIToken } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { fmtDate } from '@/lib/format'
import { P, GLOBAL_PRIVILEGES, SCOPED_PRIVILEGES, RULE_KEYS } from '@/lib/privileges'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import RoleSelect from '@/components/ui/RoleSelect.vue'
import ProxiesAdmin from '@/components/admin/ProxiesAdmin.vue'
import { useProxiesStore } from '@/stores/proxies'
import { parseRules, matchesRules } from '@/lib/proxy'
import Toggle from '@/components/ui/Toggle.vue'
import BackupCard from '@/components/admin/BackupCard.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import {
  Plus, Trash2, KeyRound, Pencil, Send, Check, Copy,
  Users, ShieldCheck, Palette, Bell, Lock, Image as ImageIcon, SlidersHorizontal, Eye, LogIn, LogOut, VenetianMask,
  Crown, LockOpen, TriangleAlert, Terminal, UserX, UserCheck, Network, RefreshCw,
  Code2, Sparkles, Type, Info,
} from 'lucide-vue-next'
import { usePrefsStore } from '@/stores/prefs'
import { DEV_THEME_LIST, resolveCrest, resolveThemeAccent, type DevThemeId } from '@/lib/devThemes'

const auth = useAuthStore()
const toast = useToastStore()
const branding = useBrandingStore()
const prefs = usePrefsStore()
const router = useRouter()
const { t, locale } = useI18n()

// "3 days ago" in the UI language, for the last sign-in column.
function ago(iso: string): string {
  const secs = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  const rtf = new Intl.RelativeTimeFormat(locale.value, { numeric: 'auto' })
  const steps: [Intl.RelativeTimeFormatUnit, number][] = [['year', 31536000], ['month', 2592000], ['day', 86400], ['hour', 3600], ['minute', 60]]
  for (const [unit, n] of steps) if (Math.abs(secs) >= n) return rtf.format(Math.round(secs / n), unit)
  return rtf.format(0, 'minute')
}

type Tab = 'users' | 'roles' | 'tokens' | 'proxies' | 'branding' | 'notifications' | 'security' | 'sso' | 'owner'

// This view backs two top-level sections: the admin area (/admin) and the
// owner-only area (/owner), selected by the route's `area` meta. Each area
// exposes only its own tabs; the owner tabs (god-mode + the dangerous,
// instance-defining settings) live solely under /owner.
const route = useRoute()
const area = computed<'admin' | 'owner'>(() => (route.meta.area === 'owner' ? 'owner' : 'admin'))
const isOwnerArea = computed(() => area.value === 'owner')

// Grouped sidebar navigation (counts shown for Users/Roles).
const navGroups = computed(() => {
  if (isOwnerArea.value) {
    const owner: { id: Tab; label: string; icon: any; count?: number }[] = []
    if (auth.isOwner) {
      owner.push({ id: 'owner', label: t('admin.owner'), icon: Crown })
      owner.push({ id: 'branding', label: t('admin.branding'), icon: Palette })
      owner.push({ id: 'notifications', label: t('admin.notifications'), icon: Bell })
      owner.push({ id: 'security', label: t('admin.security'), icon: Lock })
      owner.push({ id: 'sso', label: t('admin.sso'), icon: LogIn })
    }
    return owner.length ? [{ title: t('admin.owner'), items: owner }] : []
  }
  const mgmt: { id: Tab; label: string; icon: any; count?: number }[] = []
  if (auth.can(P.usersManage)) mgmt.push({ id: 'users', label: t('admin.users'), icon: Users, count: users.value.length })
  if (auth.can(P.rolesManage)) mgmt.push({ id: 'roles', label: t('admin.roles'), icon: ShieldCheck, count: roles.value.length })
  if (auth.can(P.usersManage)) mgmt.push({ id: 'tokens', label: t('admin.apiTokens'), icon: Terminal })
  if (auth.can(P.proxiesManage)) mgmt.push({ id: 'proxies', label: t('nav.proxies'), icon: Network })
  const sys: { id: Tab; label: string; icon: any; count?: number }[] = []
  // Branding/notifications are owner concerns and live under the Owner section.
  // A delegated (non-owner) admin granted the owner-gated privilege still manages
  // them here, so the delegation feature keeps working.
  if (!auth.isOwner && auth.canOwnerGated(P.settingsBranding)) sys.push({ id: 'branding', label: t('admin.branding'), icon: Palette })
  if (!auth.isOwner && auth.canOwnerGated(P.settingsNotifications)) sys.push({ id: 'notifications', label: t('admin.notifications'), icon: Bell })
  const groups: { title: string; items: typeof mgmt }[] = []
  if (mgmt.length) groups.push({ title: 'Management', items: mgmt })
  if (sys.length) groups.push({ title: 'System', items: sys })
  return groups
})
const tabs = computed<Tab[]>(() => navGroups.value.flatMap((g) => g.items).map((i) => i.id))
const tab = ref<Tab>('users')
// Flat list for the horizontal nav bar.
const navItems = computed(() => navGroups.value.flatMap((g) => g.items))

const users = ref<User[]>([])
const roles = ref<Role[]>([])

async function loadUsers() {
  users.value = await usersApi.list()
}
async function loadRoles() {
  roles.value = await rolesApi.list()
}

function toggle<T>(arr: T[], v: T) {
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
}
function roleNames(u: User) {
  return (u.roles ?? []).map((r) => r.name).join(', ') || '—'
}

// ---------- Users ----------
const showUser = ref(false)
const userForm = reactive<CreateUserInput>({
  username: '',
  display_name: '',
  email: '',
  password: '',
  generate_password: true,
  must_change_password: true,
  otp_required: false,
  role_ids: [],
})
// One-time display of a generated password after creating a user.
const createdCreds = ref<{ username: string; password: string; reset?: boolean } | null>(null)
async function createUser() {
  try {
    const res = await usersApi.create(userForm)
    showUser.value = false
    if (res.generated_password) {
      createdCreds.value = { username: res.user.username, password: res.generated_password }
    } else {
      toast.success('User created')
    }
    Object.assign(userForm, {
      username: '', display_name: '', email: '', password: '', generate_password: true,
      must_change_password: true, otp_required: false, role_ids: [],
    })
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function copyCreds() {
  if (!createdCreds.value) return
  try {
    await navigator.clipboard.writeText(createdCreds.value.password)
    toast.success('Password copied')
  } catch {
    toast.error('Could not copy')
  }
}

const showEditUser = ref(false)
const editUserTarget = ref<User | null>(null)
const editUserForm = reactive<{
  display_name: string
  email: string
  disabled: boolean
  otp_required: boolean
  role_ids: string[]
}>({
  display_name: '',
  email: '',
  disabled: false,
  otp_required: false,
  role_ids: [],
})
function openEditUser(u: User) {
  editUserTarget.value = u
  editUserForm.display_name = u.display_name ?? ''
  editUserForm.email = u.email ?? ''
  editUserForm.disabled = u.disabled
  editUserForm.otp_required = u.otp_required
  editUserForm.role_ids = (u.roles ?? []).map((r) => r.id)
  showEditUser.value = true
}
async function saveEditUser() {
  if (!editUserTarget.value) return
  // If we're editing our own account, our privileges may have changed — refresh
  // them so privilege-gated UI (e.g. the Admin nav button) updates without a reload.
  const editingSelf = editUserTarget.value.id === auth.user?.id
  try {
    const oidc = editUserTarget.value.oidc
    await usersApi.update(editUserTarget.value.id, {
      // SSO accounts: identity fields come from the IdP.
      ...(oidc ? {} : { display_name: editUserForm.display_name, email: editUserForm.email }),
      // You can't disable your own account (the server refuses it too).
      ...(editingSelf ? {} : { disabled: editUserForm.disabled }),
      otp_required: editUserForm.otp_required,
      role_ids: editUserForm.role_ids,
    })
    toast.success('User updated')
    showEditUser.value = false
    await reloadUsersAndFlash()
    if (editingSelf) await auth.fetchMe()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function resetUserOtp() {
  if (!editUserTarget.value) return
  try {
    await usersApi.resetOtp(editUserTarget.value.id)
    toast.success('Two-factor reset for this user')
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function resetUserWebauthn() {
  if (!editUserTarget.value) return
  try {
    await usersApi.resetWebauthn(editUserTarget.value.id)
    toast.success('Passkeys removed for this user')
  } catch (e: any) {
    toast.error(e.message)
  }
}

const resetTarget = ref<User | null>(null)
const newPassword = ref('')
// An admin-set password is temporary by default: the user must replace it at
// the next sign-in. Resetting also signs the user out everywhere.
const resetMustChange = ref(true)
// Generate (the default) creates a temporary password shown once.
const resetGenerate = ref(true)
async function resetPassword() {
  if (!resetTarget.value) return
  const u = resetTarget.value
  try {
    if (resetGenerate.value) {
      const res = await usersApi.generatePassword(u.id)
      createdCreds.value = { username: u.username, password: res.generated_password, reset: true }
    } else {
      await usersApi.resetPassword(u.id, newPassword.value, resetMustChange.value)
      toast.success(t('admin.resetDone'))
    }
    resetTarget.value = null
    newPassword.value = ''
    resetMustChange.value = true
    resetGenerate.value = true
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function unlockUser(u: User) {
  try {
    await usersApi.unlock(u.id)
    toast.success(t('admin.unlockDone', { name: u.username }))
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Status chips for the users table, most important first.
type Chip = { label: string; tone: 'ok' | 'warn' | 'crit' | 'neutral'; title?: string }
function userChips(u: User): Chip[] {
  const chips: Chip[] = []
  if (u.disabled) chips.push({ label: t('admin.stDisabled'), tone: 'crit' })
  if (u.locked_until) chips.push({ label: t('admin.stLocked'), tone: 'crit', title: t('admin.lockedUntil', { when: fmtDate(u.locked_until) }) })
  if (!u.disabled && !u.locked_until) chips.push({ label: t('admin.stActive'), tone: 'ok' })
  if (u.must_change_password && !u.oidc) chips.push({ label: t('admin.stTempPassword'), tone: 'warn' })
  if (!u.oidc) {
    if (u.otp_enabled || u.has_passkey) chips.push({ label: t('admin.st2fa'), tone: 'ok', title: [u.otp_enabled && 'TOTP', u.has_passkey && 'passkey'].filter(Boolean).join(' + ') })
    else if (u.needs_otp_setup) chips.push({ label: t('admin.st2faPending'), tone: 'warn' })
  }
  return chips
}
const chipClass: Record<Chip['tone'], string> = {
  ok: 'bg-ok-bg text-ok-text',
  warn: 'bg-warn-bg text-warn-text',
  crit: 'bg-crit-bg text-crit-text',
  neutral: 'bg-surface-3 text-text-2',
}
// Disable (confirmed) signs the user out everywhere; enable is one click.
const disableTarget = ref<User | null>(null)
async function setDisabled(u: User, disabled: boolean) {
  disableTarget.value = null
  try {
    await usersApi.update(u.id, { disabled })
    toast.success(t(disabled ? 'admin.disabledDone' : 'admin.enabledDone', { name: u.username }))
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}
const logoutTarget = ref<User | null>(null)
async function confirmForceLogout() {
  if (!logoutTarget.value) return
  try {
    await usersApi.forceLogout(logoutTarget.value.id)
    toast.success(t('admin.forceLogoutDone', { name: logoutTarget.value.username }))
    logoutTarget.value = null
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Impersonation: shown only when the flag is on and the caller holds the
// privilege, never for yourself or a superuser (the owner is blocked server-
// side too — Owner isn't exposed on the user list).
function canImpersonate(u: User): boolean {
  if (!auth.features.impersonation || !auth.canOwnerGated(P.usersImpersonate)) return false
  if (u.id === auth.user?.id || u.disabled) return false
  if (u.roles?.some((r) => r.superuser) && !auth.isOwner) return false
  return true
}
async function impersonate(u: User) {
  try {
    await auth.startImpersonation(u.id)
    toast.success(t('impersonate.started', { user: u.username }))
    router.push({ name: 'dashboard' })
  } catch (e: any) {
    toast.error(e.message)
  }
}

const deleteUserTarget = ref<User | null>(null)
async function removeUser() {
  if (!deleteUserTarget.value) return
  try {
    await usersApi.remove(deleteUserTarget.value.id)
    toast.success('User deleted')
    deleteUserTarget.value = null
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---------- Roles ----------
// A server-scoped grant in the editor: every server, or the servers matching
// the scope rules (one per line in the textarea).
interface GrantForm {
  all_servers: boolean
  rulesText: string
  privileges: string[]
}
interface RoleForm {
  id?: string
  name: string
  description: string
  global_privileges: string[]
  grants: GrantForm[]
}
const showRole = ref(false)
const roleForm = reactive<RoleForm>({ name: '', description: '', global_privileges: [], grants: [] })

function newGrant(): GrantForm {
  return { all_servers: true, rulesText: '', privileges: [] }
}
// Privileges the current user may grant (owner-only ones excluded for non-owners,
// but any already on the role are preserved so a non-owner edit doesn't drop them).
function grantableGlobals(): string[] {
  return GLOBAL_PRIVILEGES.filter((p) => auth.isOwner || !p.ownerOnly || roleForm.global_privileges.includes(p.id)).map((p) => p.id)
}
const allGlobalSelected = computed(() => {
  const g = grantableGlobals()
  return g.length > 0 && g.every((id) => roleForm.global_privileges.includes(id))
})
function toggleAllGlobal() {
  const g = grantableGlobals()
  const lockedOn = roleForm.global_privileges.filter((id) => !g.includes(id)) // owner-only already set, keep
  roleForm.global_privileges = allGlobalSelected.value ? lockedOn : Array.from(new Set([...lockedOn, ...g]))
}
function openCreateRole() {
  Object.assign(roleForm, { id: undefined, name: '', description: '', global_privileges: [], grants: [newGrant()] })
  showRole.value = true
}
function openEditRole(r: Role) {
  roleForm.id = r.id
  roleForm.name = r.name
  roleForm.description = r.description
  roleForm.global_privileges = [...r.global_privileges]
  roleForm.grants = r.grants.map((g) => ({
    all_servers: g.all_servers,
    rulesText: (g.rules ?? []).join('\n'),
    privileges: [...g.privileges],
  }))
  if (roleForm.grants.length === 0) roleForm.grants = [newGrant()]
  showRole.value = true
}
async function saveRole() {
  const body: RoleInput = {
    name: roleForm.name,
    description: roleForm.description,
    global_privileges: roleForm.global_privileges,
    grants: roleForm.grants.map((g) => ({
      all_servers: g.all_servers,
      rules: g.all_servers ? [] : g.rulesText.split('\n').map((l) => l.trim()).filter(Boolean),
      privileges: g.privileges,
    })),
  }
  // Editing a role we currently hold changes our own privileges — refresh them.
  const affectsSelf = !!roleForm.id && (auth.user?.roles ?? []).some((r) => r.id === roleForm.id)
  try {
    if (roleForm.id) await rolesApi.update(roleForm.id, body)
    else await rolesApi.create(body)
    toast.success('Role saved')
    showRole.value = false
    await reloadRolesAndFlash()
    if (affectsSelf) await auth.fetchMe()
  } catch (e: any) {
    toast.error(e.message)
  }
}
const deleteRoleTarget = ref<Role | null>(null)
async function removeRole() {
  if (!deleteRoleTarget.value) return
  const affectsSelf = (auth.user?.roles ?? []).some((r) => r.id === deleteRoleTarget.value!.id)
  try {
    await rolesApi.remove(deleteRoleTarget.value.id)
    toast.success('Role deleted')
    deleteRoleTarget.value = null
    await loadRoles()
    await loadUsers()
    if (affectsSelf) await auth.fetchMe()
  } catch (e: any) {
    toast.error(e.message)
  }
}
// Role members come from the user list (needs users.manage); membership is
// changed through the user's roles, so the server's escalation guards apply.
const canManageMembers = computed(() => auth.can(P.usersManage))
const roleMembers = (r: Role) => users.value.filter((u) => u.roles?.some((x) => x.id === r.id))
const nonMembers = (r: Role) => users.value.filter((u) => !u.roles?.some((x) => x.id === r.id))
const addMember = reactive<Record<string, string | undefined>>({})
const privLabel = (id: string) => [...GLOBAL_PRIVILEGES, ...SCOPED_PRIVILEGES].find((p) => p.id === id)?.label ?? id
async function setMember(u: User, r: Role, on: boolean) {
  const ids = (u.roles ?? []).map((x) => x.id).filter((id) => id !== r.id)
  if (on) ids.push(r.id)
  try {
    await usersApi.update(u.id, { role_ids: ids })
    toast.success(t(on ? 'admin.memberAdded' : 'admin.memberRemoved', { user: u.username, role: r.name }))
    await reloadUsersAndFlash()
    if (u.id === auth.user?.id) await auth.fetchMe()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function addMemberTo(r: Role) {
  const u = users.value.find((x) => x.id === addMember[r.id])
  addMember[r.id] = undefined
  if (u) await setMember(u, r, true)
}

// ---------- API tokens (every user's) ----------
const allTokens = ref<APIToken[]>([])
const revokeTarget = ref<APIToken | null>(null)
async function loadAllTokens() {
  try {
    allTokens.value = await apiTokensAdminApi.list()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function revokeToken() {
  const tk = revokeTarget.value
  revokeTarget.value = null
  if (!tk) return
  try {
    await apiTokensAdminApi.revoke(tk.id)
    toast.success(t('admin.revoked', { name: tk.name }))
    await loadAllTokens()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Live "matches N servers" under a grant's rules (against the servers you can see).
const px = useProxiesStore()

// Owner → Live updates: the refresh interval.
const refreshSeconds = ref(10)
watch(
  () => px.data?.ui?.refresh_seconds,
  (v) => v && (refreshSeconds.value = v),
  { immediate: true },
)
const refreshIsCustom = computed(() => !!px.data?.ui && px.data.ui.refresh_seconds !== px.data.ui.default_refresh_seconds)
async function saveRefresh() {
  const n = Math.round(refreshSeconds.value)
  if (!(n >= 1 && n <= 300)) {
    toast.error(t('owner.refreshRange'))
    return
  }
  try {
    await uiSettingsApi.setRefresh(n)
    toast.success(t('owner.refreshSaved', { n }))
    await px.fetch()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function resetRefresh() {
  try {
    await uiSettingsApi.setRefresh(0)
    await px.fetch()
    toast.success(t('owner.refreshSaved', { n: px.data?.ui?.refresh_seconds ?? 10 }))
  } catch (e: any) {
    toast.error(e.message)
  }
}
function ruleMatch(text: string): { hits: string[]; error: string } {
  const { rules, error } = parseRules(text)
  if (error) return { hits: [], error }
  return { hits: [...new Set(px.targets.filter((x) => matchesRules(x, rules)).map((x) => `${x.label} @ ${x.proxy}`))], error: '' }
}

function roleSummary(r: Role): string {
  const parts: string[] = []
  if (r.superuser) return 'Full access (superuser)'
  if (r.global_privileges.length) parts.push(`${r.global_privileges.length} global`)
  if (r.grants.length) parts.push(`${r.grants.length} server grant(s)`)
  return parts.join(' · ') || 'no privileges'
}

// ---------- Branding ----------
const accentSwatches = ['#07cab3', '#185fa5', '#7c3aed', '#16a34a', '#d97706', '#dc2626', '#db2777']
const brandForm = reactive({
  app_name: '',
  accent_color: '',
  logo_data_url: '',
  favicon_data_url: '',
  show_app_name: true,
  show_logo: true,
  // Developer themes (a closed curated set; the accent is locked to the theme while on).
  developer_themes_enabled: false,
  developer_theme: 'blue' as DevThemeId,
  asset_source_logo: 'theme' as 'theme' | 'keep',
  asset_source_wordmark: 'theme' as 'theme' | 'keep',
  asset_source_favicon: 'theme' as 'theme' | 'keep',
})
const customLoginMessage = ref(false)
const loginMessage = ref('')

// The curated themes (the grid) and the one selected.
const devThemeList = DEV_THEME_LIST
const activeTheme = computed(() => DEV_THEME_LIST.find((x) => x.id === brandForm.developer_theme) ?? DEV_THEME_LIST[0])
// Accent for the preview: the theme's when the flag is on (Ink flips with
// light/dark), else the owner's chosen accent, or none = default.
const previewAccent = computed(() => {
  if (brandForm.developer_themes_enabled) {
    const r = resolveThemeAccent(activeTheme.value, prefs.isDark())
    return { accent: r.accent, strong: r.accentStrong, fg: r.onAccent }
  }
  const a = brandForm.accent_color
  return a ? { accent: a, strong: `color-mix(in srgb, ${a} 78%, #000)`, fg: '#fff' } : null
})
// The selected theme's crest, and which mark / wordmark / favicon the preview shows.
const previewCrest = computed(() => resolveCrest(activeTheme.value, prefs.isDark()))
const previewLogoTheme = computed(() => brandForm.developer_themes_enabled && brandForm.asset_source_logo === 'theme')
const previewWordTheme = computed(() => brandForm.developer_themes_enabled && brandForm.asset_source_wordmark === 'theme')
const previewFavTheme = computed(() => brandForm.developer_themes_enabled && brandForm.asset_source_favicon === 'theme')

// The Live preview button reuses the real .btn-primary, but with the *previewed*
// accent vars scoped to it (same derivation the branding store applies on save),
// so it renders identically to the app's buttons before you save. Empty accent =
// no override → the default-theme button.
const previewBtnStyle = computed(() => {
  const p = previewAccent.value
  const s: Record<string, string> = {}
  if (p) {
    s['--accent'] = p.accent
    s['--accent-strong'] = p.strong
    s['--accent-fg'] = p.fg
  }
  return s
})
function loadBrandForm() {
  brandForm.app_name = branding.appName
  brandForm.accent_color = branding.accentColor
  brandForm.logo_data_url = branding.logoDataURL
  brandForm.favicon_data_url = branding.faviconDataURL
  brandForm.show_app_name = branding.showAppName
  brandForm.show_logo = branding.showLogo
  brandForm.developer_themes_enabled = branding.devThemesEnabled
  brandForm.developer_theme = branding.devTheme
  brandForm.asset_source_logo = branding.assetSourceLogo
  brandForm.asset_source_wordmark = branding.assetSourceWordmark
  brandForm.asset_source_favicon = branding.assetSourceFavicon
  loginMessage.value = branding.loginMessage
  customLoginMessage.value = !!branding.loginMessage
}
function onFile(e: Event, key: 'logo_data_url' | 'favicon_data_url') {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    brandForm[key] = reader.result as string
  }
  reader.readAsDataURL(file)
}
async function saveBranding() {
  try {
    const b = await settingsApi.update({
      ...brandForm,
      login_message: customLoginMessage.value ? loginMessage.value : '',
    })
    branding.apply(b)
    toast.success('Branding saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---------- Notification channels ----------
const notifCfg = ref<NotifConfig | null>(null)
const testing = ref('')
async function loadNotifConfig() {
  const cfg = await notifyApi.getConfig()
  // Tolerate older payloads.
  const defaults: NotifConfig['events'] = { server_down: false, proxy_down: false, changes: false, maint_expiring: false, drift: false, requests: false, certificates: false, channels: null, emails: '' }
  cfg.events = Object.assign(defaults, cfg.events || {})
  if (!cfg.webhook) cfg.webhook = { enabled: false, url: '', secret: '' }
  if (!cfg.teams) cfg.teams = { enabled: false, webhook_url: '' }
  if (!cfg.slack) cfg.slack = { enabled: false, webhook_url: '' }
  notifCfg.value = cfg
}
// Server events: the toggles, and the shared channels they go to.
const serverEvents = [
  { key: 'server_down', label: 'A server goes down, and is back', hint: 'Fails its health checks (repeats within 2 minutes are held back)' },
  { key: 'proxy_down', label: 'An agent is unreachable, and is back', hint: 'Two failed polls in a row' },
  { key: 'changes', label: 'Changes', hint: 'Enable, disable, drain, restore — by a user, lbctl or Orbit itself (timers, drains), with who and why' },
  { key: 'maint_expiring', label: 'A timed maintenance ends soon', hint: '15 minutes before, with who set it and why' },
  { key: 'requests', label: 'Maintenance requests', hint: 'Requests, approvals and denials, grace periods for pending extensions (approvers and requesters get them by email anyway)' },
  { key: 'drift', label: 'Drift and failures', hint: 'A server is not in its desired state; a timer, a drain or an automatic restore failed' },
  { key: 'certificates', label: 'Certificates', hint: 'A certificate a proxy (or agent) serves expires soon, was replaced, or its automatic renewal failed (with the certificates feature on)' },
] as const
const sharedChannels = [
  { key: 'discord', label: 'Discord' },
  { key: 'telegram', label: 'Telegram' },
  { key: 'ntfy', label: 'ntfy' },
  { key: 'gotify', label: 'Gotify' },
  { key: 'teams', label: 'Teams' },
  { key: 'slack', label: 'Slack' },
]
const enabledShared = computed(() => sharedChannels.filter((c) => (notifCfg.value as any)?.[c.key]?.enabled))
function eventChannelOn(key: string) {
  const ch = notifCfg.value?.events.channels
  return ch == null || ch.includes(key)
}
function toggleEventChannel(key: string, on: boolean) {
  const ev = notifCfg.value!.events
  const cur = ev.channels ?? enabledShared.value.map((c) => c.key)
  ev.channels = on ? [...new Set([...cur, key])] : cur.filter((c) => c !== key)
}
async function saveNotifConfig() {
  if (!notifCfg.value) return
  try {
    notifCfg.value = await notifyApi.updateConfig(notifCfg.value)
    toast.success('Notification channels saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function testChannel(ch: string) {
  if (!notifCfg.value) return
  testing.value = ch
  try {
    // Persist current edits first so the test uses the on-screen config.
    notifCfg.value = await notifyApi.updateConfig(notifCfg.value)
    await notifyApi.test(ch)
    toast.success(`Test sent via ${ch}`)
  } catch (e: any) {
    toast.error(`Test failed: ${e.message}`)
  } finally {
    testing.value = ''
  }
}

// ---------- Security / login policy ----------
const otpEnforced = ref(false)
const hideLocalLogin = ref(false)
const passwordlessEnabled = ref(true)
const autoSso = ref(false)
const secLoaded = ref(false)
async function loadSecurity() {
  const c = await securityApi.get()
  otpEnforced.value = c.otp_enforced
  hideLocalLogin.value = c.hide_local_login
  passwordlessEnabled.value = c.passwordless_enabled
  autoSso.value = c.auto_sso
  secLoaded.value = true
}
async function setEnforce(v: boolean) {
  try {
    otpEnforced.value = (await securityApi.update({ otp_enforced: v })).otp_enforced
    toast.success(v ? 'Two-factor is now enforced' : 'Two-factor enforcement disabled')
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setHideLocalLogin(v: boolean) {
  try {
    hideLocalLogin.value = (await securityApi.update({ hide_local_login: v })).hide_local_login
    toast.success('Saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setPasswordless(v: boolean) {
  try {
    passwordlessEnabled.value = (await securityApi.update({ passwordless_enabled: v })).passwordless_enabled
    toast.success('Saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setAutoSso(v: boolean) {
  try {
    autoSso.value = (await securityApi.update({ auto_sso: v })).auto_sso
    toast.success('Saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}
// ---------- SSO / OIDC ----------
const oidcCfg = ref<OIDCConfig | null>(null)
const oidcBusy = ref(false)
async function loadOIDC() {
  oidcCfg.value = await oidcApi.getConfig()
}
async function saveOIDC() {
  if (!oidcCfg.value) return
  oidcBusy.value = true
  try {
    oidcCfg.value = await oidcApi.updateConfig(oidcCfg.value)
    await branding.load() // refresh the public oidc_enabled flag
    toast.success('SSO settings saved')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    oidcBusy.value = false
  }
}
async function testOIDC() {
  if (!oidcCfg.value?.issuer) return
  try {
    await oidcApi.testDiscovery(oidcCfg.value.issuer)
    toast.success('Discovery succeeded — issuer looks good')
  } catch (e: any) {
    toast.error(e.message)
  }
}


// Keep the active tab valid for the current area, and fire its lazy loader —
// including when navigating between /admin and /owner (same component reused).
watch(
  tabs,
  (list) => {
    if (list.length && !list.includes(tab.value)) switchTab(list[0])
  },
  { immediate: true },
)

onMounted(async () => {
  loadBrandForm()
  if (auth.can(P.usersManage)) await loadUsers()
  if (auth.can(P.rolesManage) || auth.can(P.usersManage)) await loadRoles()
  await loadSecurityAlerts()
})

// Live updates: when any admin creates/edits/deletes a role, refetch the roles
// list and briefly glow the rows that appeared or changed.
const { flash: flashRole, isFlashing: roleFlashing } = useFlash()
const roleSig = (r: Role) => JSON.stringify(r)
// Reload roles and glow rows that appeared or changed — used for our own
// mutations and for live events alike, so the actor sees the highlight too
// (their own reload would otherwise hide the diff).
async function reloadRolesAndFlash() {
  const before = new Map(roles.value.map((r) => [r.id, roleSig(r)]))
  await loadRoles()
  flashRole(roles.value.filter((r) => before.get(r.id) !== roleSig(r)).map((r) => r.id))
}
const offRolesLive = onLiveEvent('roles', async () => {
  if (!(auth.can(P.rolesManage) || auth.can(P.usersManage))) return
  await reloadRolesAndFlash()
})
onUnmounted(() => offRolesLive())

// Live updates: when any admin creates/edits/deletes a user, refetch the user
// list and glow the rows that appeared or changed (roles, disabled, or email).
const { flash: flashUser, isFlashing: userFlashing } = useFlash()
const userSig = (u: User) =>
  `${u.disabled}|${(u.roles ?? []).map((r) => r.id).sort().join(',')}|${u.email ?? ''}`
async function reloadUsersAndFlash() {
  const before = new Map(users.value.map((u) => [u.id, userSig(u)]))
  await loadUsers()
  flashUser(users.value.filter((u) => before.get(u.id) !== userSig(u)).map((u) => u.id))
}
const offUsersLive = onLiveEvent('users', async () => {
  if (!auth.can(P.usersManage)) return
  await reloadUsersAndFlash()
})
onUnmounted(() => offUsersLive())

// ---------- Owner (god mode) ----------
const featureMeta: { key: string; label: string; hint: string }[] = [
  { key: 'sessions', label: 'Active sessions', hint: 'Let users see and revoke their signed-in devices/sessions.' },
  { key: 'impersonation', label: 'User impersonation', hint: 'Let admins with the “Impersonate users” privilege sign in as another user to diagnose issues.' },
  { key: 'rolling', label: 'Rolling actions', hint: 'Roll… on an Operations card: one server at a time — out of rotation (draining first on HAProxy), wait for Next (or N minutes, or lbctl next from a deploy), back, healthy, next. Stops if a server doesn\'t come back.' },
  { key: 'certificates', label: 'Certificates (on by default)', hint: 'The Certificates page: what every proxy and agent serves, replacing it through the agents (upload or PKI Management), automatic renewal and expiry alerts. Off: the page, its API and the background checks are gone.' },
  { key: 'weights', label: 'Weights (canary releases)', hint: 'Holders of servers.weight change server weights from a backend\'s Weights… (HAProxy live, nginx through its upstream files), for example to send 10% of new traffic to a new version.' },
  { key: 'step_up_auth', label: 'Step-up re-authentication', hint: 'Require a fresh password/passkey confirmation before dangerous actions (security policy and SSO changes, owner transfer, impersonation).' },
]
const featureFlags = ref<Record<string, boolean>>({ ...auth.features })
async function toggleFeature(key: string, on: boolean) {
  const prev = featureFlags.value[key] ?? false
  featureFlags.value = { ...featureFlags.value, [key]: on }
  try {
    const updated = await featuresApi.update({ [key]: on })
    featureFlags.value = updated
    auth.features = updated // keep the app-wide flags in sync
    toast.success(on ? 'Feature enabled' : 'Feature disabled')
  } catch (e: any) {
    featureFlags.value = { ...featureFlags.value, [key]: prev } // revert on failure
    toast.error(e.message)
  }
}

// Owner-managed security alerts (broadcast on high-signal actions).
const securityAlertMeta: { key: string; label: string }[] = [
  { key: 'impersonation', label: 'Impersonation started' },
  { key: 'owner_transfer', label: 'Ownership transferred' },
  { key: 'admin_grant', label: 'Admin role granted' },
  { key: 'config_change', label: 'Security/SSO config changed' },
  { key: 'refresh_reuse', label: 'Session token reuse (possible theft)' },
  { key: 'backup_export', label: 'An encrypted backup is downloaded' },
]
const alertEvents = ref<Record<string, boolean>>({})
const alertChannels = ref<string[]>([])
const alertAvailable = ref<string[]>([])
const alertEmail = ref('')
async function loadSecurityAlerts() {
  if (!auth.isOwner) return
  try {
    const r = await ownerApi.getSecurityAlerts()
    alertEvents.value = r.events
    alertAvailable.value = r.available_channels
    alertChannels.value = r.channels ?? [...r.available_channels] // null = all enabled
    alertEmail.value = r.email
  } catch {
    /* not owner / unavailable */
  }
}
async function toggleSecurityAlert(key: string, on: boolean) {
  const prev = alertEvents.value[key] ?? false
  alertEvents.value = { ...alertEvents.value, [key]: on }
  try {
    await ownerApi.updateSecurityAlerts({ events: { [key]: on } })
  } catch (e: any) {
    alertEvents.value = { ...alertEvents.value, [key]: prev }
    toast.error(e.message)
  }
}
async function toggleAlertChannel(ch: string, on: boolean) {
  const next = on ? [...new Set([...alertChannels.value, ch])] : alertChannels.value.filter((c) => c !== ch)
  const prev = alertChannels.value
  alertChannels.value = next
  try {
    await ownerApi.updateSecurityAlerts({ channels: next })
  } catch (e: any) {
    alertChannels.value = prev
    toast.error(e.message)
  }
}
async function saveAlertEmail() {
  try {
    await ownerApi.updateSecurityAlerts({ email: alertEmail.value })
    toast.success('Alert recipient saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}

const transferTarget = ref<string | null>(null)
const transferConfirm = ref(false)
const transferCandidates = computed(() => users.value.filter((u) => u.id !== auth.user?.id && !u.disabled))
const transferName = computed(() => transferCandidates.value.find((u) => u.id === transferTarget.value)?.username ?? '')
async function runTransfer() {
  transferConfirm.value = false
  if (!transferTarget.value) return
  try {
    await ownerApi.transfer(transferTarget.value)
    toast.success('Ownership transferred')
    await auth.fetchMe() // we're no longer the owner → the whole Owner area disappears
    transferTarget.value = null
    if (!auth.isOwner) router.push({ name: 'dashboard' })
  } catch (e: any) {
    toast.error(e.message)
  }
}

function switchTab(t: Tab) {
  tab.value = t
  if (t === 'tokens') loadAllTokens()
  if (t === 'branding') loadBrandForm()
  if (t === 'notifications' && !notifCfg.value) loadNotifConfig()
  if (t === 'security' && !secLoaded.value) loadSecurity()
  if (t === 'sso' && !oidcCfg.value) loadOIDC()
}
</script>

<template>
  <div>
    <header class="mb-4">
      <h1 class="text-[19px] font-semibold tracking-tight">{{ isOwnerArea ? t('admin.ownerTitle') : t('admin.title') }}</h1>
      <p class="mt-1 text-[13px] text-text-2">{{ isOwnerArea ? t('admin.ownerSubtitle') : t('admin.subtitle') }}</p>
    </header>

    <!-- Section tabs (v0's sub-tabs) -->
    <div class="subtabs">
      <button v-for="it in navItems" :key="it.id" class="subtab" :class="{ on: tab === it.id }" @click="switchTab(it.id)">
        <component :is="it.icon" :size="15" />
        <span>{{ it.label }}</span>
        <span v-if="it.count !== undefined" class="count">{{ it.count }}</span>
      </button>
    </div>

    <!-- ===== Users ===== -->
    <section v-if="tab === 'users'">
      <div class="mb-3 flex justify-end">
        <button class="btn-primary" @click="showUser = true"><Plus class="h-4 w-4" /> {{ t('admin.newUser') }}</button>
      </div>
      <div class="card overflow-hidden">
        <div class="overflow-x-auto"><table class="w-full min-w-[640px]">
          <thead class="border-b border-border">
            <tr><th class="th">{{ t('admin.colUsername') }}</th><th class="th">{{ t('common.type') }}</th><th class="th">{{ t('admin.colEmail') }}</th><th class="th">{{ t('admin.colRoles') }}</th><th class="th">{{ t('common.status') }}</th><th class="th">{{ t('admin.colLastLogin') }}</th><th class="th text-right">{{ t('common.actions') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="u in users" :key="u.id" class="border-b border-line-soft last:border-0" :class="userFlashing(u.id) && 'row-flash'">
              <td class="td">
                <div class="font-medium">
                  {{ u.username }}
                  <span v-if="u.id === auth.user?.id" class="ml-1 rounded bg-accent-bg px-1.5 py-0.5 text-[10px] font-medium text-accent-text">{{ t('admin.you') }}</span>
                </div>
                <div v-if="u.display_name && u.display_name !== u.username" class="text-[12px] text-text-3">{{ u.display_name }}</div>
              </td>
              <td class="td">
                <span
                  class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                  :class="u.oidc ? 'bg-accent-bg text-accent-text' : 'bg-surface-3 text-text-2'"
                >{{ u.oidc ? 'SSO' : t('admin.local') }}</span>
              </td>
              <td class="td text-text-2">{{ u.email || '—' }}</td>
              <td class="td text-text-2">{{ roleNames(u) }}</td>
              <td class="td">
                <div class="flex flex-wrap gap-1">
                  <span v-for="c in userChips(u)" :key="c.label" class="rounded-full px-2 py-0.5 text-[11px] font-medium" :class="chipClass[c.tone]" :title="c.title">{{ c.label }}</span>
                </div>
              </td>
              <td class="td whitespace-nowrap text-text-2" :title="u.last_login_at ? fmtDate(u.last_login_at) : ''">{{ u.last_login_at ? ago(u.last_login_at) : t('admin.never') }}</td>
              <td class="td">
                <div class="flex justify-end gap-1">
                  <button class="btn-ghost px-2 py-1" :title="t('admin.edit')" @click="openEditUser(u)"><Pencil class="h-4 w-4" /></button>
                  <button v-if="u.locked_until" class="btn-ghost px-2 py-1" :title="t('admin.unlock')" @click="unlockUser(u)"><LockOpen class="h-4 w-4" /></button>
                  <button v-if="!u.oidc" class="btn-ghost px-2 py-1" :title="t('admin.resetPassword')" @click="resetTarget = u"><KeyRound class="h-4 w-4" /></button>
                  <button class="btn-ghost px-2 py-1" :title="t('admin.forceLogout')" @click="logoutTarget = u"><LogOut class="h-4 w-4" /></button>
                  <template v-if="u.id !== auth.user?.id">
                    <button v-if="u.disabled" class="btn-ghost px-2 py-1 text-ok-text" :title="t('admin.enableUser')" @click="setDisabled(u, false)"><UserCheck class="h-4 w-4" /></button>
                    <button v-else class="btn-ghost px-2 py-1 text-warn-text" :title="t('admin.disableUser')" @click="disableTarget = u"><UserX class="h-4 w-4" /></button>
                  </template>
                  <button v-if="canImpersonate(u)" class="btn-ghost px-2 py-1" :title="t('impersonate.action')" @click="impersonate(u)"><VenetianMask class="h-4 w-4" /></button>
                  <button v-if="u.id !== auth.user?.id" class="btn-ghost px-2 py-1 text-crit-text" :title="t('common.delete')" @click="deleteUserTarget = u"><Trash2 class="h-4 w-4" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table></div>
      </div>
    </section>

    <!-- ===== Roles: v0's group cards (members | access) ===== -->
    <section v-else-if="tab === 'roles'">
      <div class="mb-3 flex justify-end">
        <button class="btn-primary" @click="openCreateRole"><Plus class="h-4 w-4" /> {{ t('admin.newRole') }}</button>
      </div>
      <div class="flex flex-col gap-4">
        <div v-for="r in roles" :key="r.id" class="card role-card overflow-hidden" :class="roleFlashing(r.id) && 'row-flash'">
          <div class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
            <div class="min-w-0">
              <h3 class="font-mono text-[13.5px] font-semibold">
                {{ r.name }}
                <span v-if="r.builtin" class="ml-1 rounded bg-surface-3 px-1.5 py-0.5 font-sans text-[10px] font-medium uppercase text-text-3">{{ t('admin.builtin') }}</span>
              </h3>
              <div class="text-[12px] text-text-3">{{ r.description || roleSummary(r) }}</div>
            </div>
            <div class="flex gap-1">
              <button class="btn-ghost px-2 py-1" :disabled="r.builtin" :title="r.builtin ? t('admin.builtinImmutable') : t('admin.edit')" @click="!r.builtin && openEditRole(r)"><Pencil class="h-4 w-4" /></button>
              <button class="btn-ghost px-2 py-1 text-crit-text" :disabled="r.builtin" :title="t('common.delete')" @click="!r.builtin && (deleteRoleTarget = r)"><Trash2 class="h-4 w-4" /></button>
            </div>
          </div>
          <div class="grid md:grid-cols-2">
            <!-- Members (needs users.manage to list and change) -->
            <div class="min-w-0 border-border p-4 md:border-r">
              <span class="cap mb-2 block">{{ t('admin.members') }}<template v-if="canManageMembers"> · {{ roleMembers(r).length }}</template></span>
              <template v-if="canManageMembers">
                <div class="mb-2.5 flex flex-wrap gap-1.5">
                  <span v-for="u in roleMembers(r)" :key="u.id" class="inline-flex items-center gap-0.5 rounded-full border border-border bg-surface-2 py-0.5 pl-2.5 pr-1 font-mono text-[12px]">
                    {{ u.username }}
                    <button class="px-1 text-text-3 hover:text-crit-text" :title="t('admin.removeMember')" @click="setMember(u, r, false)">×</button>
                  </span>
                  <span v-if="!roleMembers(r).length" class="text-[12px] text-text-3">{{ t('admin.noMembers') }}</span>
                </div>
                <div v-if="nonMembers(r).length" class="flex gap-1.5">
                  <select v-model="addMember[r.id]" class="input min-w-0 flex-1 py-1">
                    <option :value="undefined">{{ t('admin.addMember') }}</option>
                    <option v-for="u in nonMembers(r)" :key="u.id" :value="u.id">{{ u.username }}</option>
                  </select>
                  <button class="btn-ghost" :disabled="!addMember[r.id]" @click="addMemberTo(r)">{{ t('admin.add') }}</button>
                </div>
              </template>
              <p v-else class="text-[12px] text-text-3">{{ t('admin.membersNeedUsers') }}</p>
            </div>
            <!-- Access -->
            <div class="min-w-0 p-4">
              <span class="cap mb-2 block">{{ t('admin.access') }}</span>
              <p v-if="r.superuser" class="text-[12.5px] font-medium text-violet">{{ t('admin.fullAccess') }}</p>
              <template v-else>
                <div v-if="r.global_privileges.length" class="mb-2 flex flex-wrap gap-1">
                  <span v-for="p in r.global_privileges" :key="p" class="rounded bg-surface-3 px-1.5 py-0.5 text-[11.5px] text-text-2">{{ privLabel(p) }}</span>
                </div>
                <div v-for="(g, i) in r.grants" :key="i" class="mb-1.5 text-[12px]">
                  <span v-if="g.all_servers" class="font-medium">{{ t('admin.everyServer') }}</span>
                  <span v-else class="inline-flex flex-wrap gap-1 align-middle">
                    <code v-for="rule in g.rules" :key="rule" class="rounded border border-border bg-surface-2 px-1.5 text-[11.5px]">{{ rule }}</code>
                  </span>
                  <span class="text-text-3"> → </span>
                  <span class="text-text-2">{{ g.privileges.map(privLabel).join(', ') }}</span>
                </div>
                <p v-if="!r.global_privileges.length && !r.grants.length" class="text-[12px] text-text-3">{{ t('admin.noPrivileges') }}</p>
              </template>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ===== Proxies (registry) ===== -->
    <section v-else-if="tab === 'proxies'"><ProxiesAdmin /></section>

    <!-- ===== API tokens (every user's) ===== -->
    <section v-else-if="tab === 'tokens'">
      <p class="mb-3 text-[12.5px] text-text-2">{{ t('admin.tokensHint') }}</p>
      <div class="card overflow-hidden">
        <div class="overflow-x-auto"><table class="w-full min-w-[720px]">
          <thead class="border-b border-border">
            <tr><th class="th">{{ t('common.name') }}</th><th class="th">{{ t('admin.colOwner') }}</th><th class="th">{{ t('admin.colScope') }}</th><th class="th">{{ t('admin.colLastUsed') }}</th><th class="th">{{ t('admin.colExpires') }}</th><th class="th text-right">{{ t('common.actions') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="tk in allTokens" :key="tk.id" class="border-b border-line-soft last:border-0">
              <td class="td"><span class="font-medium">{{ tk.name }}</span> <span class="font-mono text-[11px] text-text-3">{{ tk.prefix }}…</span></td>
              <td class="td font-mono">{{ tk.username || '—' }}</td>
              <td class="td">
                <span v-if="!tk.privileges?.length && !tk.rules?.length" class="text-text-3">{{ t('admin.tokenFull') }}</span>
                <span v-else class="flex flex-wrap gap-1">
                  <span v-for="p in tk.privileges ?? []" :key="p" class="rounded bg-surface-3 px-1.5 text-[11.5px]">{{ privLabel(p) }}</span>
                  <code v-for="rule in tk.rules ?? []" :key="rule" class="rounded border border-border bg-surface-2 px-1.5 text-[11.5px]">{{ rule }}</code>
                </span>
              </td>
              <td class="td text-text-2">{{ tk.last_used_at ? ago(tk.last_used_at) : t('admin.never') }}</td>
              <td class="td text-text-2">{{ tk.expires_at ? fmtDate(tk.expires_at) : t('admin.never') }}</td>
              <td class="td text-right">
                <button class="btn-ghost px-2 py-1 text-crit-text" :title="t('admin.revoke')" @click="revokeTarget = tk"><Trash2 class="h-4 w-4" /></button>
              </td>
            </tr>
            <tr v-if="!allTokens.length"><td class="td text-text-3" colspan="6">{{ t('admin.noTokens') }}</td></tr>
          </tbody>
        </table></div>
      </div>
    </section>

    <!-- ===== Branding ===== -->
    <section v-else-if="tab === 'branding'" class="grid items-start gap-5 lg:grid-cols-2">
      <!-- LEFT column: Identity -->
      <div class="flex flex-col gap-5">
      <!-- Identity -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><SlidersHorizontal class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Identity</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div><label class="label">Application name</label><input v-model="brandForm.app_name" class="input" placeholder="Orbit" /></div>
          <div>
            <div class="lockable" :class="{ locked: brandForm.developer_themes_enabled }">
              <label class="label">Accent colour</label>
              <div class="flex items-center gap-2">
                <input v-model="brandForm.accent_color" type="color" class="h-10 w-12 shrink-0 rounded-lg border border-border bg-surface-2" />
                <input v-model="brandForm.accent_color" class="input font-mono" placeholder="#07cab3" />
              </div>
              <div class="mt-2 flex gap-2">
                <button
                  v-for="sw in accentSwatches"
                  :key="sw"
                  type="button"
                  class="h-6 w-6 rounded-full border-2 transition"
                  :class="brandForm.accent_color.toLowerCase() === sw ? 'border-text' : 'border-transparent'"
                  :style="{ backgroundColor: sw }"
                  @click="brandForm.accent_color = sw"
                />
              </div>
            </div>
            <div class="lock-note" :class="{ show: brandForm.developer_themes_enabled }">
              <Info class="h-4 w-4 shrink-0" />
              <span>Accent colour is controlled by the active developer theme. Pick a theme below, or turn the flag off to choose your own.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Developer themes -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <Code2 class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Developer themes</h3>
          <span class="ml-1 rounded-full px-2 py-0.5 text-[11px] font-semibold" :class="brandForm.developer_themes_enabled ? 'bg-accent-bg text-accent-text' : 'bg-surface-3 text-text-3'">{{ brandForm.developer_themes_enabled ? 'On' : 'Off' }}</span>
        </div>
        <div class="p-5">
          <div class="dev-master">
            <div class="flex items-center gap-[13px]">
              <div class="dm-ico"><Sparkles class="h-[19px] w-[19px]" /></div>
              <div>
                <div class="text-[15px] font-semibold">Enable developer themes</div>
                <p class="mt-[3px] max-w-[420px] text-[12.5px] leading-snug text-text-3">Locks the accent to a curated theme and swaps in the matching UBonito crest, wordmark &amp; favicon for everyone.</p>
              </div>
            </div>
            <Toggle v-model="brandForm.developer_themes_enabled" />
          </div>

          <div v-if="brandForm.developer_themes_enabled" class="mt-[22px]">
            <div class="mb-[13px] text-[11px] font-semibold uppercase tracking-wider text-text-faint">Theme — sets the accent colour</div>
            <div class="theme-grid">
              <button
                v-for="th in devThemeList"
                :key="th.id"
                type="button"
                class="theme-card"
                :class="{ on: brandForm.developer_theme === th.id }"
                :aria-pressed="brandForm.developer_theme === th.id"
                :style="{ '--tc': th.accent, '--tc-ground': th.ground, '--tc-badge': th.badge, '--tc-mark': th.mark, '--tc-halo': th.halo }"
                @click="brandForm.developer_theme = th.id"
              >
                <span class="tc-check"><Check class="h-[13px] w-[13px]" /></span>
                <div class="theme-prev">
                  <img :src="th.crest" alt="" class="h-[50px] w-[50px]" />
                  <div class="tp-word blackletter">UBonito</div>
                </div>
                <div class="theme-foot">
                  <span class="tf-dot" /><b class="text-[13.5px] font-semibold">{{ th.name }}</b><span class="tf-hex">{{ th.accent }}</span>
                </div>
              </button>
            </div>

            <div class="mt-6">
              <div class="mb-[13px] text-[11px] font-semibold uppercase tracking-wider text-text-faint">Apply theme assets — or keep your own</div>
              <div class="src-row">
                <div class="src-ico"><ImageIcon class="h-4 w-4" /></div>
                <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">Logo / mark</b><span class="mt-0.5 block text-[12px] text-text-3">{{ brandForm.logo_data_url ? 'You have an uploaded logo' : 'No custom logo uploaded' }}</span></div>
                <div class="seg">
                  <button type="button" :class="{ on: brandForm.asset_source_logo === 'theme' }" @click="brandForm.asset_source_logo = 'theme'">Theme crest</button>
                  <button type="button" class="keep" :class="{ on: brandForm.asset_source_logo === 'keep' }" @click="brandForm.asset_source_logo = 'keep'">{{ brandForm.logo_data_url ? 'My logo' : 'Default' }}</button>
                </div>
              </div>
              <div class="src-row">
                <div class="src-ico"><Type class="h-4 w-4" /></div>
                <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">Wordmark</b><span class="mt-0.5 block text-[12px] text-text-3">Blackletter “UBonito” vs. your application name</span></div>
                <div class="seg">
                  <button type="button" :class="{ on: brandForm.asset_source_wordmark === 'theme' }" @click="brandForm.asset_source_wordmark = 'theme'">Theme wordmark</button>
                  <button type="button" class="keep" :class="{ on: brandForm.asset_source_wordmark === 'keep' }" @click="brandForm.asset_source_wordmark = 'keep'">App name</button>
                </div>
              </div>
              <div class="src-row">
                <div class="src-ico"><ShieldCheck class="h-4 w-4" /></div>
                <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">Favicon</b><span class="mt-0.5 block text-[12px] text-text-3">{{ brandForm.favicon_data_url ? 'You have an uploaded favicon' : 'No custom favicon uploaded' }}</span></div>
                <div class="seg">
                  <button type="button" :class="{ on: brandForm.asset_source_favicon === 'theme' }" @click="brandForm.asset_source_favicon = 'theme'">Theme favicon</button>
                  <button type="button" class="keep" :class="{ on: brandForm.asset_source_favicon === 'keep' }" @click="brandForm.asset_source_favicon = 'keep'">{{ brandForm.favicon_data_url ? 'My favicon' : 'Default' }}</button>
                </div>
              </div>
              <div class="src-row forced">
                <div class="src-ico"><SlidersHorizontal class="h-4 w-4" /></div>
                <div class="min-w-0 flex-1"><b class="text-[14px] font-semibold">Accent colour</b><span class="mt-0.5 block text-[12px] text-text-3">Driven by the selected theme above</span></div>
                <span class="src-badge">Always theme</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      </div><!-- /left column -->

      <!-- RIGHT column: Assets → Display options → Live preview -->
      <div class="flex flex-col gap-5">
      <!-- Display options -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><SlidersHorizontal class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Display options</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div class="flex items-center justify-between gap-4" :class="{ 'opacity-50': brandForm.developer_themes_enabled }">
            <div><div class="text-[13px] font-semibold">Show logo in sidebar</div><div class="text-[12px] text-text-3">Displays the mark/logo badge</div></div>
            <Toggle v-model="brandForm.show_logo" :disabled="brandForm.developer_themes_enabled" />
          </div>
          <div class="flex items-center justify-between gap-4 border-t border-border pt-4" :class="{ 'opacity-50': brandForm.developer_themes_enabled }">
            <div><div class="text-[13px] font-semibold">Show application name in sidebar</div><div class="text-[12px] text-text-3">Displays the name next to the logo</div></div>
            <Toggle v-model="brandForm.show_app_name" :disabled="brandForm.developer_themes_enabled" />
          </div>
          <p v-if="brandForm.developer_themes_enabled" class="-mt-1 flex items-start gap-1.5 text-[11px] text-text-3"><Info class="h-3.5 w-3.5 shrink-0" />Controlled by the active developer theme.</p>
          <div class="border-t border-border pt-4">
            <div class="flex items-center justify-between gap-4">
              <div><div class="text-[13px] font-semibold">Custom login page message</div><div class="text-[12px] text-text-3">Show a welcome message on the sign-in screen</div></div>
              <Toggle v-model="customLoginMessage" />
            </div>
            <textarea v-if="customLoginMessage" v-model="loginMessage" class="input mt-3 h-20" placeholder="Welcome — sign in with your company account."></textarea>
          </div>
        </div>
      </div>
      <!-- Assets -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><ImageIcon class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Assets</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div>
            <div class="lockable" :class="{ locked: previewLogoTheme }">
              <div class="mb-1.5 text-[12px] text-text-2"><span class="font-semibold text-text">Logo</span> shown in the sidebar header</div>
              <div class="mb-2 flex h-20 items-center justify-center rounded-xl border border-border bg-surface-2">
                <img v-if="brandForm.logo_data_url" :src="brandForm.logo_data_url" class="max-h-14 rounded" />
                <span v-else class="text-[12px] text-text-3">No logo uploaded</span>
              </div>
              <div class="flex items-center justify-between gap-2 rounded-xl border border-dashed border-border-2 px-3 py-3">
                <span class="text-[13px]"><span class="font-medium">Upload logo</span><br /><span class="text-[12px] text-text-3">SVG or PNG, any aspect ratio</span></span>
                <div class="flex shrink-0 items-center gap-2">
                  <button v-if="brandForm.logo_data_url" type="button" class="btn-ghost" @click="brandForm.logo_data_url = ''">Remove</button>
                  <label class="btn-ghost cursor-pointer">Choose file<input type="file" accept="image/*" class="hidden" @change="onFile($event, 'logo_data_url')" /></label>
                </div>
              </div>
            </div>
            <div class="lock-note" :class="{ show: previewLogoTheme }">
              <Info class="h-4 w-4 shrink-0" />
              <span>Showing the theme crest. To use your own, set <b>Logo / mark</b> to “{{ brandForm.logo_data_url ? 'My logo' : 'Default' }}” under Developer themes.</span>
            </div>
          </div>
          <div class="border-t border-border pt-4">
            <div class="lockable" :class="{ locked: previewFavTheme }">
              <div class="mb-1.5 text-[12px] text-text-2"><span class="font-semibold text-text">Favicon</span> browser tab icon, 32×32 recommended</div>
              <div class="flex items-center justify-between gap-2 rounded-xl border border-dashed border-border-2 px-3 py-3">
                <span class="flex items-center gap-3 text-[13px]">
                  <img v-if="brandForm.favicon_data_url" :src="brandForm.favicon_data_url" class="h-7 w-7 rounded" />
                  <span><span class="font-medium">Upload favicon</span><br /><span class="text-[12px] text-text-3">ICO, PNG or SVG</span></span>
                </span>
                <div class="flex shrink-0 items-center gap-2">
                  <button v-if="brandForm.favicon_data_url" type="button" class="btn-ghost" @click="brandForm.favicon_data_url = ''">Remove</button>
                  <label class="btn-ghost cursor-pointer">Choose file<input type="file" accept="image/*" class="hidden" @change="onFile($event, 'favicon_data_url')" /></label>
                </div>
              </div>
            </div>
            <div class="lock-note" :class="{ show: previewFavTheme }">
              <Info class="h-4 w-4 shrink-0" />
              <span>Showing the theme favicon. To use your own, set <b>Favicon</b> to “{{ brandForm.favicon_data_url ? 'My favicon' : 'Default' }}” under Developer themes.</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Live preview -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Eye class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Live preview</h3></div>
        <div class="p-5">
          <div class="overflow-hidden rounded-xl border border-border">
            <!-- browser-tab strip (favicon) -->
            <div class="flex items-center gap-2 border-b border-border bg-surface-2 px-3 py-2.5">
              <img v-if="previewFavTheme" :src="previewCrest" alt="" class="h-6 w-6" />
              <img v-else-if="brandForm.favicon_data_url" :src="brandForm.favicon_data_url" class="h-6 w-6 rounded" />
              <img v-else :src="DEFAULT_MARK" alt="" class="h-6 w-6 rounded" />
              <span class="text-[13px] font-semibold">{{ brandForm.app_name || 'Orbit' }}</span>
            </div>
            <!-- sidebar brand (mark + wordmark), honouring the show-logo/name toggles -->
            <div class="flex items-center gap-3 px-3 pt-3">
              <template v-if="previewLogoTheme || brandForm.show_logo">
                <img v-if="previewLogoTheme" :src="previewCrest" alt="" class="h-[42px] w-[42px]" />
                <img v-else-if="brandForm.logo_data_url" :src="brandForm.logo_data_url" class="h-[38px] w-auto max-w-[150px] rounded-md object-contain" />
                <img v-else :src="DEFAULT_MARK" alt="" class="h-[42px] w-[42px]" />
              </template>
              <div v-if="previewWordTheme || brandForm.show_app_name">
                <div v-if="previewWordTheme" class="blackletter text-[22px] leading-none" :style="{ color: activeTheme.mark, '-webkit-text-stroke': `2.2px ${activeTheme.halo}` }">UBonito</div>
                <div v-else class="text-[18px] font-semibold leading-tight">{{ brandForm.app_name || 'Orbit' }}</div>
                <div class="mt-0.5 text-[11.5px] text-text-3">{{ t('nav.tagline') }}</div>
              </div>
            </div>
            <div class="flex items-center gap-3 p-3">
              <button class="btn-primary" :style="previewBtnStyle">Enable server</button>
              <a class="text-[12px] hover:underline" :style="{ color: previewAccent?.accent || 'var(--accent)' }">View details →</a>
            </div>
          </div>
        </div>
      </div>

      </div><!-- /right column -->

      <div class="flex justify-end lg:col-span-2"><button class="btn-primary" @click="saveBranding"><Send class="h-4 w-4" /> Save branding</button></div>
    </section>

    <!-- ===== Notifications ===== -->
    <section v-else-if="tab === 'notifications'">
      <div v-if="notifCfg" class="flex flex-col gap-4">
        <p class="text-[13px] text-text-2">
          Configure the channels users can choose from. Enable a channel, fill its settings, then send a test.
        </p>

        <div class="columns-1 gap-x-4 lg:columns-2 [&>*]:mb-4 [&>*]:break-inside-avoid">
        <!-- Email -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-[14px] font-semibold">Email (SMTP)</h3>
            <Toggle v-model="notifCfg.email.enabled" />
          </div>
          <div v-if="notifCfg.email.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">Host</label><input v-model="notifCfg.email.host" class="input" placeholder="smtp.example.org" /></div>
              <div><label class="label">Port</label><input v-model.number="notifCfg.email.port" type="number" class="input" placeholder="587" /></div>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">Username</label><input v-model="notifCfg.email.username" class="input" /></div>
              <div><label class="label">Password</label><input v-model="notifCfg.email.password" type="password" class="input" /></div>
            </div>
            <div><label class="label">From address</label><input v-model="notifCfg.email.from" class="input" placeholder="orbit@example.org" /></div>
            <Checkbox v-model="notifCfg.email.start_tls" class="text-text-2">Use STARTTLS</Checkbox>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'email'" @click="testChannel('email')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Discord -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Discord</h3><Toggle v-model="notifCfg.discord.enabled" /></div>
          <div v-if="notifCfg.discord.enabled" class="flex flex-col gap-3">
            <div><label class="label">Webhook URL</label><input v-model="notifCfg.discord.webhook_url" class="input" placeholder="https://discord.com/api/webhooks/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'discord'" @click="testChannel('discord')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Telegram -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Telegram</h3><Toggle v-model="notifCfg.telegram.enabled" /></div>
          <div v-if="notifCfg.telegram.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">Bot token</label><input v-model="notifCfg.telegram.bot_token" class="input" /></div>
              <div><label class="label">Chat ID</label><input v-model="notifCfg.telegram.chat_id" class="input" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'telegram'" @click="testChannel('telegram')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- ntfy -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">ntfy</h3><Toggle v-model="notifCfg.ntfy.enabled" /></div>
          <div v-if="notifCfg.ntfy.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">Server URL</label><input v-model="notifCfg.ntfy.server_url" class="input" placeholder="https://ntfy.sh" /></div>
              <div><label class="label">Topic</label><input v-model="notifCfg.ntfy.topic" class="input" /></div>
            </div>
            <div><label class="label">Token (optional)</label><input v-model="notifCfg.ntfy.token" class="input" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'ntfy'" @click="testChannel('ntfy')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Gotify -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Gotify</h3><Toggle v-model="notifCfg.gotify.enabled" /></div>
          <div v-if="notifCfg.gotify.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">Server URL</label><input v-model="notifCfg.gotify.server_url" class="input" placeholder="https://gotify.example.org" /></div>
              <div><label class="label">App token</label><input v-model="notifCfg.gotify.token" class="input" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'gotify'" @click="testChannel('gotify')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Microsoft Teams -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Microsoft Teams</h3><Toggle v-model="notifCfg.teams.enabled" /></div>
          <div v-if="notifCfg.teams.enabled" class="flex flex-col gap-3">
            <p class="text-[12px] text-text-3">In the channel: <b>Workflows → "Send webhook alerts to a channel"</b> (or "When a Teams webhook request is received"), and paste its URL. Messages arrive as an Adaptive Card.</p>
            <div><label class="label">Workflow URL</label><input v-model="notifCfg.teams.webhook_url" class="input" placeholder="https://….logic.azure.com/workflows/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'teams'" @click="testChannel('teams')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Slack -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Slack</h3><Toggle v-model="notifCfg.slack.enabled" /></div>
          <div v-if="notifCfg.slack.enabled" class="flex flex-col gap-3">
            <p class="text-[12px] text-text-3">A Slack app's <b>Incoming Webhook</b> for the channel.</p>
            <div><label class="label">Webhook URL</label><input v-model="notifCfg.slack.webhook_url" class="input" placeholder="https://hooks.slack.com/services/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'slack'" @click="testChannel('slack')"><Send class="h-4 w-4" /> Test</button></div>
          </div>
        </div>

        <!-- Integration webhook -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Integration webhook</h3><Toggle v-model="notifCfg.webhook.enabled" /></div>
          <div v-if="notifCfg.webhook.enabled" class="flex flex-col gap-3">
            <p class="text-[12px] text-text-3">Signed JSON POST on Orbit events (HMAC-SHA256 in <code>X-Orbit-Signature</code>).</p>
            <div><label class="label">URL</label><input v-model="notifCfg.webhook.url" class="input" placeholder="https://hooks.example.org/orbit" /></div>
            <div><label class="label">Signing secret</label><input v-model="notifCfg.webhook.secret" type="password" class="input" placeholder="HMAC key" /></div>
          </div>
        </div>

        </div>
        <!-- /channel grid -->

        <!-- Server events for the organisation -->
        <div class="card overflow-hidden">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Bell class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">Server events</h3></div>
          <div class="flex flex-col gap-4 p-5">
            <p class="text-[12px] text-text-3">What goes to the channels above for everyone (all off by default). Users can also get them by email for the servers in their scope (Settings → Notifications).</p>
            <div v-for="ev in serverEvents" :key="ev.key" class="flex items-center justify-between gap-4">
              <div><div class="text-[13px] font-semibold">{{ ev.label }}</div><div class="text-[12px] text-text-3">{{ ev.hint }}</div></div>
              <Toggle v-model="notifCfg.events[ev.key]" :aria-label="ev.label" />
            </div>
            <div class="border-t border-border pt-4">
              <div class="mb-2 text-[13px] font-semibold">Send to</div>
              <div v-if="enabledShared.length" class="flex flex-wrap gap-3">
                <Checkbox v-for="c in enabledShared" :key="c.key" :model-value="eventChannelOn(c.key)" class="text-text-2" @update:model-value="(v: boolean) => toggleEventChannel(c.key, v)">{{ c.label }}</Checkbox>
              </div>
              <p v-else class="text-[12px] text-text-3">No shared channel is enabled above.</p>
              <div class="mt-3">
                <label class="label">Email to</label>
                <input v-model="notifCfg.events.emails" class="input" :disabled="!notifCfg.email.enabled" placeholder="ops@example.org, oncall@example.org" />
                <p class="mt-1 text-[11.5px] text-text-3">{{ notifCfg.email.enabled ? 'Comma-separated. Leave empty for no email.' : 'Turn on Email above to send these by email.' }}</p>
              </div>
              <p v-if="notifCfg.webhook.enabled" class="mt-3 text-[12px] text-text-3">The integration webhook gets the same events, as JSON (<code>X-Orbit-Event: server.down</code>, …).</p>
            </div>
          </div>
        </div>

        <div class="flex justify-end"><button class="btn-primary" @click="saveNotifConfig">Save</button></div>
      </div>
      <div v-else class="text-[13px] text-text-3">Loading…</div>
    </section>

    <!-- ===== Security ===== -->
    <section v-else-if="tab === 'security'" class="max-w-3xl">
      <div class="card flex flex-col divide-y divide-border">
        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">Enforce two-factor authentication</div>
            <div class="text-[12px] text-text-3">
              Every local user without 2FA must set it up at next sign-in (SSO accounts are exempt — MFA is the IdP's job).
            </div>
          </div>
          <Toggle :model-value="otpEnforced" @update:model-value="setEnforce" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">Hide password login</div>
            <div class="text-[12px] text-text-3">
              Hide the username/password form on the login page (effective when SSO is enabled). Recover it any
              time at <code>/login?local=1</code> if your provider is down.
            </div>
          </div>
          <Toggle :model-value="hideLocalLogin" @update:model-value="setHideLocalLogin" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">Auto-start SSO sign-in</div>
            <div class="text-[12px] text-text-3">
              Redirect straight to your SSO provider when someone opens the login page. Skipped at
              <code>/login?local=1</code>.
              <span v-if="!branding.oidcEnabled" class="text-text-3">Enable SSO first.</span>
            </div>
          </div>
          <Toggle :model-value="autoSso" :disabled="!branding.oidcEnabled" @update:model-value="setAutoSso" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">Allow passwordless passkey sign-in</div>
            <div class="text-[12px] text-text-3">
              Show the "Sign in with a passkey" button and allow passwordless login. Passkeys as a second factor are unaffected.
            </div>
          </div>
          <Toggle :model-value="passwordlessEnabled" @update:model-value="setPasswordless" />
        </div>

      </div>
    </section>

    <!-- ===== SSO / OIDC ===== -->
    <section v-else-if="tab === 'sso'" class="max-w-3xl">
      <div v-if="oidcCfg" class="card p-5">
        <div class="mb-4 flex items-center justify-between gap-4">
          <div>
            <h3 class="text-[14px] font-semibold">Single sign-on (OIDC)</h3>
            <div class="text-[12px] text-text-3">Let users sign in via an OpenID Connect provider (Keycloak, Authentik, Google, …).</div>
          </div>
          <Toggle v-model="oidcCfg.enabled" />
        </div>
        <div class="flex flex-col gap-3">
          <div>
            <label class="label">Issuer URL</label>
            <div class="flex gap-2">
              <input v-model="oidcCfg.issuer" class="input" placeholder="https://idp.example.com/realms/main" />
              <button type="button" class="btn-ghost shrink-0" @click="testOIDC">Test</button>
            </div>
            <p class="mt-1 text-[11px] text-text-3">The provider's base URL; its <code>/.well-known/openid-configuration</code> is discovered automatically.</p>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div><label class="label">Client ID</label><input v-model="oidcCfg.client_id" class="input" /></div>
            <div>
              <label class="label">Client secret</label>
              <input v-model="oidcCfg.client_secret" type="password" class="input" :placeholder="oidcCfg.has_secret ? '•••••••• (unchanged)' : ''" />
            </div>
          </div>
          <div>
            <label class="label">Redirect URL</label>
            <input v-model="oidcCfg.redirect_url" class="input" placeholder="https://lb.example.org/api/v1/auth/oidc/callback" />
            <p class="mt-1 text-[11px] text-text-3">Register this exact URL at your provider.</p>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div><label class="label">Extra scopes</label><input v-model="oidcCfg.scopes" class="input" placeholder="groups" /></div>
            <div><label class="label">Role claim</label><input v-model="oidcCfg.role_claim" class="input" placeholder="groups" /></div>
            <div><label class="label">Default role</label><input v-model="oidcCfg.default_role" class="input" placeholder="none" /></div>
          </div>
          <p class="text-[11px] text-text-3">
            New users are provisioned on first login. If a role claim is set, its values are matched to role names each
            login; otherwise (or with no match) the default role is assigned.
          </p>
          <div class="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-2 px-4 py-3">
            <div>
              <div class="text-[13px] font-semibold">{{ t('admin.ssoAllowAdmin') }}</div>
              <div class="text-[12px] text-text-3">{{ t('admin.ssoAllowAdminHint') }}</div>
            </div>
            <Toggle :model-value="!!oidcCfg.allow_admin" @update:model-value="(v) => oidcCfg && (oidcCfg.allow_admin = v)" />
          </div>
          <div class="flex justify-end"><button class="btn-primary" :disabled="oidcBusy" @click="saveOIDC">{{ oidcBusy ? 'Saving…' : 'Save SSO settings' }}</button></div>
        </div>
      </div>
    </section>


    <!-- ===== Owner (god mode) ===== -->
    <section v-else-if="tab === 'owner'" class="flex flex-col gap-5">
      <div class="grid items-start gap-5 lg:grid-cols-2">
      <!-- LEFT column: Feature flags -->
      <div class="flex flex-col gap-5">
      <!-- Feature flags -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <Crown class="h-[18px] w-[18px] text-violet" /><h3 class="text-[14px] font-semibold">Feature flags</h3>
        </div>
        <div class="flex flex-col gap-4 p-5">
          <p class="text-[12px] text-text-3">Opt-in capabilities. Only you (the instance owner) can change these.</p>
          <div v-for="f in featureMeta" :key="f.key" class="flex items-center justify-between gap-4">
            <div>
              <div class="text-[13px] font-semibold">{{ f.label }}</div>
              <div class="text-[12px] text-text-3">{{ f.hint }}</div>
            </div>
            <Toggle :model-value="featureFlags[f.key] ?? false" :aria-label="f.label" @update:model-value="(v) => toggleFeature(f.key, v)" />
          </div>
        </div>
      </div>

      <!-- Live updates: how often proxies are polled and pages refresh -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <RefreshCw class="h-[18px] w-[18px] text-violet" /><h3 class="text-[14px] font-semibold">{{ t('owner.refreshTitle') }}</h3>
        </div>
        <div class="flex flex-col gap-3 p-5">
          <p class="text-[12px] text-text-3">{{ t('owner.refreshHint', { d: px.data?.ui?.default_refresh_seconds ?? 10 }) }}</p>
          <form class="flex items-center gap-2" @submit.prevent="saveRefresh">
            <input v-model.number="refreshSeconds" type="number" min="1" max="300" class="input w-24" />
            <span class="text-[13px] text-text-2">{{ t('owner.seconds') }}</span>
            <span class="flex-1" />
            <button v-if="refreshIsCustom" type="button" class="btn-ghost" @click="resetRefresh">{{ t('owner.useDefault') }}</button>
            <button type="submit" class="btn-primary">{{ t('common.save') }}</button>
          </form>
        </div>
      </div>

      </div><!-- /left column -->

      <!-- RIGHT column: Security alerts -->
      <div class="flex flex-col gap-5">
      <!-- Security alerts -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <Bell class="h-[18px] w-[18px] text-violet" /><h3 class="text-[14px] font-semibold">Security alerts</h3>
        </div>
        <div class="flex flex-col gap-4 p-5">
          <p class="text-[12px] text-text-3">
            Notify when a high-signal action happens. Configure channels under Notifications.
          </p>
          <div v-for="a in securityAlertMeta" :key="a.key" class="flex items-center justify-between gap-4">
            <div class="text-[13px]">{{ a.label }}</div>
            <Toggle :model-value="alertEvents[a.key] ?? false" @update:model-value="(v) => toggleSecurityAlert(a.key, v)" />
          </div>

          <div class="border-t border-border pt-4">
            <div class="mb-2 text-[12px] font-semibold text-text-2">Notify via</div>
            <p v-if="alertAvailable.length === 0" class="text-[12px] text-text-3">
              No channels enabled — set one up under Notifications first.
            </p>
            <div v-else class="flex flex-wrap gap-3">
              <Checkbox
                v-for="ch in alertAvailable"
                :key="ch"
                :model-value="alertChannels.includes(ch)"
                class="text-text-2 capitalize"
                @update:model-value="(v) => toggleAlertChannel(ch, v)"
              >{{ ch }}</Checkbox>
            </div>
          </div>

          <div v-if="alertAvailable.includes('email')" class="border-t border-border pt-4">
            <label class="label">Alert email recipient</label>
            <div class="flex gap-2">
              <input v-model="alertEmail" class="input" placeholder="defaults to the owner's email" />
              <button class="btn-ghost shrink-0" @click="saveAlertEmail">Save</button>
            </div>
          </div>
        </div>
      </div>
      </div><!-- /right column -->
      </div>

      <BackupCard />

      <!-- Danger zone: actions you can't undo yourself. -->
      <div class="card overflow-hidden border-crit-border">
        <div class="flex items-center gap-2 border-b border-crit-border bg-crit-bg px-5 py-3">
          <TriangleAlert class="h-[18px] w-[18px] text-crit-text" /><h3 class="text-[14px] font-semibold text-crit-text">{{ t('owner.dangerZone') }}</h3>
        </div>
        <div class="flex flex-wrap items-center gap-4 p-5">
          <div class="min-w-[240px] flex-1">
            <div class="text-[13px] font-semibold">{{ t('owner.transferTitle') }}</div>
            <div class="text-[12px] text-text-3">{{ t('owner.transferHint') }}</div>
          </div>
          <select v-model="transferTarget" class="input w-auto min-w-[200px]">
            <option :value="null">{{ t('owner.selectUser') }}</option>
            <option v-for="u in transferCandidates" :key="u.id" :value="u.id">{{ u.username }}</option>
          </select>
          <button class="btn-danger" :disabled="!transferTarget" @click="transferConfirm = true">{{ t('owner.transferBtn') }}</button>
        </div>
      </div>
    </section>

    <Confirm
      :open="transferConfirm"
      :title="t('owner.transferConfirmTitle')"
      :message="t('owner.transferConfirm', { name: transferName })"
      :type-to-confirm="transferName"
      :confirm-label="t('owner.transferBtn')"
      danger
      @confirm="runTransfer"
      @close="transferConfirm = false"
    />

    <!-- ===== New user modal ===== -->
    <Overlay :open="showUser" title="New user" mode="centered" @close="showUser = false">
      <form class="flex flex-col gap-4" @submit.prevent="createUser">
        <div><label class="label">Username</label><input v-model="userForm.username" class="input" /></div>
        <div><label class="label">{{ t('admin.displayName') }}</label><input v-model="userForm.display_name" class="input" /></div>
        <div><label class="label">Email</label><input v-model="userForm.email" type="email" class="input" /></div>
        <Checkbox v-model="userForm.generate_password" class="text-text-2">{{ t('admin.generateTemp') }}</Checkbox>
        <div v-if="!userForm.generate_password">
          <label class="label">Password (min 8)</label>
          <input v-model="userForm.password" type="password" class="input" />
        </div>
        <div><label class="label">Roles</label><RoleSelect v-model="userForm.role_ids" :roles="roles" /></div>
        <Checkbox v-if="!userForm.generate_password" v-model="userForm.must_change_password" class="text-text-2">Require password change on first login</Checkbox>
        <Checkbox v-model="userForm.otp_required" class="text-text-2">{{ t('admin.requireOtp') }}</Checkbox>
        <div class="mt-1 flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showUser = false">Cancel</button>
          <button type="submit" class="btn-primary">Create user</button>
        </div>
      </form>
    </Overlay>

    <!-- ===== Generated credentials (one-time) ===== -->
    <Overlay :open="!!createdCreds" :title="createdCreds?.reset ? t('admin.passwordReset') : 'User created'" mode="centered" @close="createdCreds = null">
      <div v-if="createdCreds" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
          Copy this password now — it is shown only once and cannot be retrieved later.
          The user must choose a new one at the first sign-in.
        </p>
        <div class="grid grid-cols-[auto,1fr] items-center gap-x-3 gap-y-1.5">
          <span class="text-text-3">Username</span><span class="font-medium">{{ createdCreds.username }}</span>
          <span class="text-text-3">Password</span><code class="select-all rounded bg-surface-2 px-2 py-1 font-mono">{{ createdCreds.password }}</code>
        </div>
        <div class="mt-1 flex justify-end gap-2">
          <button class="btn-ghost" @click="copyCreds"><Copy class="h-4 w-4" /> Copy password</button>
          <button class="btn-primary" @click="createdCreds = null">Done</button>
        </div>
      </div>
    </Overlay>

    <!-- ===== Edit user modal ===== -->
    <Overlay :open="showEditUser" title="Edit user" mode="centered" @close="showEditUser = false">
      <form class="flex flex-col gap-4" @submit.prevent="saveEditUser">
        <div v-if="editUserTarget?.oidc" class="rounded-xl border border-border bg-surface-2 px-3 py-2 text-[12px] text-text-3">
          This is an <b>SSO</b> account — its email and display name come from the identity provider and are synced at each sign-in.
        </div>
        <div>
          <label class="label">{{ t('admin.displayName') }}</label>
          <input v-model="editUserForm.display_name" class="input" :disabled="editUserTarget?.oidc" />
        </div>
        <div>
          <label class="label">Email</label>
          <input v-model="editUserForm.email" type="email" class="input" :disabled="editUserTarget?.oidc" />
        </div>
        <div><label class="label">Roles</label><RoleSelect v-model="editUserForm.role_ids" :roles="roles" /></div>

        <div v-if="editUserTarget?.id !== auth.user?.id" class="flex items-center justify-between border-t border-border pt-4">
          <div>
            <div class="text-[13px] font-semibold">Disable account</div>
            <div class="text-[12px] text-text-3">{{ t('admin.disableHint') }}</div>
          </div>
          <Toggle v-model="editUserForm.disabled" />
        </div>

        <div v-if="!editUserTarget?.oidc" class="flex items-center justify-between border-t border-border pt-4">
          <div>
            <div class="text-[13px] font-semibold">{{ t('admin.requireOtp') }}</div>
            <div class="text-[12px] text-text-3">{{ t('admin.requireOtpHint') }}</div>
          </div>
          <Toggle v-model="editUserForm.otp_required" />
        </div>

        <div v-if="!editUserTarget?.oidc" class="flex items-center justify-between border-t border-border pt-4">
          <div>
            <div class="text-[13px] font-semibold">Two-factor</div>
            <div class="text-[12px] text-text-3">
              {{ editUserTarget?.otp_enabled ? 'Enabled' : 'Not set up' }} — reset to clear a lost authenticator
            </div>
          </div>
          <button type="button" class="btn-ghost" :disabled="!editUserTarget?.otp_enabled" @click="resetUserOtp">
            Reset 2FA
          </button>
        </div>

        <div class="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-2 px-4 py-3">
          <div>
            <div class="text-[13px] font-semibold">Passkeys</div>
            <div class="text-[12px] text-text-3">Remove all of this user's registered passkeys (e.g. a lost device).</div>
          </div>
          <button type="button" class="btn-ghost" @click="resetUserWebauthn">Reset passkeys</button>
        </div>

        <div class="mt-1 flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showEditUser = false">Cancel</button>
          <button type="submit" class="btn-primary">Save changes</button>
        </div>
      </form>
    </Overlay>

    <!-- ===== Role editor modal ===== -->
    <Overlay :open="showRole" :title="roleForm.id ? 'Edit role' : 'New role'" mode="centered" wide @close="showRole = false">
      <form class="flex flex-col gap-6" @submit.prevent="saveRole">
        <!-- Name + description -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label">Name <span class="text-text-3">required</span></label>
            <input v-model="roleForm.name" class="input" placeholder="e.g. Staging operators" />
          </div>
          <div>
            <label class="label">Description <span class="text-text-3">optional</span></label>
            <input v-model="roleForm.description" class="input" placeholder="What this role allows" />
          </div>
        </div>

        <!-- Global privileges -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-[11px] font-semibold uppercase tracking-wide text-text-3">Global privileges</span>
              <span class="rounded-full bg-surface-3 px-1.5 py-0.5 text-[11px] text-text-2">{{ roleForm.global_privileges.length }} of {{ GLOBAL_PRIVILEGES.length }}</span>
            </div>
            <button type="button" class="btn-ghost px-2.5 py-1" @click="toggleAllGlobal">{{ allGlobalSelected ? 'Clear all' : 'Select all' }}</button>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <button
              v-for="gp in GLOBAL_PRIVILEGES"
              :key="gp.id"
              type="button"
              :disabled="gp.ownerOnly && !auth.isOwner"
              :title="gp.ownerOnly && !auth.isOwner ? 'Only the instance owner can grant this' : ''"
              class="flex items-start gap-3 rounded-xl border p-3 text-left transition disabled:cursor-not-allowed disabled:opacity-50"
              :class="roleForm.global_privileges.includes(gp.id) ? 'border-accent bg-accent-bg' : 'border-border bg-surface-2 hover:border-border-2'"
              @click="toggle(roleForm.global_privileges, gp.id)"
            >
              <component :is="gp.icon" class="mt-0.5 h-[18px] w-[18px] shrink-0 text-text-2" />
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5 text-[13px] font-medium">{{ gp.label }}<Lock v-if="gp.ownerOnly && !auth.isOwner" class="h-3 w-3 text-text-3" /></div>
                <div class="text-[11px] text-text-3">{{ gp.desc }}</div>
              </div>
              <span class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border" :class="roleForm.global_privileges.includes(gp.id) ? 'border-accent bg-accent text-white' : 'border-border-2'">
                <Check v-if="roleForm.global_privileges.includes(gp.id)" class="h-3 w-3" />
              </span>
            </button>
          </div>
        </div>

        <!-- Server grants -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-[11px] font-semibold uppercase tracking-wide text-text-3">Server grants</span>
              <span class="rounded-full bg-surface-3 px-1.5 py-0.5 text-[11px] text-text-2">{{ roleForm.grants.length }} grant{{ roleForm.grants.length === 1 ? '' : 's' }}</span>
            </div>
            <button type="button" class="btn-ghost px-2.5 py-1" @click="roleForm.grants.push(newGrant())"><Plus class="h-4 w-4" /> Add grant</button>
          </div>

          <div class="flex flex-col gap-3">
            <div v-for="(g, gi) in roleForm.grants" :key="gi" class="overflow-hidden rounded-xl border border-border">
              <!-- Grant header -->
              <div class="flex items-center justify-between border-b border-border bg-surface-2 px-3 py-2.5">
                <div class="flex items-center gap-2">
                  <span class="flex h-5 w-5 items-center justify-center rounded-full border border-border-2 text-[11px] text-text-2">{{ gi + 1 }}</span>
                  <span class="text-[13px] font-medium">Server grant</span>
                </div>
                <button type="button" class="rounded-lg border border-border px-2 py-1 text-text-3 hover:text-crit-text" @click="roleForm.grants.splice(gi, 1)"><Trash2 class="h-4 w-4" /></button>
              </div>

              <!-- All servers -->
              <button
                type="button"
                class="flex w-full items-center justify-between border-b border-border px-3 py-2.5 text-left transition hover:bg-surface-2"
                @click="g.all_servers = !g.all_servers"
              >
                <span class="flex items-center gap-2 text-[13px] font-medium">
                  <span class="flex h-4 w-4 shrink-0 items-center justify-center rounded border" :class="g.all_servers ? 'border-accent bg-accent text-white' : 'border-border-2'">
                    <Check v-if="g.all_servers" class="h-3 w-3" />
                  </span>
                  All servers
                </span>
                <span class="text-[12px] text-text-3">Every server on every proxy</span>
              </button>

              <!-- Scope rules -->
              <div v-if="!g.all_servers" class="border-b border-border px-3 py-2.5">
                <div class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-text-3">Scope rules</div>
                <textarea
                  v-model="g.rulesText"
                  rows="3"
                  class="input font-mono text-[12.5px]"
                  :placeholder="'env=gmr\nservice=facebook\nenv=rda machine=web0*'"
                  spellcheck="false"
                />
                <p v-if="g.rulesText.trim()" class="mt-1.5 text-[12px]" :class="ruleMatch(g.rulesText).error ? 'text-crit-text' : 'text-text-2'">
                  <template v-if="ruleMatch(g.rulesText).error">{{ t('admin.ruleBad', { part: ruleMatch(g.rulesText).error }) }}</template>
                  <template v-else>
                    {{ t('admin.ruleMatches', { n: ruleMatch(g.rulesText).hits.length, m: px.targets.length }) }}
                    <span v-if="ruleMatch(g.rulesText).hits.length" class="font-mono text-text-3">
                      {{ ruleMatch(g.rulesText).hits.slice(0, 6).join(', ') }}{{ ruleMatch(g.rulesText).hits.length > 6 ? ', …' : '' }}
                    </span>
                  </template>
                </p>
                <p class="mt-1.5 text-[11.5px] text-text-3">
                  One rule per line; a server is in scope if any rule matches. A rule's parts must all match. Keys:
                  <code v-for="(k, ki) in RULE_KEYS" :key="k">{{ k }}{{ ki < RULE_KEYS.length - 1 ? ', ' : '' }}</code>; <code>*</code> and <code>?</code> wildcards.
                </p>
              </div>

              <!-- Scoped privileges -->
              <div class="grid grid-cols-2 gap-x-6 gap-y-1 p-3">
                <button
                  v-for="sp in SCOPED_PRIVILEGES"
                  :key="sp.id"
                  type="button"
                  class="flex items-center justify-between rounded-lg px-1.5 py-1.5 text-left transition hover:bg-surface-2"
                  @click="toggle(g.privileges, sp.id)"
                >
                  <span class="flex items-center gap-2 text-[13px]">
                    <component :is="sp.icon" class="h-[16px] w-[16px] text-text-3" /> {{ sp.label }}
                  </span>
                  <span class="flex h-4 w-4 items-center justify-center rounded border" :class="g.privileges.includes(sp.id) ? 'border-accent bg-accent text-white' : 'border-border-2'">
                    <Check v-if="g.privileges.includes(sp.id)" class="h-3 w-3" />
                  </span>
                </button>
              </div>
            </div>

            <button type="button" class="flex items-center justify-center gap-2 rounded-xl border border-dashed border-border-2 py-2.5 text-[13px] text-text-2 hover:bg-surface-2" @click="roleForm.grants.push(newGrant())">
              <Plus class="h-4 w-4" /> Add grant
            </button>
          </div>
        </div>

        <!-- Footer -->
        <div class="flex items-center justify-between border-t border-border pt-4">
          <span class="text-[12px] text-text-3">Roles apply to users immediately on save</span>
          <div class="flex gap-2">
            <button type="button" class="btn-ghost" @click="showRole = false">Cancel</button>
            <button type="submit" class="btn-primary">Save role</button>
          </div>
        </div>
      </form>
    </Overlay>

    <!-- ===== Reset password modal ===== -->
    <Overlay :open="!!resetTarget" title="Reset password" mode="centered" @close="resetTarget = null">
      <form class="flex flex-col gap-3" @submit.prevent="resetPassword">
        <p class="text-[13px] text-text-2">Set a new password for <b>{{ resetTarget?.username }}</b>.</p>
        <Checkbox v-model="resetGenerate" class="text-text-2">{{ t('admin.generateTemp') }}</Checkbox>
        <template v-if="!resetGenerate">
          <input v-model="newPassword" type="password" class="input" placeholder="New password (min 8)" />
          <Checkbox v-model="resetMustChange" class="text-text-2">Require a new password at next sign-in</Checkbox>
        </template>
        <p class="text-[12px] text-text-3">All of this user's sessions are signed out immediately.</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="resetTarget = null">Cancel</button>
          <button type="submit" class="btn-primary">Reset</button>
        </div>
      </form>
    </Overlay>

    <Confirm
      :open="!!revokeTarget"
      :title="t('admin.revokeTitle')"
      :message="t('admin.revokeConfirm', { name: revokeTarget?.name, user: revokeTarget?.username })"
      :confirm-label="t('admin.revoke')"
      danger
      @confirm="revokeToken"
      @close="revokeTarget = null"
    />
    <Confirm
      :open="!!disableTarget"
      :title="t('admin.disableTitle')"
      :message="t('admin.disableConfirm', { name: disableTarget?.username })"
      :confirm-label="t('admin.disableUser')"
      danger
      @confirm="disableTarget && setDisabled(disableTarget, true)"
      @close="disableTarget = null"
    />
    <Confirm
      :open="!!logoutTarget"
      :title="t('admin.forceLogout')"
      :message="t('admin.forceLogoutConfirm', { name: logoutTarget?.username })"
      :confirm-label="t('admin.forceLogoutBtn')"
      danger
      @confirm="confirmForceLogout"
      @close="logoutTarget = null"
    />
    <Confirm
      :open="!!deleteUserTarget"
      title="Delete user?"
      :message="`Permanently delete ${deleteUserTarget?.username}?`"
      confirm-label="Delete"
      danger
      @confirm="removeUser"
      @close="deleteUserTarget = null"
    />
    <Confirm
      :open="!!deleteRoleTarget"
      title="Delete role?"
      :message="`Delete role ${deleteRoleTarget?.name}? It will be removed from any users who have it.`"
      confirm-label="Delete"
      danger
      @confirm="removeRole"
      @close="deleteRoleTarget = null"
    />
  </div>
</template>

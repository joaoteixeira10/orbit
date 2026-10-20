<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter, useRoute } from 'vue-router'
import { onLiveEvent } from '@/lib/live'
import { useFlash } from '@/lib/highlight'
import { usersApi, apiTokensAdminApi, rolesApi, settingsApi, notifyApi, securityApi, oidcApi, featuresApi, ownerApi, type CreateUserInput, type RoleInput } from '@/api'
import type { User, Role, NotifConfig, OIDCConfig, APIToken } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { fmtDate } from '@/lib/format'
import { P, GLOBAL_PRIVILEGES, roleDescription } from '@/lib/privileges'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import RoleSelect from '@/components/ui/RoleSelect.vue'
import Toggle from '@/components/ui/Toggle.vue'
import BackupCard from '@/components/admin/BackupCard.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import {
  Plus, Trash2, KeyRound, Pencil, Send, Check, Copy,
  Users, ShieldCheck, Palette, Bell, Lock, Image as ImageIcon, SlidersHorizontal, Eye, LogIn, LogOut, VenetianMask,
  Crown, LockOpen, TriangleAlert, Terminal, UserX, UserCheck,
  Info,
} from 'lucide-vue-next'
import { usePrefsStore } from '@/stores/prefs'
import { DEV_THEME_LIST, resolveCrest, resolveThemeAccent, type DevThemeId } from '@/lib/devThemes'
import DevThemesCard from '@/components/admin/DevThemesCard.vue'

const auth = useAuthStore()
const toast = useToastStore()
const branding = useBrandingStore()
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

type Tab = 'users' | 'roles' | 'tokens' | 'branding' | 'notifications' | 'security' | 'sso' | 'owner'

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
  const sys: { id: Tab; label: string; icon: any; count?: number }[] = []
  // Branding/notifications are owner concerns and live under the Owner section.
  // A delegated (non-owner) admin granted the owner-gated privilege still manages
  // them here, so the delegation feature keeps working.
  if (!auth.isOwner && auth.canOwnerGated(P.settingsBranding)) sys.push({ id: 'branding', label: t('admin.branding'), icon: Palette })
  if (!auth.isOwner && auth.canOwnerGated(P.settingsNotifications)) sys.push({ id: 'notifications', label: t('admin.notifications'), icon: Bell })
  const groups: { title: string; items: typeof mgmt }[] = []
  if (mgmt.length) groups.push({ title: t('adm.management'), items: mgmt })
  if (sys.length) groups.push({ title: t('nav.system'), items: sys })
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
      toast.success(t('adm.users.created'))
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
    toast.success(t('adm.users.passwordCopied'))
  } catch {
    toast.error(t('adm.users.copyFailed'))
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
    toast.success(t('adm.users.updated'))
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
    toast.success(t('adm.users.otpResetDone'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function resetUserWebauthn() {
  if (!editUserTarget.value) return
  try {
    await usersApi.resetWebauthn(editUserTarget.value.id)
    toast.success(t('adm.users.passkeysRemoved'))
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
    toast.success(t('adm.users.deleted'))
    deleteUserTarget.value = null
    await reloadUsersAndFlash()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---------- Roles ----------
// A role is a set of instance privileges. What someone may do inside a
// workspace is their role there, set by its owners.
interface RoleForm {
  id?: string
  name: string
  description: string
  global_privileges: string[]
}
const showRole = ref(false)
const roleForm = reactive<RoleForm>({ name: '', description: '', global_privileges: [] })
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
  Object.assign(roleForm, { id: undefined, name: '', description: '', global_privileges: [] })
  showRole.value = true
}
function openEditRole(r: Role) {
  roleForm.id = r.id
  roleForm.name = r.name
  roleForm.description = r.description
  roleForm.global_privileges = [...r.global_privileges]
  showRole.value = true
}
async function saveRole() {
  const body: RoleInput = {
    name: roleForm.name,
    description: roleForm.description,
    global_privileges: roleForm.global_privileges,
  }
  // Editing a role we currently hold changes our own privileges — refresh them.
  const affectsSelf = !!roleForm.id && (auth.user?.roles ?? []).some((r) => r.id === roleForm.id)
  try {
    if (roleForm.id) await rolesApi.update(roleForm.id, body)
    else await rolesApi.create(body)
    toast.success(t('adm.roles.saved'))
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
    toast.success(t('adm.roles.deleted'))
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
const privLabel = (id: string) => GLOBAL_PRIVILEGES.find((p) => p.id === id)?.label ?? id
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

function roleSummary(r: Role): string {
  const parts: string[] = []
  if (r.superuser) return t('admin.fullAccess')
  if (r.global_privileges.length) parts.push(t('adm.roles.privCount', { n: r.global_privileges.length }, r.global_privileges.length))
  return parts.join(' · ') || t('adm.roles.noPrivs')
}

// ---------- Branding ----------
const accentSwatches = ['#007AFF', '#5856D6', '#AF52DE', '#FF2D55', '#FF3B30', '#FF9500', '#34C759', '#30B0C7']
const brandForm = reactive({
  app_name: '',
  accent_color: '',
  logo_data_url: '',
  favicon_data_url: '',
  show_app_name: true,
  show_logo: true,
  // Developer themes (a closed curated set; the accent is locked to the theme while on).
  developer_themes_enabled: false,
  developer_themes_personal: false,
  developer_theme: 'blue' as DevThemeId,
  asset_source_logo: 'theme' as 'theme' | 'keep',
  asset_source_wordmark: 'theme' as 'theme' | 'keep',
  asset_source_favicon: 'theme' as 'theme' | 'keep',
})
const customLoginMessage = ref(false)
const loginMessage = ref('')

// The curated themes (the grid) and the one selected.
const prefs = usePrefsStore()
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
  brandForm.developer_themes_personal = branding.devThemesPersonal
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
    toast.success(t('adm.branding.saved'))
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
  if (!cfg.webhook) cfg.webhook = { enabled: false, url: '', secret: '' }
  if (!cfg.teams) cfg.teams = { enabled: false, webhook_url: '' }
  if (!cfg.slack) cfg.slack = { enabled: false, webhook_url: '' }
  notifCfg.value = cfg
}
async function saveNotifConfig() {
  if (!notifCfg.value) return
  try {
    notifCfg.value = await notifyApi.updateConfig(notifCfg.value)
    toast.success(t('adm.notify.saved'))
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
    toast.success(t('adm.notify.testSent', { channel: ch }))
  } catch (e: any) {
    toast.error(t('adm.notify.testFailed', { error: e.message }))
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
    toast.success(v ? t('adm.security.enforced') : t('adm.security.unenforced'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setHideLocalLogin(v: boolean) {
  try {
    hideLocalLogin.value = (await securityApi.update({ hide_local_login: v })).hide_local_login
    toast.success(t('common.saved'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setPasswordless(v: boolean) {
  try {
    passwordlessEnabled.value = (await securityApi.update({ passwordless_enabled: v })).passwordless_enabled
    toast.success(t('common.saved'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setAutoSso(v: boolean) {
  try {
    autoSso.value = (await securityApi.update({ auto_sso: v })).auto_sso
    toast.success(t('common.saved'))
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
    toast.success(t('adm.sso.saved'))
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
    toast.success(t('adm.sso.discoveryOk'))
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
const featureMeta = computed<{ key: string; label: string; hint: string }[]>(() => [
  { key: 'sessions', label: t('adm.owner.feature.sessions'), hint: t('adm.owner.feature.sessionsHint') },
  { key: 'impersonation', label: t('adm.owner.feature.impersonation'), hint: t('adm.owner.feature.impersonationHint') },
  { key: 'calendar_subscriptions', label: t('adm.owner.feature.calSubs'), hint: t('adm.owner.feature.calSubsHint') },
  { key: 'calendar_feeds', label: t('adm.owner.feature.calFeeds'), hint: t('adm.owner.feature.calFeedsHint') },
  { key: 'git_integration', label: t('adm.owner.feature.git'), hint: t('adm.owner.feature.gitHint') },
  { key: 'workspace_invites', label: t('adm.owner.feature.invites'), hint: t('adm.owner.feature.invitesHint') },
  { key: 'personal_channels', label: t('adm.owner.feature.ownChannels'), hint: t('adm.owner.feature.ownChannelsHint') },
  { key: 'step_up_auth', label: t('adm.owner.feature.stepUp'), hint: t('adm.owner.feature.stepUpHint') },
])
const featureFlags = ref<Record<string, boolean>>({ ...auth.features })
async function toggleFeature(key: string, on: boolean) {
  const prev = featureFlags.value[key] ?? false
  featureFlags.value = { ...featureFlags.value, [key]: on }
  try {
    const updated = await featuresApi.update({ [key]: on })
    featureFlags.value = updated
    auth.features = updated // keep the app-wide flags in sync
    toast.success(on ? t('adm.owner.featureOn') : t('adm.owner.featureOff'))
  } catch (e: any) {
    featureFlags.value = { ...featureFlags.value, [key]: prev } // revert on failure
    toast.error(e.message)
  }
}

// Owner-managed security alerts (broadcast on high-signal actions).
const securityAlertMeta = computed<{ key: string; label: string }[]>(() => [
  { key: 'impersonation', label: t('adm.owner.alert.impersonation') },
  { key: 'owner_transfer', label: t('adm.owner.alert.ownerTransfer') },
  { key: 'admin_grant', label: t('adm.owner.alert.adminGrant') },
  { key: 'config_change', label: t('adm.owner.alert.configChange') },
  { key: 'refresh_reuse', label: t('adm.owner.alert.refreshReuse') },
  { key: 'backup_export', label: t('adm.owner.alert.backupExport') },
])
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
    toast.success(t('adm.owner.alertEmailSaved'))
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
    toast.success(t('adm.owner.transferred'))
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
  <div class="admin-page">
    <header class="mb-6">
      <h1 class="large-title">{{ isOwnerArea ? t('admin.ownerTitle') : t('admin.title') }}</h1>
      <p class="subhead mt-1">{{ isOwnerArea ? t('admin.ownerSubtitle') : t('admin.subtitle') }}</p>
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
              <div class="text-[12px] text-text-3">{{ roleDescription(r) || roleSummary(r) }}</div>
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
                <p v-if="!r.global_privileges.length" class="text-[12px] text-text-3">{{ t('admin.noPrivileges') }}</p>
              </template>
            </div>
          </div>
        </div>
      </div>
    </section>

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
                <span v-if="!tk.privileges?.length && !tk.read_only" class="text-text-3">{{ t('admin.tokenFull') }}</span>
                <span v-else class="flex flex-wrap gap-1">
                  <span v-if="tk.read_only" class="pill !text-[11px]">{{ t('settings.tokenReadOnly') }}</span>
                  <span v-for="p in tk.privileges ?? []" :key="p" class="pill !text-[11px]">{{ privLabel(p) }}</span>
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
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><SlidersHorizontal class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('adm.branding.identity') }}</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div><label class="label">{{ t('adm.branding.appName') }}</label><input v-model="brandForm.app_name" class="input" placeholder="Orbit" /></div>
          <div>
            <div class="lockable" :class="{ locked: brandForm.developer_themes_enabled }">
              <label class="label">{{ t('adm.branding.accent') }}</label>
              <div class="flex items-center gap-2">
                <input :value="brandForm.accent_color || '#007AFF'" type="color" class="h-10 w-12 shrink-0 rounded-lg border border-border bg-surface-2" @input="brandForm.accent_color = ($event.target as HTMLInputElement).value" />
                <input v-model="brandForm.accent_color" class="input font-mono" placeholder="#007AFF" />
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
              <span>{{ t('adm.branding.accentLocked') }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Developer themes (the shared card) -->
      <DevThemesCard
        v-model:enabled="brandForm.developer_themes_enabled"
        v-model:theme="brandForm.developer_theme"
        v-model:logo="brandForm.asset_source_logo"
        v-model:wordmark="brandForm.asset_source_wordmark"
        v-model:favicon="brandForm.asset_source_favicon"
        v-model:personal="brandForm.developer_themes_personal"
        :has-logo="!!brandForm.logo_data_url"
        :has-favicon="!!brandForm.favicon_data_url"
      />

      </div><!-- /left column -->

      <!-- RIGHT column: Assets → Display options → Live preview -->
      <div class="flex flex-col gap-5">
      <!-- Display options -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><SlidersHorizontal class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('adm.branding.display') }}</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div class="flex items-center justify-between gap-4" :class="{ 'opacity-50': brandForm.developer_themes_enabled }">
            <div><div class="text-[13px] font-semibold">{{ t('adm.branding.showLogo') }}</div><div class="text-[12px] text-text-3">{{ t('adm.branding.showLogoHint') }}</div></div>
            <Toggle v-model="brandForm.show_logo" :disabled="brandForm.developer_themes_enabled" />
          </div>
          <div class="flex items-center justify-between gap-4 border-t border-border pt-4" :class="{ 'opacity-50': brandForm.developer_themes_enabled }">
            <div><div class="text-[13px] font-semibold">{{ t('adm.branding.showName') }}</div><div class="text-[12px] text-text-3">{{ t('adm.branding.showNameHint') }}</div></div>
            <Toggle v-model="brandForm.show_app_name" :disabled="brandForm.developer_themes_enabled" />
          </div>
          <p v-if="brandForm.developer_themes_enabled" class="-mt-1 flex items-start gap-1.5 text-[11px] text-text-3"><Info class="h-3.5 w-3.5 shrink-0" />{{ t('adm.branding.byTheme') }}</p>
          <div class="border-t border-border pt-4">
            <div class="flex items-center justify-between gap-4">
              <div><div class="text-[13px] font-semibold">{{ t('adm.branding.loginMessage') }}</div><div class="text-[12px] text-text-3">{{ t('adm.branding.loginMessageHint') }}</div></div>
              <Toggle v-model="customLoginMessage" />
            </div>
            <textarea v-if="customLoginMessage" v-model="loginMessage" class="input mt-3 h-20" :placeholder="t('adm.branding.loginMessagePlaceholder')"></textarea>
          </div>
        </div>
      </div>
      <!-- Assets -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><ImageIcon class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('adm.branding.assets') }}</h3></div>
        <div class="flex flex-col gap-4 p-5">
          <div>
            <div class="lockable" :class="{ locked: previewLogoTheme }">
              <div class="mb-1.5 text-[12px] text-text-2"><span class="font-semibold text-text">{{ t('adm.branding.logo') }}</span> {{ t('adm.branding.logoHint') }}</div>
              <div class="mb-2 flex h-20 items-center justify-center rounded-xl border border-border bg-surface-2">
                <img v-if="brandForm.logo_data_url" :src="brandForm.logo_data_url" class="max-h-14 rounded" />
                <span v-else class="text-[12px] text-text-3">{{ t('adm.branding.noLogo') }}</span>
              </div>
              <div class="flex items-center justify-between gap-2 rounded-xl border border-dashed border-border-2 px-3 py-3">
                <span class="text-[13px]"><span class="font-medium">{{ t('adm.branding.uploadLogo') }}</span><br /><span class="text-[12px] text-text-3">{{ t('adm.branding.logoFormats') }}</span></span>
                <div class="flex shrink-0 items-center gap-2">
                  <button v-if="brandForm.logo_data_url" type="button" class="btn-ghost" @click="brandForm.logo_data_url = ''">{{ t('adm.remove') }}</button>
                  <label class="btn-ghost cursor-pointer">{{ t('adm.chooseFile') }}<input type="file" accept="image/*" class="hidden" @change="onFile($event, 'logo_data_url')" /></label>
                </div>
              </div>
            </div>
            <div class="lock-note" :class="{ show: previewLogoTheme }">
              <Info class="h-4 w-4 shrink-0" />
              <span>{{ t('adm.branding.logoLocked', { choice: brandForm.logo_data_url ? t('adm.branding.myLogo') : t('adm.branding.default') }) }}</span>
            </div>
          </div>
          <div class="border-t border-border pt-4">
            <div class="lockable" :class="{ locked: previewFavTheme }">
              <div class="mb-1.5 text-[12px] text-text-2"><span class="font-semibold text-text">{{ t('adm.branding.favicon') }}</span> {{ t('adm.branding.faviconHint') }}</div>
              <div class="flex items-center justify-between gap-2 rounded-xl border border-dashed border-border-2 px-3 py-3">
                <span class="flex items-center gap-3 text-[13px]">
                  <img v-if="brandForm.favicon_data_url" :src="brandForm.favicon_data_url" class="h-7 w-7 rounded" />
                  <span><span class="font-medium">{{ t('adm.branding.uploadFavicon') }}</span><br /><span class="text-[12px] text-text-3">{{ t('adm.branding.faviconFormats') }}</span></span>
                </span>
                <div class="flex shrink-0 items-center gap-2">
                  <button v-if="brandForm.favicon_data_url" type="button" class="btn-ghost" @click="brandForm.favicon_data_url = ''">{{ t('adm.remove') }}</button>
                  <label class="btn-ghost cursor-pointer">{{ t('adm.chooseFile') }}<input type="file" accept="image/*" class="hidden" @change="onFile($event, 'favicon_data_url')" /></label>
                </div>
              </div>
            </div>
            <div class="lock-note" :class="{ show: previewFavTheme }">
              <Info class="h-4 w-4 shrink-0" />
              <span>{{ t('adm.branding.faviconLocked', { choice: brandForm.favicon_data_url ? t('adm.branding.myFavicon') : t('adm.branding.default') }) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Live preview -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Eye class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('adm.branding.preview') }}</h3></div>
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
              </div>
            </div>
            <div class="flex items-center gap-3 p-3">
              <button class="btn-primary" :style="previewBtnStyle">{{ t('adm.branding.previewButton') }}</button>
              <a class="text-[12px] hover:underline" :style="{ color: previewAccent?.accent || 'var(--accent)' }">{{ t('adm.branding.previewLink') }}</a>
            </div>
          </div>
        </div>
      </div>

      </div><!-- /right column -->

      <div class="flex justify-end lg:col-span-2"><button class="btn-primary" @click="saveBranding"><Send class="h-4 w-4" /> {{ t('adm.branding.save') }}</button></div>
    </section>

    <!-- ===== Notifications ===== -->
    <section v-else-if="tab === 'notifications'">
      <div v-if="notifCfg" class="flex flex-col gap-4">
        <p class="text-[13px] text-text-2">
          {{ t('adm.notify.intro') }}
        </p>

        <div class="columns-1 gap-x-4 lg:columns-2 [&>*]:mb-4 [&>*]:break-inside-avoid">
        <!-- Email -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between">
            <h3 class="text-[14px] font-semibold">{{ t('adm.notify.email') }}</h3>
            <Toggle v-model="notifCfg.email.enabled" />
          </div>
          <div v-if="notifCfg.email.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">{{ t('adm.notify.host') }}</label><input v-model="notifCfg.email.host" class="input" placeholder="smtp.example.org" /></div>
              <div><label class="label">{{ t('adm.notify.port') }}</label><input v-model.number="notifCfg.email.port" type="number" class="input" placeholder="587" /></div>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">{{ t('admin.colUsername') }}</label><input v-model="notifCfg.email.username" class="input" /></div>
              <div><label class="label">{{ t('adm.password') }}</label><input v-model="notifCfg.email.password" type="password" class="input" /></div>
            </div>
            <div><label class="label">{{ t('adm.notify.from') }}</label><input v-model="notifCfg.email.from" class="input" placeholder="orbit@example.org" /></div>
            <Checkbox v-model="notifCfg.email.start_tls" class="text-text-2">{{ t('adm.notify.startTls') }}</Checkbox>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'email'" @click="testChannel('email')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Discord -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Discord</h3><Toggle v-model="notifCfg.discord.enabled" /></div>
          <div v-if="notifCfg.discord.enabled" class="flex flex-col gap-3">
            <div><label class="label">{{ t('adm.notify.webhookUrl') }}</label><input v-model="notifCfg.discord.webhook_url" class="input" placeholder="https://discord.com/api/webhooks/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'discord'" @click="testChannel('discord')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Telegram -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Telegram</h3><Toggle v-model="notifCfg.telegram.enabled" /></div>
          <div v-if="notifCfg.telegram.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">{{ t('adm.notify.botToken') }}</label><input v-model="notifCfg.telegram.bot_token" class="input" /></div>
              <div><label class="label">{{ t('adm.notify.chatId') }}</label><input v-model="notifCfg.telegram.chat_id" class="input" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'telegram'" @click="testChannel('telegram')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- ntfy -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">ntfy</h3><Toggle v-model="notifCfg.ntfy.enabled" /></div>
          <div v-if="notifCfg.ntfy.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">{{ t('adm.notify.serverUrl') }}</label><input v-model="notifCfg.ntfy.server_url" class="input" placeholder="https://ntfy.sh" /></div>
              <div><label class="label">{{ t('adm.notify.topic') }}</label><input v-model="notifCfg.ntfy.topic" class="input" /></div>
            </div>
            <div><label class="label">{{ t('adm.notify.tokenOptional') }}</label><input v-model="notifCfg.ntfy.token" class="input" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'ntfy'" @click="testChannel('ntfy')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Gotify -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Gotify</h3><Toggle v-model="notifCfg.gotify.enabled" /></div>
          <div v-if="notifCfg.gotify.enabled" class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div><label class="label">{{ t('adm.notify.serverUrl') }}</label><input v-model="notifCfg.gotify.server_url" class="input" placeholder="https://gotify.example.org" /></div>
              <div><label class="label">{{ t('adm.notify.appToken') }}</label><input v-model="notifCfg.gotify.token" class="input" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'gotify'" @click="testChannel('gotify')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Microsoft Teams -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Microsoft Teams</h3><Toggle v-model="notifCfg.teams.enabled" /></div>
          <div v-if="notifCfg.teams.enabled" class="flex flex-col gap-3">
            <i18n-t scope="global" keypath="adm.notify.teamsHint" tag="p" class="text-[12px] text-text-3"><template #path><b>{{ t('adm.notify.teamsPath') }}</b></template></i18n-t>
            <div><label class="label">{{ t('adm.notify.workflowUrl') }}</label><input v-model="notifCfg.teams.webhook_url" class="input" placeholder="https://….logic.azure.com/workflows/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'teams'" @click="testChannel('teams')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Slack -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">Slack</h3><Toggle v-model="notifCfg.slack.enabled" /></div>
          <div v-if="notifCfg.slack.enabled" class="flex flex-col gap-3">
            <i18n-t scope="global" keypath="adm.notify.slackHint" tag="p" class="text-[12px] text-text-3"><template #hook><b>{{ t('adm.notify.slackHook') }}</b></template></i18n-t>
            <div><label class="label">{{ t('adm.notify.webhookUrl') }}</label><input v-model="notifCfg.slack.webhook_url" class="input" placeholder="https://hooks.slack.com/services/…" /></div>
            <div class="flex justify-end"><button class="btn-ghost" :disabled="testing === 'slack'" @click="testChannel('slack')"><Send class="h-4 w-4" /> {{ t('adm.test') }}</button></div>
          </div>
        </div>

        <!-- Integration webhook -->
        <div class="card p-5">
          <div class="mb-3 flex items-center justify-between"><h3 class="text-[14px] font-semibold">{{ t('adm.notify.webhook') }}</h3><Toggle v-model="notifCfg.webhook.enabled" /></div>
          <div v-if="notifCfg.webhook.enabled" class="flex flex-col gap-3">
            <i18n-t scope="global" keypath="adm.notify.webhookHint" tag="p" class="text-[12px] text-text-3"><template #event><code>X-Orbit-Event: audit.user.create</code></template><template #header><code>X-Orbit-Signature</code></template></i18n-t>
            <div><label class="label">URL</label><input v-model="notifCfg.webhook.url" class="input" placeholder="https://hooks.example.org/orbit" /></div>
            <div><label class="label">{{ t('adm.notify.secret') }}</label><input v-model="notifCfg.webhook.secret" type="password" class="input" :placeholder="t('adm.notify.secretPlaceholder')" /></div>
          </div>
        </div>

        </div>
        <!-- /channel grid -->

        <p class="text-[12.5px] text-text-3">{{ t('adm.notify.perUser') }}</p>

        <div class="flex justify-end"><button class="btn-primary" @click="saveNotifConfig">{{ t('common.save') }}</button></div>
      </div>
      <div v-else class="text-[13px] text-text-3">{{ t('common.loading') }}</div>
    </section>

    <!-- ===== Security ===== -->
    <section v-else-if="tab === 'security'" class="max-w-3xl">
      <div class="card flex flex-col divide-y divide-border">
        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">{{ t('adm.security.enforce') }}</div>
            <div class="text-[12px] text-text-3">
              {{ t('adm.security.enforceHint') }}
            </div>
          </div>
          <Toggle :model-value="otpEnforced" @update:model-value="setEnforce" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">{{ t('adm.security.hideLocal') }}</div>
            <i18n-t scope="global" keypath="adm.security.hideLocalHint" tag="div" class="text-[12px] text-text-3">
              <template #path><code>/login?local=1</code></template>
            </i18n-t>
          </div>
          <Toggle :model-value="hideLocalLogin" @update:model-value="setHideLocalLogin" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">{{ t('adm.security.autoSso') }}</div>
            <div class="text-[12px] text-text-3">
              <i18n-t scope="global" keypath="adm.security.autoSsoHint" tag="span">
                <template #path><code>/login?local=1</code></template>
              </i18n-t>
              <span v-if="!branding.oidcEnabled" class="text-text-3">{{ t('adm.security.enableSsoFirst') }}</span>
            </div>
          </div>
          <Toggle :model-value="autoSso" :disabled="!branding.oidcEnabled" @update:model-value="setAutoSso" />
        </div>

        <div class="flex items-center justify-between gap-4 p-5">
          <div>
            <div class="text-[13px] font-semibold">{{ t('adm.security.passwordless') }}</div>
            <div class="text-[12px] text-text-3">
              {{ t('adm.security.passwordlessHint') }}
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
            <h3 class="text-[14px] font-semibold">{{ t('adm.sso.title') }}</h3>
            <div class="text-[12px] text-text-3">{{ t('adm.sso.hint') }}</div>
          </div>
          <Toggle v-model="oidcCfg.enabled" />
        </div>
        <div class="flex flex-col gap-3">
          <div>
            <label class="label">{{ t('adm.sso.issuer') }}</label>
            <div class="flex gap-2">
              <input v-model="oidcCfg.issuer" class="input" placeholder="https://idp.example.com/realms/main" />
              <button type="button" class="btn-ghost shrink-0" @click="testOIDC">{{ t('adm.test') }}</button>
            </div>
            <i18n-t scope="global" keypath="adm.sso.issuerHint" tag="p" class="mt-1 text-[11px] text-text-3"><template #path><code>/.well-known/openid-configuration</code></template></i18n-t>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div><label class="label">{{ t('adm.sso.clientId') }}</label><input v-model="oidcCfg.client_id" class="input" /></div>
            <div>
              <label class="label">{{ t('adm.sso.clientSecret') }}</label>
              <input v-model="oidcCfg.client_secret" type="password" class="input" :placeholder="oidcCfg.has_secret ? t('adm.sso.secretUnchanged') : ''" />
            </div>
          </div>
          <div>
            <label class="label">{{ t('adm.sso.redirect') }}</label>
            <input v-model="oidcCfg.redirect_url" class="input" placeholder="https://orbit.example.org/api/v1/auth/oidc/callback" />
            <p class="mt-1 text-[11px] text-text-3">{{ t('adm.sso.redirectHint') }}</p>
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div><label class="label">{{ t('adm.sso.scopes') }}</label><input v-model="oidcCfg.scopes" class="input" placeholder="groups" /></div>
            <div><label class="label">{{ t('adm.sso.roleClaim') }}</label><input v-model="oidcCfg.role_claim" class="input" placeholder="groups" /></div>
            <div><label class="label">{{ t('adm.sso.defaultRole') }}</label><input v-model="oidcCfg.default_role" class="input" :placeholder="t('adm.sso.defaultRoleNone')" /></div>
          </div>
          <p class="text-[11px] text-text-3">
            {{ t('adm.sso.provisioning') }}
          </p>
          <div class="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-2 px-4 py-3">
            <div>
              <div class="text-[13px] font-semibold">{{ t('admin.ssoAllowAdmin') }}</div>
              <div class="text-[12px] text-text-3">{{ t('admin.ssoAllowAdminHint') }}</div>
            </div>
            <Toggle :model-value="!!oidcCfg.allow_admin" @update:model-value="(v) => oidcCfg && (oidcCfg.allow_admin = v)" />
          </div>
          <div class="flex justify-end"><button class="btn-primary" :disabled="oidcBusy" @click="saveOIDC">{{ oidcBusy ? t('common.saving') : t('adm.sso.save') }}</button></div>
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
          <Crown class="h-[18px] w-[18px] text-violet" /><h3 class="text-[14px] font-semibold">{{ t('adm.owner.flags') }}</h3>
        </div>
        <div class="flex flex-col gap-4 p-5">
          <p class="text-[12px] text-text-3">{{ t('adm.owner.flagsHint') }}</p>
          <div v-for="f in featureMeta" :key="f.key" class="flex items-center justify-between gap-4">
            <div>
              <div class="text-[13px] font-semibold">{{ f.label }}</div>
              <div class="text-[12px] text-text-3">{{ f.hint }}</div>
            </div>
            <Toggle :model-value="featureFlags[f.key] ?? false" :aria-label="f.label" @update:model-value="(v) => toggleFeature(f.key, v)" />
          </div>
        </div>
      </div>

      </div><!-- /left column -->

      <!-- RIGHT column: Security alerts -->
      <div class="flex flex-col gap-5">
      <!-- Security alerts -->
      <div class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <Bell class="h-[18px] w-[18px] text-violet" /><h3 class="text-[14px] font-semibold">{{ t('adm.owner.alerts') }}</h3>
        </div>
        <div class="flex flex-col gap-4 p-5">
          <p class="text-[12px] text-text-3">
            {{ t('adm.owner.alertsHint') }}
          </p>
          <div v-for="a in securityAlertMeta" :key="a.key" class="flex items-center justify-between gap-4">
            <div class="text-[13px]">{{ a.label }}</div>
            <Toggle :model-value="alertEvents[a.key] ?? false" @update:model-value="(v) => toggleSecurityAlert(a.key, v)" />
          </div>

          <div class="border-t border-border pt-4">
            <div class="mb-2 text-[12px] font-semibold text-text-2">{{ t('adm.owner.notifyVia') }}</div>
            <p v-if="alertAvailable.length === 0" class="text-[12px] text-text-3">
              {{ t('adm.owner.noChannels') }}
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
            <label class="label">{{ t('adm.owner.alertEmail') }}</label>
            <div class="flex gap-2">
              <input v-model="alertEmail" class="input" :placeholder="t('adm.owner.alertEmailPlaceholder')" />
              <button class="btn-ghost shrink-0" @click="saveAlertEmail">{{ t('common.save') }}</button>
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
    <Overlay :open="showUser" :title="t('admin.newUser')" mode="centered" @close="showUser = false">
      <form class="flex flex-col gap-4" @submit.prevent="createUser">
        <div><label class="label">{{ t('admin.colUsername') }}</label><input v-model="userForm.username" class="input" /></div>
        <div><label class="label">{{ t('admin.displayName') }}</label><input v-model="userForm.display_name" class="input" /></div>
        <div><label class="label">{{ t('admin.colEmail') }}</label><input v-model="userForm.email" type="email" class="input" /></div>
        <Checkbox v-model="userForm.generate_password" class="text-text-2">{{ t('admin.generateTemp') }}</Checkbox>
        <div v-if="!userForm.generate_password">
          <label class="label">{{ t('adm.users.passwordMin') }}</label>
          <input v-model="userForm.password" type="password" class="input" />
        </div>
        <div><label class="label">{{ t('admin.colRoles') }}</label><RoleSelect v-model="userForm.role_ids" :roles="roles" /></div>
        <Checkbox v-if="!userForm.generate_password" v-model="userForm.must_change_password" class="text-text-2">{{ t('adm.users.mustChangeFirst') }}</Checkbox>
        <Checkbox v-model="userForm.otp_required" class="text-text-2">{{ t('admin.requireOtp') }}</Checkbox>
        <div class="mt-1 flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showUser = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary">{{ t('adm.users.createUser') }}</button>
        </div>
      </form>
    </Overlay>

    <!-- ===== Generated credentials (one-time) ===== -->
    <Overlay :open="!!createdCreds" :title="createdCreds?.reset ? t('admin.passwordReset') : t('adm.users.created')" mode="centered" @close="createdCreds = null">
      <div v-if="createdCreds" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
          {{ t('adm.users.credsOnce') }}
        </p>
        <div class="grid grid-cols-[auto,1fr] items-center gap-x-3 gap-y-1.5">
          <span class="text-text-3">{{ t('admin.colUsername') }}</span><span class="font-medium">{{ createdCreds.username }}</span>
          <span class="text-text-3">{{ t('adm.password') }}</span><code class="select-all rounded bg-surface-2 px-2 py-1 font-mono">{{ createdCreds.password }}</code>
        </div>
        <div class="mt-1 flex justify-end gap-2">
          <button class="btn-ghost" @click="copyCreds"><Copy class="h-4 w-4" /> {{ t('adm.users.copyPassword') }}</button>
          <button class="btn-primary" @click="createdCreds = null">{{ t('common.done') }}</button>
        </div>
      </div>
    </Overlay>

    <!-- ===== Edit user modal ===== -->
    <Overlay :open="showEditUser" :title="t('adm.users.editUser')" mode="centered" @close="showEditUser = false">
      <form class="flex flex-col gap-4" @submit.prevent="saveEditUser">
        <i18n-t scope="global" v-if="editUserTarget?.oidc" keypath="adm.users.ssoAccount" tag="div" class="rounded-xl border border-border bg-surface-2 px-3 py-2 text-[12px] text-text-3">
          <template #sso><b>SSO</b></template>
        </i18n-t>
        <div>
          <label class="label">{{ t('admin.displayName') }}</label>
          <input v-model="editUserForm.display_name" class="input" :disabled="editUserTarget?.oidc" />
        </div>
        <div>
          <label class="label">{{ t('admin.colEmail') }}</label>
          <input v-model="editUserForm.email" type="email" class="input" :disabled="editUserTarget?.oidc" />
        </div>
        <div><label class="label">{{ t('admin.colRoles') }}</label><RoleSelect v-model="editUserForm.role_ids" :roles="roles" /></div>

        <div v-if="editUserTarget?.id !== auth.user?.id" class="flex items-center justify-between border-t border-border pt-4">
          <div>
            <div class="text-[13px] font-semibold">{{ t('adm.users.disableAccount') }}</div>
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
            <div class="text-[13px] font-semibold">{{ t('settings.twofa') }}</div>
            <div class="text-[12px] text-text-3">
              {{ editUserTarget?.otp_enabled ? t('adm.users.otpOn') : t('adm.users.otpOff') }}
            </div>
          </div>
          <button type="button" class="btn-ghost" :disabled="!editUserTarget?.otp_enabled" @click="resetUserOtp">
            {{ t('adm.users.resetOtp') }}
          </button>
        </div>

        <div class="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-2 px-4 py-3">
          <div>
            <div class="text-[13px] font-semibold">{{ t('settings.passkeys') }}</div>
            <div class="text-[12px] text-text-3">{{ t('adm.users.passkeysHint') }}</div>
          </div>
          <button type="button" class="btn-ghost" @click="resetUserWebauthn">{{ t('adm.users.resetPasskeys') }}</button>
        </div>

        <div class="mt-1 flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showEditUser = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary">{{ t('adm.users.saveChanges') }}</button>
        </div>
      </form>
    </Overlay>

    <!-- ===== Role editor modal ===== -->
    <Overlay :open="showRole" :title="roleForm.id ? t('adm.roles.editRole') : t('admin.newRole')" mode="centered" wide @close="showRole = false">
      <form class="flex flex-col gap-6" @submit.prevent="saveRole">
        <!-- Name + description -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="label">{{ t('common.name') }} <span class="text-text-3">{{ t('adm.roles.required') }}</span></label>
            <input v-model="roleForm.name" class="input" :placeholder="t('adm.roles.namePlaceholder')" />
          </div>
          <div>
            <label class="label">{{ t('admin.colDescription') }} <span class="text-text-3">{{ t('adm.roles.optional') }}</span></label>
            <input v-model="roleForm.description" class="input" :placeholder="t('adm.roles.descPlaceholder')" />
          </div>
        </div>

        <!-- Global privileges -->
        <div>
          <div class="mb-2 flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-[11px] font-semibold uppercase tracking-wide text-text-3">{{ t('adm.roles.globalPrivs') }}</span>
              <span class="rounded-full bg-surface-3 px-1.5 py-0.5 text-[11px] text-text-2">{{ t('adm.roles.selected', { n: roleForm.global_privileges.length, total: GLOBAL_PRIVILEGES.length }) }}</span>
            </div>
            <button type="button" class="btn-ghost px-2.5 py-1" @click="toggleAllGlobal">{{ allGlobalSelected ? t('adm.roles.clearAll') : t('adm.roles.selectAll') }}</button>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <button
              v-for="gp in GLOBAL_PRIVILEGES"
              :key="gp.id"
              type="button"
              :disabled="gp.ownerOnly && !auth.isOwner"
              :title="gp.ownerOnly && !auth.isOwner ? t('adm.roles.ownerOnly') : ''"
              class="flex items-start gap-3 rounded-xl border p-3 text-left transition disabled:cursor-not-allowed disabled:opacity-50"
              :class="roleForm.global_privileges.includes(gp.id) ? 'border-accent bg-accent-bg' : 'border-border bg-surface-2 hover:border-border-2'"
              @click="toggle(roleForm.global_privileges, gp.id)"
            >
              <component :is="gp.icon" class="mt-0.5 h-[18px] w-[18px] shrink-0 text-text-2" />
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5 text-[13px] font-medium">{{ gp.label }}<Lock v-if="gp.ownerOnly && !auth.isOwner" class="h-3 w-3 text-text-3" /></div>
                <div class="text-[11px] text-text-3">{{ gp.desc }}</div>
              </div>
              <span class="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center rounded border" :class="roleForm.global_privileges.includes(gp.id) ? 'border-accent bg-accent text-accent-fg' : 'border-border-2'">
                <Check v-if="roleForm.global_privileges.includes(gp.id)" class="h-3 w-3" />
              </span>
            </button>
          </div>
        </div>

        <p class="rounded-xl bg-fill-2 px-3 py-2.5 text-[12.5px] text-text-2">
          {{ t('adm.roles.explain') }}
        </p>

        <!-- Footer -->
        <div class="flex items-center justify-between border-t border-border pt-4">
          <span class="text-[12px] text-text-3">{{ t('adm.roles.applyNow') }}</span>
          <div class="flex gap-2">
            <button type="button" class="btn-ghost" @click="showRole = false">{{ t('common.cancel') }}</button>
            <button type="submit" class="btn-primary">{{ t('adm.roles.saveRole') }}</button>
          </div>
        </div>
      </form>
    </Overlay>

    <!-- ===== Reset password modal ===== -->
    <Overlay :open="!!resetTarget" :title="t('admin.resetPassword')" mode="centered" @close="resetTarget = null">
      <form class="flex flex-col gap-3" @submit.prevent="resetPassword">
        <i18n-t scope="global" keypath="adm.reset.setFor" tag="p" class="text-[13px] text-text-2"><template #user><b>{{ resetTarget?.username }}</b></template></i18n-t>
        <Checkbox v-model="resetGenerate" class="text-text-2">{{ t('admin.generateTemp') }}</Checkbox>
        <template v-if="!resetGenerate">
          <input v-model="newPassword" type="password" class="input" :placeholder="t('adm.reset.newPassword')" />
          <Checkbox v-model="resetMustChange" class="text-text-2">{{ t('adm.reset.mustChange') }}</Checkbox>
        </template>
        <p class="text-[12px] text-text-3">{{ t('adm.reset.signsOut') }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="resetTarget = null">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary">{{ t('adm.reset.reset') }}</button>
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
      :title="t('adm.users.deleteTitle')"
      :message="t('adm.users.deleteConfirm', { name: deleteUserTarget?.username })"
      :confirm-label="t('common.delete')"
      danger
      @confirm="removeUser"
      @close="deleteUserTarget = null"
    />
    <Confirm
      :open="!!deleteRoleTarget"
      :title="t('adm.roles.deleteTitle')"
      :message="t('adm.roles.deleteConfirm', { name: deleteRoleTarget?.name })"
      :confirm-label="t('common.delete')"
      danger
      @confirm="removeRole"
      @close="deleteRoleTarget = null"
    />
  </div>
</template>

<style scoped>
.admin-page {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
}
</style>

<script setup lang="ts">
import NotificationSettings from '@/components/settings/NotificationSettings.vue'
import GitSettings from '@/components/settings/GitSettings.vue'
import DevThemePicker from '@/components/DevThemePicker.vue'
import { useBrandingStore } from '@/stores/branding'
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n, I18nT } from 'vue-i18n'
import { setLocale, currentLocale, SUPPORTED_LOCALES, type LocaleCode } from '@/i18n'
import { usePrefsStore, type Theme } from '@/stores/prefs'
import { api, setAccessToken } from '@/lib/api'
import { meApi, sessionsApi, type SessionInfo } from '@/api'
import { passkeysApi, webauthnSupported } from '@/api/webauthn'
import type { WebAuthnCredential } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { fmtDate, parseUA, relTime } from '@/lib/format'
import Avatar from '@/components/ui/Avatar.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import OtpSetup from '@/components/ui/OtpSetup.vue'
import CodeInput from '@/components/ui/CodeInput.vue'
import { User, Palette, Globe, Lock, Bell, ShieldCheck, KeyRound, Fingerprint, Trash2, Terminal, Copy, Sun, Moon, Monitor, Eye, EyeOff, Upload, MonitorSmartphone, Smartphone, Laptop, LogOut, CalendarRange, Plus, ExternalLink, GitBranch, Bot } from 'lucide-vue-next'
import CopyButton from '@/components/ui/CopyButton.vue'
import { calendarApi, type Feed } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import type { APIToken } from '@/api/types'
import { GLOBAL_PRIVILEGES, isOwnerOnlyPriv } from '@/lib/privileges'

const prefs = usePrefsStore()
const branding = useBrandingStore()
const auth = useAuthStore()
const wss = useWorkspacesStore()
const { t } = useI18n()

// ---- Language ----
const locale = ref<LocaleCode>(currentLocale())
function changeLocale(code: LocaleCode) {
  locale.value = code
  setLocale(code)
  void prefs.persist({ locale: code })
}
const toast = useToastStore()

const sections = computed(() => [
  { group: t('settings.personal'), items: [
    { id: 'account', label: t('settings.account'), icon: User },
    { id: 'appearance', label: t('settings.appearance'), icon: Palette },
    { id: 'language', label: t('settings.languageTime'), icon: Globe },
    { id: 'notifications', label: t('settings.notifications'), icon: Bell },
    ...(auth.features.git_integration !== false ? [{ id: 'git', label: t('git.settings.title'), icon: GitBranch }] : []),
    ...(auth.features.calendar_feeds !== false ? [{ id: 'feeds', label: t('settings.feeds'), icon: CalendarRange }] : []),
  ] },
  { group: t('settings.security'), items: [
    { id: 'password', label: t('settings.password'), icon: Lock },
    // TOTP is unusable for SSO accounts (no password login), so hide it for them.
    ...(auth.user?.oidc ? [] : [{ id: 'twofa', label: t('settings.twofa'), icon: ShieldCheck }]),
    ...(passkeysAvailable.value ? [{ id: 'passkeys', label: t('settings.passkeys'), icon: KeyRound }] : []),
    { id: 'apitokens', label: t('settings.apitokens'), icon: Terminal },
    ...(auth.features.sessions ? [{ id: 'sessions', label: t('settings.sessions'), icon: MonitorSmartphone }] : []),
  ] },
])
const active = ref('account')
function go(id: string) {
  active.value = id
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}
// /settings#git opens at that section (links from elsewhere in the app).
const route = useRoute()
onMounted(() => {
  const id = route.hash.slice(1)
  // After the router's own scroll to the top.
  if (id) void nextTick(() => setTimeout(() => go(id), 150))
})

// ---- Account ----
const displayName = ref(auth.user?.display_name ?? '')
const email = ref(auth.user?.email ?? '')
const avatarUrl = ref(auth.user?.avatar_url ?? '')
const avatarBroken = ref(false)
function onAvatar(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    avatarUrl.value = reader.result as string
    avatarBroken.value = false
  }
  reader.readAsDataURL(file)
}

async function saveAccount() {
  try {
    const updated = await meApi.updateProfile({
      email: email.value,
      display_name: displayName.value,
      avatar_url: avatarBroken.value ? '' : avatarUrl.value,
    })
    if (avatarBroken.value) avatarUrl.value = ''
    auth.user = updated
    toast.success(t('acct.account.updated'))
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Appearance ----
const themes: { value: Theme; label: string; icon: any }[] = [
  { value: 'light', label: t('settings.themeLight'), icon: Sun },
  { value: 'dark', label: t('settings.themeDark'), icon: Moon },
  { value: 'system', label: t('settings.themeSystem'), icon: Monitor },
]
const accentSwatches = ['#007AFF', '#5856D6', '#AF52DE', '#FF2D55', '#FF3B30', '#FF9500', '#34C759', '#30B0C7']

// ---- Language & time ----
const deviceTZ = Intl.DateTimeFormat().resolvedOptions().timeZone
const explicitZones = [
  'UTC', deviceTZ, 'Europe/Lisbon', 'Europe/London', 'Europe/Madrid',
  'Europe/Berlin', 'America/New_York', 'America/Sao_Paulo', 'Asia/Tokyo',
].filter((v, i, a) => a.indexOf(v) === i)

// ---- Calendar feeds ----
const feeds = ref<Feed[]>([])
const feedSheet = ref(false)
const feedName = ref('')
const feedAll = ref(true)
const feedWs = ref<string[]>([])
const feedTasks = ref(true)
const feedResult = ref<{ url: string; webcal: string } | null>(null)
const confirmFeed = ref<Feed | null>(null)
async function loadFeeds() {
  if (auth.features.calendar_feeds === false) return
  try {
    feeds.value = await calendarApi.feeds()
  } catch {
    /* ignore */
  }
}
function openFeed() {
  feedName.value = 'Orbit'
  feedAll.value = true
  feedWs.value = []
  feedTasks.value = true
  feedResult.value = null
  feedSheet.value = true
}
async function createFeed() {
  try {
    const r = await calendarApi.createFeed({ name: feedName.value.trim(), workspace_ids: feedAll.value ? [] : feedWs.value, include_tasks: feedTasks.value })
    feedResult.value = { url: r.url, webcal: r.webcal }
    await loadFeeds()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function revokeFeed() {
  const f = confirmFeed.value
  confirmFeed.value = null
  if (!f) return
  try {
    await calendarApi.removeFeed(f.id)
    await loadFeeds()
  } catch (e: any) {
    toast.error(e.message)
  }
}
function copyText(v: string) {
  navigator.clipboard.writeText(v)
  toast.success(t('settings.copied'))
}
const feedWsNames = (f: Feed) => (f.workspace_ids.length ? f.workspace_ids.map((id) => wss.byId(id)?.name ?? '…').join(', ') : t('settings.feedAllWs'))
onMounted(loadFeeds)

// ---- Password ----
const oldPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const showOld = ref(false)
const showNew = ref(false)
const showConfirm = ref(false)
const pwMsg = ref('')
const pwErr = ref('')
async function changePassword() {
  pwMsg.value = ''
  pwErr.value = ''
  if (newPw.value !== confirmPw.value) {
    pwErr.value = t('acct.password.mismatch')
    return
  }
  try {
    // Changing the password signs out every other session; this one continues
    // on the fresh access token returned by the API.
    const res = await api<{ access_token?: string }>('/me/password', {
      method: 'PUT',
      body: JSON.stringify({ old_password: oldPw.value, new_password: newPw.value }),
    })
    if (res?.access_token) setAccessToken(res.access_token)
    pwMsg.value = t('acct.password.updatedOthersOut')
    oldPw.value = ''
    newPw.value = ''
    confirmPw.value = ''
  } catch (e: any) {
    pwErr.value = e?.message || t('acct.password.cantChange')
  }
}

// ---- Two-factor ----
const showOtpSetup = ref(false)
const disableCode = ref('')
const otpErr = ref('')
const showDisable = ref(false)
function openDisable() {
  disableCode.value = ''
  otpErr.value = ''
  showDisable.value = true
}
const showRegen = ref(false)
const regenCode = ref('')
const regenCodes = ref<string[]>([])
const regenErr = ref('')
function openRegen() {
  regenCode.value = ''
  regenCodes.value = []
  regenErr.value = ''
  showRegen.value = true
}
async function regenerate() {
  if (regenCode.value.length < 6) return
  regenErr.value = ''
  try {
    const r = await meApi.otpRegenerateBackup(regenCode.value.trim())
    regenCodes.value = r.backup_codes
    await auth.fetchMe()
  } catch (e: any) {
    regenErr.value = e?.message || t('acct.otp.invalidCode')
    regenCode.value = ''
  }
}
async function onOtpVerified() {
  showOtpSetup.value = false
  await auth.fetchMe()
  toast.success(t('acct.otp.enabledToast'))
}
async function disableOtp() {
  if (disableCode.value.length < 6) return
  otpErr.value = ''
  try {
    await meApi.otpDisable(disableCode.value.trim())
    disableCode.value = ''
    showDisable.value = false
    await auth.fetchMe()
    toast.success(t('acct.otp.disabledToast'))
  } catch (e: any) {
    otpErr.value = e?.message || t('acct.otp.cantDisable')
    disableCode.value = ''
  }
}

// ---- Passkeys ----
const passkeysAvailable = computed(() => auth.webauthnEnabled && webauthnSupported())
const passkeys = ref<WebAuthnCredential[]>([])
const showPkName = ref(false)
const pkName = ref('')
const pkErr = ref('')
const pkBusy = ref(false)
const confirmPk = ref<WebAuthnCredential | null>(null)

async function loadPasskeys() {
  try {
    passkeys.value = (await passkeysApi.list()) ?? []
  } catch {
    /* ignore */
  }
}
function openAddPasskey() {
  pkName.value = ''
  pkErr.value = ''
  showPkName.value = true
}
async function createPasskey() {
  pkErr.value = ''
  pkBusy.value = true
  try {
    await passkeysApi.register(pkName.value.trim() || 'Passkey')
    showPkName.value = false
    await loadPasskeys()
    await auth.fetchMe()
    toast.success(t('acct.passkeys.added'))
  } catch (e: any) {
    pkErr.value = e?.name === 'NotAllowedError' ? t('acct.passkeys.cancelled') : e?.message || t('acct.passkeys.cantAdd')
  } finally {
    pkBusy.value = false
  }
}
async function removePasskey() {
  const cred = confirmPk.value
  confirmPk.value = null
  if (!cred) return
  try {
    await passkeysApi.remove(cred.id)
    await loadPasskeys()
    await auth.fetchMe()
    toast.success(t('acct.passkeys.removed'))
  } catch (e: any) {
    toast.error(e?.message || t('acct.passkeys.cantRemove'))
  }
}
onMounted(loadPasskeys)

// ---- Assistants (MCP) ----
const mcpUrl = `${location.origin}/mcp`

// ---- API tokens ----
const tokens = ref<APIToken[]>([])
const showTokenCreate = ref(false)
const tkName = ref('')
const tkExpiry = ref(0) // days; 0 = never
const tkBusy = ref(false)
const tkErr = ref('')
const createdToken = ref('') // shown once
// Optional narrowing: fewer instance privileges, and/or read-only access to
// workspace content.
const tkLimit = ref(false)
const tkPrivs = ref<string[]>([])
const tkReadOnly = ref(false)
// Only what you hold can be given to a token (owner status never is).
const tokenPrivChoices = computed(() => GLOBAL_PRIVILEGES.filter((p) => (isOwnerOnlyPriv(p.id) ? auth.privileges.global.includes(p.id) : auth.can(p.id))))
const privLabel = (id: string) => GLOBAL_PRIVILEGES.find((p) => p.id === id)?.label ?? id
function toggleTkPriv(id: string, on: boolean) {
  tkPrivs.value = on ? [...tkPrivs.value, id] : tkPrivs.value.filter((p) => p !== id)
}
const confirmTk = ref<APIToken | null>(null)

async function loadTokens() {
  try {
    tokens.value = (await meApi.apiTokens()) ?? []
  } catch {
    /* ignore */
  }
}
function openCreateToken() {
  tkName.value = ''
  tkExpiry.value = 0
  tkErr.value = ''
  tkLimit.value = false
  tkPrivs.value = []
  tkReadOnly.value = false
  createdToken.value = ''
  showTokenCreate.value = true
}
async function createToken() {
  tkErr.value = ''
  tkBusy.value = true
  try {
    const privs = tkLimit.value ? tkPrivs.value : []
    const r = await meApi.createApiToken(tkName.value.trim() || 'token', tkExpiry.value, privs, tkReadOnly.value)
    createdToken.value = r.token
    await loadTokens()
  } catch (e: any) {
    tkErr.value = e?.message || t('acct.tokens.cantCreate')
  } finally {
    tkBusy.value = false
  }
}
function copyToken() {
  navigator.clipboard.writeText(createdToken.value)
  toast.success(t('acct.tokens.copied'))
}
async function revokeToken() {
  const tk = confirmTk.value
  confirmTk.value = null
  if (!tk) return
  try {
    await meApi.revokeApiToken(tk.id)
    await loadTokens()
    toast.success(t('acct.tokens.revoked'))
  } catch (e: any) {
    toast.error(e?.message || t('acct.tokens.cantRevoke'))
  }
}
onMounted(loadTokens)

// ---- Active sessions (feature-flagged) ----
const sessions = ref<SessionInfo[]>([])
const sessionsBusy = ref(false)
const sessionDevice = (ua: string) => {
  const { os, browser } = parseUA(ua)
  return `${os} · ${browser}`
}
const sessionIcon = (ua: string) => {
  const os = parseUA(ua).os
  if (os === 'iOS' || os === 'Android') return Smartphone
  if (os === 'macOS' || os === 'Linux') return Laptop
  return Monitor
}
const sessionRel = (s: SessionInfo) => (s.current ? t('acct.sessions.activeNow') : relTime(s.last_used_at))
async function loadSessions() {
  if (!auth.features.sessions) return
  try {
    sessions.value = (await sessionsApi.list()).sessions
  } catch {
    /* feature may be off or transient */
  }
}
async function revokeSession(sid: string) {
  sessionsBusy.value = true
  try {
    await sessionsApi.revoke(sid)
    await loadSessions()
    toast.success(t('acct.sessions.revoked'))
  } catch (e: any) {
    toast.error(e?.message || t('acct.sessions.cantRevoke'))
  } finally {
    sessionsBusy.value = false
  }
}
async function revokeOtherSessions() {
  sessionsBusy.value = true
  try {
    await sessionsApi.revokeOthers()
    await loadSessions()
    toast.success(t('acct.sessions.signedOutOthers'))
  } catch (e: any) {
    toast.error(e?.message || t('acct.sessions.cantSignOutOthers'))
  } finally {
    sessionsBusy.value = false
  }
}
onMounted(loadSessions)
</script>

<template>
  <div class="settings-page">
    <header class="mb-7">
      <h1 class="large-title">{{ t('nav.settings') }}</h1>
      <p class="subhead mt-1">{{ t('settings.subtitle') }}</p>
    </header>

    <div class="flex gap-8">
      <!-- Left sub-nav -->
      <nav class="sticky top-0 hidden w-48 shrink-0 flex-col gap-4 self-start md:flex">
        <div v-for="grp in sections" :key="grp.group">
          <div class="mb-1.5 px-2 text-[11px] font-semibold uppercase tracking-wide text-text-3">{{ grp.group }}</div>
          <div class="flex flex-col gap-1">
            <button
              v-for="it in grp.items"
              :key="it.id"
              class="flex items-center gap-2.5 rounded-xl border px-3 py-2 text-[13px] transition"
              :class="active === it.id ? 'border-border-2 bg-surface text-text' : 'border-transparent text-text-2 hover:bg-surface-2 hover:text-text'"
              @click="go(it.id)"
            >
              <component :is="it.icon" class="h-[18px] w-[18px]" />
              {{ it.label }}
            </button>
          </div>
        </div>
      </nav>

      <div class="sections min-w-0 flex-1">
        <!-- Account -->
        <section id="account" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><User class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.account') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <img v-if="avatarUrl && !avatarBroken" :src="avatarUrl" class="h-12 w-12 rounded-full object-cover" @error="avatarBroken = true" />
                <Avatar v-else :name="displayName || auth.user?.username || '?'" :size="48" />
                <div>
                  <div class="text-[14px] font-semibold">{{ displayName || auth.user?.username }}</div>
                  <div class="text-[12px] text-text-3">{{ auth.user?.username }}</div>
                </div>
              </div>
              <label class="btn-ghost cursor-pointer">
                <Upload class="h-4 w-4" /> {{ t('acct.account.uploadPhoto') }}
                <input type="file" accept="image/*" class="hidden" @change="onAvatar" />
              </label>
            </div>
            <div v-if="auth.user?.oidc" class="rounded-xl border border-border bg-surface-2 px-3 py-2 text-[12px] text-text-3">
              {{ t('acct.account.ssoManaged') }}
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div><label class="label">{{ t('admin.displayName') }}</label><input v-model="displayName" class="input" :placeholder="t('acct.account.namePlaceholder')" :disabled="auth.user?.oidc" /></div>
              <div><label class="label">{{ t('acct.account.email') }}</label><input v-model="email" type="email" class="input" placeholder="you@example.org" :disabled="auth.user?.oidc" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-primary" @click="saveAccount">{{ t('acct.saveChanges') }}</button></div>
          </div>
        </section>

        <!-- Appearance -->
        <section id="appearance" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Palette class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.appearance') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div class="flex items-center justify-between gap-4">
              <div><div class="text-[13px] font-semibold">{{ t('acct.appearance.theme') }}</div><div class="text-[12px] text-text-3">{{ t('acct.appearance.themeHint') }}</div></div>
              <div class="seg">
                <button v-for="t in themes" :key="t.value" class="inline-flex items-center gap-1.5" :class="{ on: prefs.theme === t.value }" @click="prefs.setTheme(t.value)">
                  <component :is="t.icon" class="h-3.5 w-3.5" /> {{ t.label }}
                </button>
              </div>
            </div>
            <div class="border-t border-border pt-5">
              <div class="flex items-center justify-between gap-4">
                <div><div class="text-[13px] font-semibold">{{ t('acct.appearance.accent') }}</div><div class="text-[12px] text-text-3">{{ t('acct.appearance.accentHint') }}</div></div>
                <button v-if="prefs.accentColor" class="text-[12px] text-text-3 hover:text-text" @click="prefs.setAccentColor('')">{{ t('acct.appearance.resetAccent') }}</button>
              </div>
              <div class="mt-3 flex items-center gap-3">
                <input :value="prefs.accentColor || '#3b82f6'" type="color" class="h-9 w-11 shrink-0 rounded-lg border border-border bg-surface-2" @input="prefs.setAccentColor(($event.target as HTMLInputElement).value)" />
                <div class="flex flex-wrap gap-2">
                  <button
                    v-for="sw in accentSwatches"
                    :key="sw"
                    type="button"
                    class="h-6 w-6 rounded-full border-2 transition"
                    :class="prefs.accentColor.toLowerCase() === sw ? 'border-text' : 'border-transparent'"
                    :style="{ backgroundColor: sw }"
                    @click="prefs.setAccentColor(sw)"
                  />
                </div>
              </div>
            </div>
            <div class="flex items-center justify-between gap-4 border-t border-border pt-5">
              <div><div class="text-[13px] font-semibold">{{ t('acct.appearance.remoteImages') }}</div><div class="text-[12px] text-text-3">{{ t('acct.appearance.remoteImagesHint') }}</div></div>
              <div class="seg">
                <button type="button" :class="{ on: prefs.remoteImages === 'ask' }" @click="prefs.setRemoteImages('ask')">{{ t('acct.appearance.remoteAsk') }}</button>
                <button type="button" :class="{ on: prefs.remoteImages === 'always' }" @click="prefs.setRemoteImages('always')">{{ t('acct.appearance.remoteAlways') }}</button>
              </div>
            </div>
            <!-- Own developer theme, when the owner lets each person choose. -->
            <div v-if="branding.devThemesPersonal" class="border-t border-border pt-5">
              <div class="mb-3"><div class="text-[13px] font-semibold">{{ t('acct.appearance.devTheme') }}</div><div class="text-[12px] text-text-3">{{ t('acct.appearance.devThemeHint') }}</div></div>
              <DevThemePicker :model-value="prefs.devTheme" :instance-theme="branding.instanceTheme?.id ?? null" :dark="branding.dark" @update:model-value="prefs.setDevTheme($event)" />
            </div>
          </div>
        </section>

        <!-- Language & time -->
        <section id="language" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Globe class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.languageTime') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div>
              <div class="text-[13px] font-semibold">{{ t('settings.language') }}</div>
              <div class="mb-2 text-[12px] text-text-3">{{ t('settings.languageHint') }}</div>
              <select :value="locale" class="input" @change="changeLocale(($event.target as HTMLSelectElement).value as LocaleCode)">
                <option v-for="l in SUPPORTED_LOCALES" :key="l.code" :value="l.code">{{ l.label }}</option>
              </select>
            </div>
            <div class="border-t border-border pt-5">
              <div class="text-[13px] font-semibold">{{ t('settings.timezone') }}</div>
              <div class="mb-2 text-[12px] text-text-3">{{ t('settings.timezoneHint') }}</div>
              <select :value="prefs.timezone" class="input" @change="prefs.setTimezone(($event.target as HTMLSelectElement).value)">
                <option value="local">{{ t('settings.tzDevice', { tz: deviceTZ }) }}</option>
                <option v-for="tz in explicitZones" :key="tz" :value="tz">{{ tz }}</option>
              </select>
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
              <div><div class="text-[13px] font-semibold">{{ t('settings.weekStart') }}</div><div class="text-[12px] text-text-3">{{ t('settings.weekStartHint') }}</div></div>
              <div class="seg">
                <button :class="{ on: prefs.weekStart === 1 }" @click="prefs.setWeekStart(1)">{{ t('settings.monday') }}</button>
                <button :class="{ on: prefs.weekStart === 0 }" @click="prefs.setWeekStart(0)">{{ t('settings.sunday') }}</button>
              </div>
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
              <div><div class="text-[13px] font-semibold">{{ t('settings.timeFormat') }}</div><div class="text-[12px] text-text-3">{{ t('settings.timeFormatHint') }}</div></div>
              <div class="seg">
                <button :class="{ on: prefs.timeFormat === '24h' }" @click="prefs.setTimeFormat('24h')">13:00</button>
                <button :class="{ on: prefs.timeFormat === '12h' }" @click="prefs.setTimeFormat('12h')">1:00 PM</button>
              </div>
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-5">
              <div><div class="text-[13px] font-semibold">{{ t('acct.pageEditor.label') }}</div><div class="text-[12px] text-text-3">{{ t('acct.pageEditor.hint') }}</div></div>
              <div class="seg">
                <button :class="{ on: prefs.pageEditor === 'visual' }" @click="prefs.setPageEditor('visual')">{{ t('acct.pageEditor.visual') }}</button>
                <button :class="{ on: prefs.pageEditor === 'markdown' }" @click="prefs.setPageEditor('markdown')">{{ t('acct.pageEditor.markdown') }}</button>
              </div>
            </div>
          </div>
        </section>

        <!-- Notifications -->
        <section id="notifications" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Bell class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.notifications') }}</h2></div>
          <div class="p-5"><NotificationSettings /></div>
        </section>

        <!-- Git accounts -->
        <section v-if="auth.features.git_integration !== false" id="git" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><GitBranch class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('git.settings.title') }}</h2></div>
          <div class="p-5"><GitSettings /></div>
        </section>

        <!-- Calendar feeds -->
        <section v-if="auth.features.calendar_feeds !== false" id="feeds" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><CalendarRange class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.feeds') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <p class="text-[12.5px] text-text-3">{{ t('settings.feedsHint') }}</p>
              <button class="btn-primary shrink-0" @click="openFeed"><Plus class="h-4 w-4" />{{ t('settings.newFeed') }}</button>
            </div>
            <div v-if="feeds.length" class="group">
              <div v-for="f in feeds" :key="f.id" class="group-row">
                <CalendarRange class="h-4 w-4 text-text-3" />
                <div class="min-w-0 flex-1">
                  <div class="truncate text-[13.5px] font-medium">{{ f.name }} <span class="font-mono text-[11px] text-text-3">{{ f.prefix }}…</span></div>
                  <div class="truncate text-[12px] text-text-3">{{ feedWsNames(f) }}{{ f.include_tasks ? ' · ' + t('settings.feedWithTasks') : '' }}<span v-if="f.last_used_at"> · {{ t('settings.lastUsed', { when: fmtDate(f.last_used_at) }) }}</span></div>
                </div>
                <button class="icon-btn !text-danger" :title="t('common.revoke')" @click="confirmFeed = f"><Trash2 class="h-4 w-4" /></button>
              </div>
            </div>
          </div>
        </section>

        <!-- Password -->
        <section id="password" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Lock class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.password') }}</h2></div>
          <div v-if="auth.user?.oidc" class="p-5 text-[13px] text-text-2">
            {{ t('acct.password.ssoManaged') }}
          </div>
          <form v-else class="flex flex-col gap-4 p-5" @submit.prevent="changePassword">
            <div>
              <label class="label">{{ t('acct.password.current') }}</label>
              <div class="relative">
                <input v-model="oldPw" :type="showOld ? 'text' : 'password'" autocomplete="current-password" class="input pr-10" :placeholder="t('acct.password.currentPlaceholder')" />
                <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showOld = !showOld"><component :is="showOld ? EyeOff : Eye" class="h-4 w-4" /></button>
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label">{{ t('acct.password.new') }}</label>
                <div class="relative">
                  <input v-model="newPw" :type="showNew ? 'text' : 'password'" autocomplete="new-password" class="input pr-10" :placeholder="t('acct.password.newPlaceholder')" />
                  <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showNew = !showNew"><component :is="showNew ? EyeOff : Eye" class="h-4 w-4" /></button>
                </div>
              </div>
              <div>
                <label class="label">{{ t('acct.password.confirm') }}</label>
                <div class="relative">
                  <input v-model="confirmPw" :type="showConfirm ? 'text' : 'password'" autocomplete="new-password" class="input pr-10" :placeholder="t('acct.password.confirmPlaceholder')" />
                  <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showConfirm = !showConfirm"><component :is="showConfirm ? EyeOff : Eye" class="h-4 w-4" /></button>
                </div>
              </div>
            </div>
            <p v-if="pwErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ pwErr }}</p>
            <p v-if="pwMsg" class="rounded-lg bg-ok-bg px-3 py-2 text-[12px] text-ok-text">{{ pwMsg }}</p>
            <div class="flex justify-end"><button type="submit" class="btn-primary">{{ t('acct.password.update') }}</button></div>
          </form>
        </section>

        <!-- Two-factor -->
        <section v-if="!auth.user?.oidc" id="twofa" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><ShieldCheck class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.twofaFull') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">{{ t('acct.otp.app') }}</div>
                <div class="text-[12px] text-text-3">{{ t('acct.otp.appHint') }}</div>
              </div>
              <span
                class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                :class="auth.user?.otp_enabled ? 'bg-ok-bg text-ok-text' : 'bg-surface-3 text-text-2'"
              >
                {{ auth.user?.otp_enabled ? t('acct.enabled') : t('acct.disabled') }}
              </span>
            </div>

            <div v-if="!auth.user?.otp_enabled">
              <button class="btn-primary" @click="showOtpSetup = true">{{ t('acct.otp.setUp') }}</button>
            </div>
            <template v-else>
              <!-- Recovery codes -->
              <div class="flex items-center justify-between border-t border-border pt-4">
                <div>
                  <div class="text-[13px] font-semibold">{{ t('acct.otp.recoveryCodes') }}</div>
                  <div class="text-[12px] text-text-3">{{ t('acct.otp.remaining', auth.otpBackupRemaining) }}</div>
                </div>
                <button class="btn-ghost" @click="openRegen">{{ t('acct.otp.regenerate') }}</button>
              </div>
              <!-- Disable -->
              <div class="border-t border-border pt-4">
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <div class="text-[13px] font-semibold">{{ t('acct.otp.disableTitle') }}</div>
                    <div class="text-[12px] text-text-3">{{ t('acct.otp.disableHint') }}</div>
                  </div>
                  <button class="btn-danger" @click="openDisable">{{ t('acct.otp.disable') }}</button>
                </div>
              </div>
            </template>
          </div>
        </section>

        <!-- Passkeys -->
        <section v-if="passkeysAvailable" id="passkeys" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><KeyRound class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.passkeys') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">{{ t('settings.passkeys') }}</div>
                <div class="text-[12px] text-text-3">{{ t('acct.passkeys.hint') }}</div>
              </div>
              <button class="btn-primary shrink-0" @click="openAddPasskey">{{ t('acct.passkeys.add') }}</button>
            </div>

            <div v-if="passkeys.length" class="flex flex-col divide-y divide-border rounded-xl border border-border">
              <div v-for="pk in passkeys" :key="pk.id" class="flex items-center justify-between gap-3 px-4 py-3">
                <div class="flex items-center gap-3">
                  <Fingerprint class="h-4 w-4 text-text-3" />
                  <div>
                    <div class="flex items-center gap-2 text-[13px] font-medium">
                      {{ pk.name }}
                      <span v-if="pk.backed_up" class="rounded-full bg-ok-bg px-1.5 py-0.5 text-[10px] font-medium text-ok-text">{{ t('acct.passkeys.synced') }}</span>
                    </div>
                    <div class="text-[11px] text-text-3">
                      {{ t('acct.passkeys.addedOn', { when: fmtDate(pk.created_at) }) }}<span v-if="pk.last_used_at"> · {{ t('acct.passkeys.lastUsed', { when: fmtDate(pk.last_used_at) }) }}</span>
                    </div>
                  </div>
                </div>
                <button class="rounded-lg p-1.5 text-text-3 transition hover:bg-crit-bg hover:text-crit-text" :title="t('ws.remove')" @click="confirmPk = pk">
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>
            <p v-else class="text-[12px] text-text-3">{{ t('acct.passkeys.none') }}</p>
          </div>
        </section>

        <!-- API tokens -->
        <section id="apitokens" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Terminal class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.apitokens') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">{{ t('acct.tokens.personal') }}</div>
                <I18nT scope="global" keypath="acct.tokens.hint" tag="div" class="text-[12px] text-text-3">
                  <template #code><code>Authorization: Bearer &lt;token&gt;</code></template>
                  <template #link><a href="/api/docs" target="_blank" rel="noopener" class="text-accent hover:underline">{{ t('nav.apiDocs') }}</a></template>
                </I18nT>
              </div>
              <button class="btn-primary shrink-0" @click="openCreateToken">{{ t('acct.tokens.new') }}</button>
            </div>

            <div v-if="tokens.length" class="flex flex-col divide-y divide-border rounded-xl border border-border">
              <div v-for="tk in tokens" :key="tk.id" class="flex items-center justify-between gap-3 px-4 py-3">
                <div>
                  <div class="text-[13px] font-medium">{{ tk.name }} <span class="font-mono text-[11px] text-text-3">{{ tk.prefix }}…</span></div>
                  <div v-if="tk.privileges?.length || tk.read_only" class="mt-0.5 flex flex-wrap items-center gap-1 text-[11px] text-text-2">
                    <span class="cap">{{ t('settings.tokenLimited') }}</span>
                    <span v-if="tk.read_only" class="pill !text-[11px]">{{ t('settings.tokenReadOnly') }}</span>
                    <span v-for="p in tk.privileges ?? []" :key="p" class="pill !text-[11px]">{{ privLabel(p) }}</span>
                  </div>
                  <div class="text-[11px] text-text-3">
                    {{ t('acct.tokens.addedOn', { when: fmtDate(tk.created_at) }) }}<span v-if="tk.last_used_at"> · {{ t('acct.tokens.lastUsed', { when: fmtDate(tk.last_used_at) }) }}</span><span v-if="tk.expires_at"> · {{ t('acct.tokens.expiresOn', { when: fmtDate(tk.expires_at) }) }}</span><span v-else> · {{ t('acct.tokens.neverExpires') }}</span>
                  </div>
                </div>
                <button class="rounded-lg p-1.5 text-text-3 transition hover:bg-crit-bg hover:text-crit-text" :title="t('common.revoke')" @click="confirmTk = tk">
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>
            <p v-else class="text-[12px] text-text-3">{{ t('acct.tokens.none') }}</p>
          </div>
        </section>

        <!-- Assistants (MCP) -->
        <section id="mcp" class="card overflow-hidden scroll-mt-4" data-testid="mcp-card">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Bot class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('mcp.title') }}</h2></div>
          <div class="flex flex-col gap-3 p-5 text-[13px]">
            <p class="text-text-2">{{ t('mcp.lead') }}</p>
            <div class="flex items-center gap-2 rounded-lg bg-fill-2 px-3 py-2">
              <code class="min-w-0 flex-1 truncate font-mono text-[12.5px]" data-testid="mcp-url">{{ mcpUrl }}</code>
              <CopyButton :value="mcpUrl" />
            </div>
            <ul class="flex list-disc flex-col gap-1.5 pl-5 text-[12.5px] text-text-2">
              <li>{{ t('mcp.claude') }}</li>
              <li>{{ t('mcp.chatgpt') }}</li>
              <li><span>{{ t('mcp.code') }}</span> <code class="font-mono text-[12px]">claude mcp add --transport http orbit {{ mcpUrl }}</code></li>
              <li>{{ t('mcp.cursor') }}</li>
            </ul>
            <p class="text-[12px] text-text-3">{{ t('mcp.how') }}</p>
          </div>
        </section>

        <!-- Active sessions (feature-flagged) -->
        <section v-if="auth.features.sessions" id="sessions" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center justify-between border-b border-border px-5 py-3">
            <div class="flex items-center gap-2">
              <MonitorSmartphone class="h-[18px] w-[18px] text-text-2" />
              <h2 class="text-[14px] font-semibold">{{ t('settings.sessions') }}</h2>
            </div>
            <span class="rounded-full bg-ok-bg px-2.5 py-0.5 text-[11px] font-semibold text-ok-text">{{ t('acct.sessions.active', sessions.length) }}</span>
          </div>
          <div>
            <div
              v-for="s in sessions"
              :key="s.id"
              class="flex items-center justify-between gap-4 border-b border-border px-5 py-4 last:border-0"
            >
              <div class="flex min-w-0 items-center gap-3">
                <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-2 text-text-2">
                  <component :is="sessionIcon(s.user_agent)" class="h-[18px] w-[18px]" />
                </span>
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="truncate text-[13.5px] font-semibold">{{ sessionDevice(s.user_agent) }}</span>
                    <span v-if="s.current" class="shrink-0 rounded-full bg-ok-bg px-2 py-0.5 text-[11px] font-semibold text-ok-text">{{ t('acct.sessions.current') }}</span>
                  </div>
                  <div class="truncate text-[12px] text-text-3">{{ s.ip || t('acct.sessions.unknownIp') }} · {{ sessionRel(s) }}</div>
                </div>
              </div>
              <button v-if="!s.current" class="btn-ghost shrink-0" :disabled="sessionsBusy" @click="revokeSession(s.id)">{{ t('common.revoke') }}</button>
            </div>
            <div v-if="sessions.length === 0" class="px-5 py-6 text-center text-[13px] text-text-3">{{ t('acct.sessions.none') }}</div>
            <div v-if="sessions.length > 1" class="px-5 py-4">
              <button class="btn-ghost w-full justify-center" :disabled="sessionsBusy" @click="revokeOtherSessions">
                <LogOut class="h-4 w-4" /> {{ t('acct.sessions.signOutOthers') }}
              </button>
            </div>
          </div>
        </section>
      </div>
    </div>

    <!-- Create-token modal -->
    <Overlay :open="showTokenCreate" :title="t('acct.tokens.newTitle')" mode="centered" @close="showTokenCreate = false">
      <div v-if="createdToken" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">{{ t('acct.tokens.copyNow') }}</p>
        <div class="flex items-center gap-2 rounded-xl border border-border bg-surface-2 p-3">
          <code class="flex-1 break-all font-mono text-[12px] select-all">{{ createdToken }}</code>
          <button class="rounded-lg p-1.5 text-text-3 hover:text-text" :title="t('settings.copy')" @click="copyToken"><Copy class="h-4 w-4" /></button>
        </div>
        <div class="flex justify-end"><button class="btn-primary" @click="showTokenCreate = false">{{ t('common.done') }}</button></div>
      </div>
      <form v-else class="flex flex-col gap-3" @submit.prevent="createToken">
        <div><label class="label">{{ t('common.name') }}</label><input v-model="tkName" class="input" :placeholder="t('acct.tokens.namePlaceholder')" maxlength="128" autofocus /></div>
        <div>
          <label class="label">{{ t('admin.colExpires') }}</label>
          <select v-model.number="tkExpiry" class="input">
            <option :value="0">{{ t('acct.tokens.never') }}</option>
            <option :value="30">{{ t('acct.tokens.days', 30) }}</option>
            <option :value="90">{{ t('acct.tokens.days', 90) }}</option>
            <option :value="365">{{ t('acct.tokens.oneYear') }}</option>
          </select>
        </div>
        <Checkbox v-model="tkReadOnly" class="text-text-2">{{ t('settings.tokenReadOnlyLong') }}</Checkbox>
        <Checkbox v-model="tkLimit" class="text-text-2">{{ t('settings.tokenLimit') }}</Checkbox>
        <template v-if="tkLimit">
          <div>
            <label class="label">{{ t('settings.tokenPrivs') }}</label>
            <div class="grid grid-cols-2 gap-x-4 gap-y-1.5">
              <Checkbox
                v-for="p in tokenPrivChoices"
                :key="p.id"
                :model-value="tkPrivs.includes(p.id)"
                class="text-[12.5px]"
                @update:model-value="(v) => toggleTkPriv(p.id, v)"
              >{{ p.label }}</Checkbox>
            </div>
          </div>
        </template>
        <p v-if="tkErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ tkErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showTokenCreate = false">{{ t('common.cancel') }}</button>
          <button type="submit" :disabled="tkBusy" class="btn-primary">{{ tkBusy ? t('acct.tokens.creating') : t('acct.tokens.create') }}</button>
        </div>
      </form>
    </Overlay>

    <!-- New calendar feed -->
    <Overlay :open="feedSheet" :title="t('settings.newFeed')" @close="feedSheet = false">
      <div v-if="feedResult" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">{{ t('settings.feedOnce') }}</p>
        <div class="flex items-center gap-2 rounded-xl bg-fill-2 p-3">
          <code class="flex-1 break-all font-mono text-[12px] select-all">{{ feedResult.url }}</code>
          <button class="icon-btn" :title="t('settings.copy')" @click="copyText(feedResult.url)"><Copy class="h-4 w-4" /></button>
        </div>
        <p class="text-[12px] text-text-3">{{ t('settings.feedHow') }}</p>
        <div class="flex justify-end gap-2">
          <a :href="feedResult.webcal" class="btn"><ExternalLink class="h-4 w-4" />{{ t('settings.feedOpen') }}</a>
          <button class="btn-primary" @click="feedSheet = false">{{ t('common.done') }}</button>
        </div>
      </div>
      <form v-else class="flex flex-col gap-4" @submit.prevent="createFeed">
        <div><label class="label">{{ t('common.name') }}</label><input v-model="feedName" class="input" maxlength="120" /></div>
        <div>
          <Checkbox v-model="feedAll">{{ t('settings.feedAllWs') }}</Checkbox>
          <div v-if="!feedAll" class="mt-2 grid grid-cols-2 gap-2 pl-6">
            <Checkbox v-for="w in wss.active" :key="w.id" :model-value="feedWs.includes(w.id)" @update:model-value="(v) => (feedWs = v ? [...feedWs, w.id] : feedWs.filter((x) => x !== w.id))">{{ w.name }}</Checkbox>
          </div>
        </div>
        <Checkbox v-model="feedTasks">{{ t('settings.feedIncludeTasks') }}</Checkbox>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn" @click="feedSheet = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary" :disabled="!feedAll && !feedWs.length">{{ t('common.create') }}</button>
        </div>
      </form>
    </Overlay>
    <Confirm
      :open="!!confirmFeed"
      :title="t('settings.revokeFeedTitle')"
      :message="t('settings.revokeFeedBody', { name: confirmFeed?.name })"
      :confirm-label="t('common.revoke')"
      danger
      @confirm="revokeFeed"
      @close="confirmFeed = null"
    />

    <Confirm
      :open="!!confirmTk"
      :title="t('acct.tokens.revokeTitle')"
      :message="t('acct.tokens.revokeBody', { name: confirmTk?.name })"
      :confirm-label="t('common.revoke')"
      danger
      @confirm="revokeToken"
      @close="confirmTk = null"
    />

    <!-- Add-passkey modal -->
    <Overlay :open="showPkName" :title="t('acct.passkeys.add')" mode="centered" @close="showPkName = false">
      <form class="flex flex-col gap-3" @submit.prevent="createPasskey">
        <p class="text-[13px] text-text-2">{{ t('acct.passkeys.nameHint') }}</p>
        <input v-model="pkName" class="input" :placeholder="t('acct.passkeys.namePlaceholder')" maxlength="128" autofocus />
        <p v-if="pkErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ pkErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showPkName = false">{{ t('common.cancel') }}</button>
          <button type="submit" :disabled="pkBusy" class="btn-primary">{{ pkBusy ? t('acct.passkeys.waiting') : t('acct.passkeys.create') }}</button>
        </div>
      </form>
    </Overlay>

    <Confirm
      :open="!!confirmPk"
      :title="t('acct.passkeys.removeTitle')"
      :message="t('acct.passkeys.removeBody', { name: confirmPk?.name })"
      :confirm-label="t('ws.remove')"
      danger
      @confirm="removePasskey"
      @close="confirmPk = null"
    />

    <!-- OTP setup modal -->
    <Overlay :open="showOtpSetup" :title="t('acct.otp.setUp')" mode="centered" @close="showOtpSetup = false">
      <OtpSetup v-if="showOtpSetup" @verified="onOtpVerified" />
    </Overlay>

    <!-- Regenerate recovery codes modal -->
    <Overlay :open="showRegen" :title="t('acct.otp.recoveryCodes')" mode="centered" @close="showRegen = false">
      <div v-if="regenCodes.length" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
          {{ t('acct.otp.oldInvalid') }}
        </p>
        <div class="grid grid-cols-2 gap-2 rounded-xl border border-border bg-surface-2 p-3 font-mono">
          <span v-for="c in regenCodes" :key="c" class="select-all">{{ c }}</span>
        </div>
        <div class="flex justify-end"><button class="btn-primary" @click="showRegen = false">{{ t('common.done') }}</button></div>
      </div>
      <form v-else class="flex flex-col gap-3" @submit.prevent="regenerate">
        <p class="text-center text-[13px] text-text-2">{{ t('acct.otp.regenPrompt') }}</p>
        <div class="flex justify-center"><CodeInput v-model="regenCode" @complete="regenerate" /></div>
        <p v-if="regenErr" class="rounded-lg bg-crit-bg px-3 py-2 text-center text-[12px] text-crit-text">{{ regenErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showRegen = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary" :disabled="regenCode.length < 6">{{ t('acct.otp.generate') }}</button>
        </div>
      </form>
    </Overlay>

    <!-- Turn off two-factor: the same code prompt -->
    <Overlay :open="showDisable" :title="t('acct.otp.disableTitle')" mode="centered" @close="showDisable = false">
      <form class="flex flex-col gap-3" @submit.prevent="disableOtp">
        <p class="text-center text-[13px] text-text-2">{{ t('acct.otp.disablePrompt') }}</p>
        <div class="flex justify-center"><CodeInput v-model="disableCode" @complete="disableOtp" /></div>
        <p v-if="otpErr" class="rounded-lg bg-crit-bg px-3 py-2 text-center text-[12px] text-crit-text">{{ otpErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showDisable = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-danger" :disabled="disableCode.length < 6">{{ t('acct.otp.disable') }}</button>
        </div>
      </form>
    </Overlay>
  </div>
</template>

<style scoped>
.settings-page {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
}
/* The cards stack; on wide screens they flow into two balanced columns, so
   the page uses the width it has. */
.sections {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
@media (min-width: 1500px) {
  .sections {
    display: block;
    columns: 2;
    column-gap: 20px;
  }
  .sections > * {
    break-inside: avoid;
    margin-bottom: 20px;
  }
}
</style>

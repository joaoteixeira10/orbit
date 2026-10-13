<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { setLocale, currentLocale, SUPPORTED_LOCALES, type LocaleCode } from '@/i18n'
import { usePrefsStore, type Theme, type ChangeDetails } from '@/stores/prefs'
import { useProxiesStore } from '@/stores/proxies'
import { api, setAccessToken } from '@/lib/api'
import { meApi, prefsApi, notifyApi, sessionsApi, type SessionInfo } from '@/api'
import { passkeysApi, webauthnSupported } from '@/api/webauthn'
import type { WebAuthnCredential } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { fmtDate, parseUA, relTime } from '@/lib/format'
import Toggle from '@/components/ui/Toggle.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import OtpSetup from '@/components/ui/OtpSetup.vue'
import CodeInput from '@/components/ui/CodeInput.vue'
import { User, Palette, Globe, Lock, Bell, ShieldCheck, KeyRound, Fingerprint, Trash2, Terminal, Copy, Sun, Moon, Monitor, Eye, EyeOff, Upload, MonitorSmartphone, Smartphone, Laptop, LogOut } from 'lucide-vue-next'
import type { APIToken } from '@/api/types'
import { GLOBAL_PRIVILEGES, SCOPED_PRIVILEGES, isOwnerOnlyPriv } from '@/lib/privileges'

const prefs = usePrefsStore()
// Change confirmations: the user's choice, unless an admin enforces one.
const px = useProxiesStore()
const detailPolicy = computed(() => px.data?.ui?.change_details_policy || '')
const detailChoice = computed<ChangeDetails>(() => detailPolicy.value || prefs.changeDetails)
const detailOptions = computed<{ value: ChangeDetails; label: string; hint: string }[]>(() => [
  { value: 'detailed', label: t('settings.cdDetailed'), hint: t('settings.cdDetailedHint') },
  { value: 'compact', label: t('settings.cdCompact'), hint: t('settings.cdCompactHint') },
])
const auth = useAuthStore()
const { t } = useI18n()

// ---- Language ----
const locale = ref<LocaleCode>(currentLocale())
function changeLocale(code: LocaleCode) {
  locale.value = code
  setLocale(code)
}
const toast = useToastStore()

const sections = computed(() => [
  { group: 'Personal', items: [
    { id: 'account', label: t('settings.account'), icon: User },
    { id: 'appearance', label: t('settings.appearance'), icon: Palette },
    { id: 'language', label: t('settings.languageTime'), icon: Globe },
    { id: 'notifications', label: t('settings.notifications'), icon: Bell },
  ] },
  { group: 'Security', items: [
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

// ---- Account ----
const displayName = ref(auth.user?.display_name ?? '')
const email = ref(auth.user?.email ?? '')
const avatarUrl = ref(auth.user?.avatar_url ?? '')
const avatarBroken = ref(false)
function initials(name?: string) {
  return (name || auth.user?.username || '?').slice(0, 2).toUpperCase()
}
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
    toast.success('Account updated')
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
const accentSwatches = ['#07cab3', '#185fa5', '#7c3aed', '#16a34a', '#d97706', '#dc2626', '#db2777']

// ---- Language & time ----
const deviceTZ = Intl.DateTimeFormat().resolvedOptions().timeZone
const explicitZones = [
  'UTC', deviceTZ, 'Europe/Lisbon', 'Europe/London', 'Europe/Madrid',
  'Europe/Berlin', 'America/New_York', 'America/Sao_Paulo', 'Asia/Tokyo',
].filter((v, i, a) => a.indexOf(v) === i)

// ---- Notifications ----
// Personal server alerts go by email; the shared channels (Discord, Teams, …)
// carry the organisation's server events, set by the owner.
const enabledChannels = ref<string[]>([])
const notifyEvents = ref(false)
const emailOn = computed(() => enabledChannels.value.includes('email'))
async function saveNotifications() {
  try {
    await prefsApi.update({ notify_events: notifyEvents.value, notify_channels: notifyEvents.value ? ['email'] : [] })
    toast.success('Notification preferences saved')
  } catch (e: any) {
    toast.error(e.message)
  }
}

onMounted(async () => {
  try {
    const [chans, p] = await Promise.all([notifyApi.channels(), prefsApi.get()])
    const enabled = chans.enabled ?? []
    enabledChannels.value = enabled
    notifyEvents.value = p.notify_events && (p.notify_channels || []).includes('email')
  } catch {
    /* ignore */
  }
})

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
    pwErr.value = 'New password and confirmation do not match.'
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
    pwMsg.value = 'Password updated. Your other sessions were signed out.'
    oldPw.value = ''
    newPw.value = ''
    confirmPw.value = ''
  } catch (e: any) {
    pwErr.value = e?.message || 'Could not change password'
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
    regenErr.value = e?.message || 'Invalid code'
    regenCode.value = ''
  }
}
async function onOtpVerified() {
  showOtpSetup.value = false
  await auth.fetchMe()
  toast.success('Two-factor authentication enabled')
}
async function disableOtp() {
  if (disableCode.value.length < 6) return
  otpErr.value = ''
  try {
    await meApi.otpDisable(disableCode.value.trim())
    disableCode.value = ''
    showDisable.value = false
    await auth.fetchMe()
    toast.success('Two-factor disabled')
  } catch (e: any) {
    otpErr.value = e?.message || 'Could not disable'
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
    toast.success('Passkey added')
  } catch (e: any) {
    pkErr.value = e?.name === 'NotAllowedError' ? 'Registration was cancelled' : e?.message || 'Could not add passkey'
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
    toast.success('Passkey removed')
  } catch (e: any) {
    toast.error(e?.message || 'Could not remove passkey')
  }
}
onMounted(loadPasskeys)

// ---- API tokens ----
const tokens = ref<APIToken[]>([])
const showTokenCreate = ref(false)
const tkName = ref('')
const tkExpiry = ref(0) // days; 0 = never
const tkBusy = ref(false)
const tkErr = ref('')
const createdToken = ref('') // shown once
// Optional narrowing: fewer privileges and/or scope rules (one per line).
const tkLimit = ref(false)
const tkPrivs = ref<string[]>([])
const tkRules = ref('')
// Only what you hold can be given to a token (owner status never is).
const tokenPrivChoices = computed(() => [
  ...GLOBAL_PRIVILEGES.filter((p) => (isOwnerOnlyPriv(p.id) ? auth.privileges.global.includes(p.id) : auth.can(p.id))),
  ...SCOPED_PRIVILEGES.filter((p) => auth.canAnywhere(p.id)),
])
const privLabel = (id: string) => [...GLOBAL_PRIVILEGES, ...SCOPED_PRIVILEGES].find((p) => p.id === id)?.label ?? id
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
  tkRules.value = ''
  createdToken.value = ''
  showTokenCreate.value = true
}
async function createToken() {
  tkErr.value = ''
  tkBusy.value = true
  try {
    const rules = tkLimit.value ? tkRules.value.split('\n').map((l) => l.trim()).filter(Boolean) : []
    const privs = tkLimit.value ? tkPrivs.value : []
    if (tkLimit.value && !privs.length && !rules.length) {
      tkErr.value = t('settings.tokenLimitEmpty')
      return
    }
    const r = await meApi.createApiToken(tkName.value.trim() || 'token', tkExpiry.value, privs, rules)
    createdToken.value = r.token
    await loadTokens()
  } catch (e: any) {
    tkErr.value = e?.message || 'Could not create token'
  } finally {
    tkBusy.value = false
  }
}
function copyToken() {
  navigator.clipboard.writeText(createdToken.value)
  toast.success('Token copied')
}
async function revokeToken() {
  const t = confirmTk.value
  confirmTk.value = null
  if (!t) return
  try {
    await meApi.revokeApiToken(t.id)
    await loadTokens()
    toast.success('Token revoked')
  } catch (e: any) {
    toast.error(e?.message || 'Could not revoke token')
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
const sessionRel = (s: SessionInfo) => (s.current ? 'Active now' : relTime(s.last_used_at))
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
    toast.success('Session revoked')
  } catch (e: any) {
    toast.error(e?.message || 'Could not revoke session')
  } finally {
    sessionsBusy.value = false
  }
}
async function revokeOtherSessions() {
  sessionsBusy.value = true
  try {
    await sessionsApi.revokeOthers()
    await loadSessions()
    toast.success('Signed out all other sessions')
  } catch (e: any) {
    toast.error(e?.message || 'Could not sign out other sessions')
  } finally {
    sessionsBusy.value = false
  }
}
onMounted(loadSessions)
</script>

<template>
  <div>
    <header class="mb-6">
      <h1 class="text-xl font-semibold tracking-tight">Settings</h1>
      <p class="mt-1 text-[13px] text-text-2">Your personal preferences. These follow you across devices.</p>
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

      <div class="flex min-w-0 flex-1 flex-col gap-5">
        <!-- Account -->
        <section id="account" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><User class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.account') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div class="flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <img v-if="avatarUrl && !avatarBroken" :src="avatarUrl" class="h-12 w-12 rounded-full object-cover" @error="avatarBroken = true" />
                <div v-else class="flex h-12 w-12 items-center justify-center rounded-full bg-accent-bg text-[15px] font-semibold text-accent-text">{{ initials(displayName) }}</div>
                <div>
                  <div class="text-[14px] font-semibold">{{ displayName || auth.user?.username }}</div>
                  <div class="text-[12px] text-text-3">{{ auth.user?.username }}</div>
                </div>
              </div>
              <label class="btn-ghost cursor-pointer">
                <Upload class="h-4 w-4" /> Upload photo
                <input type="file" accept="image/*" class="hidden" @change="onAvatar" />
              </label>
            </div>
            <div v-if="auth.user?.oidc" class="rounded-xl border border-border bg-surface-2 px-3 py-2 text-[12px] text-text-3">
              Your name and email are managed by your identity provider (SSO) and update on each sign-in.
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div><label class="label">Display name</label><input v-model="displayName" class="input" placeholder="Your name" :disabled="auth.user?.oidc" /></div>
              <div><label class="label">Email address</label><input v-model="email" type="email" class="input" placeholder="you@example.org" :disabled="auth.user?.oidc" /></div>
            </div>
            <div class="flex justify-end"><button class="btn-primary" @click="saveAccount">Save changes</button></div>
          </div>
        </section>

        <!-- Appearance -->
        <section id="appearance" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Palette class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.appearance') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div class="flex items-center justify-between gap-4">
              <div><div class="text-[13px] font-semibold">Theme</div><div class="text-[12px] text-text-3">Choose how the interface looks</div></div>
              <div class="flex gap-1.5">
                <button v-for="t in themes" :key="t.value" class="flex items-center gap-1.5 rounded-xl border px-3 py-2 text-[13px] transition" :class="prefs.theme === t.value ? 'border-accent bg-accent-bg text-accent-text' : 'border-border bg-surface-2 text-text-2 hover:text-text'" @click="prefs.setTheme(t.value)">
                  <component :is="t.icon" class="h-4 w-4" /> {{ t.label }}
                </button>
              </div>
            </div>
            <div class="border-t border-border pt-5">
              <div class="flex items-center justify-between gap-4">
                <div><div class="text-[13px] font-semibold">Accent colour</div><div class="text-[12px] text-text-3">A personal accent just for you — overrides the organization accent</div></div>
                <button v-if="prefs.accentColor" class="text-[12px] text-text-3 hover:text-text" @click="prefs.setAccentColor('')">Reset to default</button>
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
            <div class="border-t border-border pt-5">
              <div class="text-[13px] font-semibold">{{ t('settings.changeDetails') }}</div>
              <div class="mb-3 text-[12px] text-text-3">
                {{ detailPolicy ? t('settings.cdEnforced') : t('settings.changeDetailsHint') }}
              </div>
              <div class="grid gap-2 sm:grid-cols-2">
                <button
                  v-for="o in detailOptions"
                  :key="o.value"
                  type="button"
                  class="rounded-xl border px-3 py-2.5 text-left transition disabled:cursor-not-allowed"
                  :class="detailChoice === o.value ? 'border-accent bg-accent-bg' : 'border-border bg-surface-2 hover:border-border-strong'"
                  :disabled="!!detailPolicy"
                  :aria-pressed="detailChoice === o.value"
                  @click="prefs.setChangeDetails(o.value)"
                >
                  <div class="text-[13px] font-medium" :class="detailChoice === o.value ? 'text-accent-text' : ''">{{ o.label }}</div>
                  <div class="text-[12px] text-text-3">{{ o.hint }}</div>
                </button>
              </div>
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
                <option value="local">Device ({{ deviceTZ }})</option>
                <option v-for="tz in explicitZones" :key="tz" :value="tz">{{ tz }}</option>
              </select>
            </div>
          </div>
        </section>

        <!-- Notifications -->
        <section id="notifications" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Bell class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.notifications') }}</h2></div>
          <div class="flex flex-col gap-5 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">Server alerts by email</div>
                <div class="text-[12px] text-text-3">For the servers in your scope: down and back up, agents unreachable, changes by others, a timed maintenance ending in 15 minutes, drift and failures.</div>
              </div>
              <Toggle v-model="notifyEvents" :disabled="!emailOn" />
            </div>
            <div class="rounded-xl border border-border bg-surface-2 p-3 text-[12px] text-text-3">
              <template v-if="!emailOn">Email is not set up. Ask the owner to turn it on (Owner → Notifications).</template>
              <template v-else-if="!auth.user?.email">Sent to your email address — set one under Account above.</template>
              <template v-else>Sent to <b class="text-text">{{ auth.user.email }}</b>.</template>
              The team channels (Discord, Teams, Slack, …) get the organisation's server events, set by the owner.
            </div>
            <div class="flex justify-end"><button class="btn-primary" @click="saveNotifications">Save preferences</button></div>
          </div>
        </section>

        <!-- Password -->
        <section id="password" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Lock class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.password') }}</h2></div>
          <div v-if="auth.user?.oidc" class="p-5 text-[13px] text-text-2">
            You sign in via SSO; your password is managed by your identity provider.
          </div>
          <form v-else class="flex flex-col gap-4 p-5" @submit.prevent="changePassword">
            <div>
              <label class="label">Current password</label>
              <div class="relative">
                <input v-model="oldPw" :type="showOld ? 'text' : 'password'" autocomplete="current-password" class="input pr-10" placeholder="Enter current password" />
                <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showOld = !showOld"><component :is="showOld ? EyeOff : Eye" class="h-4 w-4" /></button>
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label">New password</label>
                <div class="relative">
                  <input v-model="newPw" :type="showNew ? 'text' : 'password'" autocomplete="new-password" class="input pr-10" placeholder="Min 8 characters" />
                  <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showNew = !showNew"><component :is="showNew ? EyeOff : Eye" class="h-4 w-4" /></button>
                </div>
              </div>
              <div>
                <label class="label">Confirm new password</label>
                <div class="relative">
                  <input v-model="confirmPw" :type="showConfirm ? 'text' : 'password'" autocomplete="new-password" class="input pr-10" placeholder="Repeat password" />
                  <button type="button" class="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-3 hover:text-text" @click="showConfirm = !showConfirm"><component :is="showConfirm ? EyeOff : Eye" class="h-4 w-4" /></button>
                </div>
              </div>
            </div>
            <p v-if="pwErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ pwErr }}</p>
            <p v-if="pwMsg" class="rounded-lg bg-ok-bg px-3 py-2 text-[12px] text-ok-text">{{ pwMsg }}</p>
            <div class="flex justify-end"><button type="submit" class="btn-primary">Update password</button></div>
          </form>
        </section>

        <!-- Two-factor -->
        <section v-if="!auth.user?.oidc" id="twofa" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><ShieldCheck class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.twofaFull') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">Authenticator app (TOTP)</div>
                <div class="text-[12px] text-text-3">Require a 6-digit code from an authenticator app at sign-in.</div>
              </div>
              <span
                class="rounded-full px-2 py-0.5 text-[11px] font-medium"
                :class="auth.user?.otp_enabled ? 'bg-ok-bg text-ok-text' : 'bg-surface-3 text-text-2'"
              >
                {{ auth.user?.otp_enabled ? 'Enabled' : 'Disabled' }}
              </span>
            </div>

            <div v-if="!auth.user?.otp_enabled">
              <button class="btn-primary" @click="showOtpSetup = true">Set up two-factor</button>
            </div>
            <template v-else>
              <!-- Recovery codes -->
              <div class="flex items-center justify-between border-t border-border pt-4">
                <div>
                  <div class="text-[13px] font-semibold">Recovery codes</div>
                  <div class="text-[12px] text-text-3">{{ auth.otpBackupRemaining }} unused code(s) remaining</div>
                </div>
                <button class="btn-ghost" @click="openRegen">Regenerate</button>
              </div>
              <!-- Disable -->
              <div class="border-t border-border pt-4">
                <div class="flex items-center justify-between gap-4">
                  <div>
                    <div class="text-[13px] font-semibold">Disable two-factor</div>
                    <div class="text-[12px] text-text-3">Enter a current code to confirm</div>
                  </div>
                  <button class="btn-danger" @click="openDisable">Disable</button>
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
                <div class="text-[13px] font-semibold">Passkeys</div>
                <div class="text-[12px] text-text-3">Sign in with Touch ID, a security key, or a password manager — no code to type. A passkey counts as two-factor.</div>
              </div>
              <button class="btn-primary shrink-0" @click="openAddPasskey">Add a passkey</button>
            </div>

            <div v-if="passkeys.length" class="flex flex-col divide-y divide-border rounded-xl border border-border">
              <div v-for="pk in passkeys" :key="pk.id" class="flex items-center justify-between gap-3 px-4 py-3">
                <div class="flex items-center gap-3">
                  <Fingerprint class="h-4 w-4 text-text-3" />
                  <div>
                    <div class="flex items-center gap-2 text-[13px] font-medium">
                      {{ pk.name }}
                      <span v-if="pk.backed_up" class="rounded-full bg-ok-bg px-1.5 py-0.5 text-[10px] font-medium text-ok-text">Synced</span>
                    </div>
                    <div class="text-[11px] text-text-3">
                      Added {{ fmtDate(pk.created_at) }}<span v-if="pk.last_used_at"> · last used {{ fmtDate(pk.last_used_at) }}</span>
                    </div>
                  </div>
                </div>
                <button class="rounded-lg p-1.5 text-text-3 transition hover:bg-crit-bg hover:text-crit-text" title="Remove" @click="confirmPk = pk">
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>
            <p v-else class="text-[12px] text-text-3">No passkeys yet.</p>
          </div>
        </section>

        <!-- API tokens -->
        <section id="apitokens" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Terminal class="h-[18px] w-[18px] text-text-2" /><h2 class="text-[14px] font-semibold">{{ t('settings.apitokens') }}</h2></div>
          <div class="flex flex-col gap-4 p-5">
            <div class="flex items-center justify-between gap-4">
              <div>
                <div class="text-[13px] font-semibold">Personal access tokens</div>
                <div class="text-[12px] text-text-3">For scripts and automation. A token carries your own permissions; use <code>Authorization: Bearer &lt;token&gt;</code>. Every endpoint is in the <a href="/api/docs" target="_blank" rel="noopener" class="text-accent hover:underline">API reference</a>.</div>
              </div>
              <button class="btn-primary shrink-0" @click="openCreateToken">New token</button>
            </div>

            <div v-if="tokens.length" class="flex flex-col divide-y divide-border rounded-xl border border-border">
              <div v-for="tk in tokens" :key="tk.id" class="flex items-center justify-between gap-3 px-4 py-3">
                <div>
                  <div class="text-[13px] font-medium">{{ tk.name }} <span class="font-mono text-[11px] text-text-3">{{ tk.prefix }}…</span></div>
                  <div v-if="tk.privileges?.length || tk.rules?.length" class="mt-0.5 flex flex-wrap items-center gap-1 text-[11px] text-text-2">
                    <span class="cap">{{ t('settings.tokenLimited') }}</span>
                    <span v-for="p in tk.privileges ?? []" :key="p" class="rounded bg-surface-3 px-1.5">{{ privLabel(p) }}</span>
                    <code v-for="r in tk.rules ?? []" :key="r" class="rounded border border-border bg-surface-2 px-1.5">{{ r }}</code>
                  </div>
                  <div class="text-[11px] text-text-3">
                    Added {{ fmtDate(tk.created_at) }}<span v-if="tk.last_used_at"> · last used {{ fmtDate(tk.last_used_at) }}</span><span v-if="tk.expires_at"> · expires {{ fmtDate(tk.expires_at) }}</span><span v-else> · never expires</span>
                  </div>
                </div>
                <button class="rounded-lg p-1.5 text-text-3 transition hover:bg-crit-bg hover:text-crit-text" title="Revoke" @click="confirmTk = tk">
                  <Trash2 class="h-4 w-4" />
                </button>
              </div>
            </div>
            <p v-else class="text-[12px] text-text-3">No API tokens yet.</p>
          </div>
        </section>

        <!-- Active sessions (feature-flagged) -->
        <section v-if="auth.features.sessions" id="sessions" class="card overflow-hidden scroll-mt-4">
          <div class="flex items-center justify-between border-b border-border px-5 py-3">
            <div class="flex items-center gap-2">
              <MonitorSmartphone class="h-[18px] w-[18px] text-text-2" />
              <h2 class="text-[14px] font-semibold">{{ t('settings.sessions') }}</h2>
            </div>
            <span class="rounded-full bg-ok-bg px-2.5 py-0.5 text-[11px] font-semibold text-ok-text">{{ sessions.length }} active</span>
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
                    <span v-if="s.current" class="shrink-0 rounded-full bg-ok-bg px-2 py-0.5 text-[11px] font-semibold text-ok-text">Current</span>
                  </div>
                  <div class="truncate text-[12px] text-text-3">{{ s.ip || 'unknown IP' }} · {{ sessionRel(s) }}</div>
                </div>
              </div>
              <button v-if="!s.current" class="btn-ghost shrink-0" :disabled="sessionsBusy" @click="revokeSession(s.id)">Revoke</button>
            </div>
            <div v-if="sessions.length === 0" class="px-5 py-6 text-center text-[13px] text-text-3">No active sessions.</div>
            <div v-if="sessions.length > 1" class="px-5 py-4">
              <button class="btn-ghost w-full justify-center" :disabled="sessionsBusy" @click="revokeOtherSessions">
                <LogOut class="h-4 w-4" /> Sign out all other sessions
              </button>
            </div>
          </div>
        </section>
      </div>
    </div>

    <!-- Create-token modal -->
    <Overlay :open="showTokenCreate" title="New API token" mode="centered" @close="showTokenCreate = false">
      <div v-if="createdToken" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">Copy this token now — it won't be shown again.</p>
        <div class="flex items-center gap-2 rounded-xl border border-border bg-surface-2 p-3">
          <code class="flex-1 break-all font-mono text-[12px] select-all">{{ createdToken }}</code>
          <button class="rounded-lg p-1.5 text-text-3 hover:text-text" title="Copy" @click="copyToken"><Copy class="h-4 w-4" /></button>
        </div>
        <div class="flex justify-end"><button class="btn-primary" @click="showTokenCreate = false">Done</button></div>
      </div>
      <form v-else class="flex flex-col gap-3" @submit.prevent="createToken">
        <div><label class="label">Name</label><input v-model="tkName" class="input" placeholder="CI deploy" maxlength="128" autofocus /></div>
        <div>
          <label class="label">Expires</label>
          <select v-model.number="tkExpiry" class="input">
            <option :value="0">Never</option>
            <option :value="30">30 days</option>
            <option :value="90">90 days</option>
            <option :value="365">1 year</option>
          </select>
        </div>
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
          <div>
            <label class="label">{{ t('settings.tokenRules') }}</label>
            <textarea v-model="tkRules" rows="3" class="input font-mono" placeholder="env=gmr&#10;env=rda machine=web0*" />
            <p class="mt-1 text-[11.5px] text-text-3">{{ t('settings.tokenRulesHint') }}</p>
          </div>
        </template>
        <p v-if="tkErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ tkErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showTokenCreate = false">Cancel</button>
          <button type="submit" :disabled="tkBusy" class="btn-primary">{{ tkBusy ? 'Creating…' : 'Create token' }}</button>
        </div>
      </form>
    </Overlay>

    <Confirm
      :open="!!confirmTk"
      title="Revoke API token"
      :message="`Revoke “${confirmTk?.name}”? Anything using it will stop working immediately.`"
      confirm-label="Revoke"
      danger
      @confirm="revokeToken"
      @close="confirmTk = null"
    />

    <!-- Add-passkey modal -->
    <Overlay :open="showPkName" title="Add a passkey" mode="centered" @close="showPkName = false">
      <form class="flex flex-col gap-3" @submit.prevent="createPasskey">
        <p class="text-[13px] text-text-2">Give this passkey a name so you can recognise it later (e.g. "MacBook Touch ID", "YubiKey").</p>
        <input v-model="pkName" class="input" placeholder="My passkey" maxlength="128" autofocus />
        <p v-if="pkErr" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ pkErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showPkName = false">Cancel</button>
          <button type="submit" :disabled="pkBusy" class="btn-primary">{{ pkBusy ? 'Waiting…' : 'Create passkey' }}</button>
        </div>
      </form>
    </Overlay>

    <Confirm
      :open="!!confirmPk"
      title="Remove passkey"
      :message="`Remove “${confirmPk?.name}”? You won't be able to sign in with it anymore.`"
      confirm-label="Remove"
      danger
      @confirm="removePasskey"
      @close="confirmPk = null"
    />

    <!-- OTP setup modal -->
    <Overlay :open="showOtpSetup" title="Set up two-factor" mode="centered" @close="showOtpSetup = false">
      <OtpSetup v-if="showOtpSetup" @verified="onOtpVerified" />
    </Overlay>

    <!-- Regenerate recovery codes modal -->
    <Overlay :open="showRegen" title="Recovery codes" mode="centered" @close="showRegen = false">
      <div v-if="regenCodes.length" class="flex flex-col gap-3 text-[13px]">
        <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
          Your old codes are now invalid. Save these — shown only once.
        </p>
        <div class="grid grid-cols-2 gap-2 rounded-xl border border-border bg-surface-2 p-3 font-mono">
          <span v-for="c in regenCodes" :key="c" class="select-all">{{ c }}</span>
        </div>
        <div class="flex justify-end"><button class="btn-primary" @click="showRegen = false">Done</button></div>
      </div>
      <form v-else class="flex flex-col gap-3" @submit.prevent="regenerate">
        <p class="text-center text-[13px] text-text-2">Enter the 6-digit code from your authenticator app to generate a new set of recovery codes.</p>
        <div class="flex justify-center"><CodeInput v-model="regenCode" @complete="regenerate" /></div>
        <p v-if="regenErr" class="rounded-lg bg-crit-bg px-3 py-2 text-center text-[12px] text-crit-text">{{ regenErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showRegen = false">Cancel</button>
          <button type="submit" class="btn-primary" :disabled="regenCode.length < 6">Generate</button>
        </div>
      </form>
    </Overlay>

    <!-- Turn off two-factor: the same code prompt -->
    <Overlay :open="showDisable" title="Disable two-factor" mode="centered" @close="showDisable = false">
      <form class="flex flex-col gap-3" @submit.prevent="disableOtp">
        <p class="text-center text-[13px] text-text-2">Enter the 6-digit code from your authenticator app to turn two-factor off.</p>
        <div class="flex justify-center"><CodeInput v-model="disableCode" @complete="disableOtp" /></div>
        <p v-if="otpErr" class="rounded-lg bg-crit-bg px-3 py-2 text-center text-[12px] text-crit-text">{{ otpErr }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="showDisable = false">Cancel</button>
          <button type="submit" class="btn-danger" :disabled="disableCode.length < 6">Disable</button>
        </div>
      </form>
    </Overlay>
  </div>
</template>

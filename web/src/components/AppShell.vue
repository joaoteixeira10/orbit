<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterView, useRoute, useRouter } from 'vue-router'
import {
  LayoutDashboard,
  Network,
  LayoutGrid,
  Server,
  ClipboardList,
  UserRound,
  RefreshCw as Refresh,
  ArrowUp,
  BookOpen,
  Settings,
  ShieldEllipsis,
  Crown,
  Moon,
  Sun,
  LogOut,
  ChevronDown,
  Copy,
  RefreshCw,
  VenetianMask,
  FileBadge,
  Menu,
} from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { useToastStore } from '@/stores/toast'
import { getAccessToken, setAccessToken } from '@/lib/api'
import { meApi } from '@/api'
import { useFlash } from '@/lib/highlight'
import ServerStatus from '@/components/ServerStatus.vue'
import ChangeDialog from '@/components/proxy/ChangeDialog.vue'
import { useProxiesStore } from '@/stores/proxies'
import { P } from '@/lib/privileges'
import { actionKind, changeDetail, fmtDur, uniq } from '@/lib/proxy'

const auth = useAuthStore()
const prefs = usePrefsStore()
const branding = useBrandingStore()
const toast = useToastStore()
const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const menuOpen = ref(false)
const displayName = computed(() => auth.user?.display_name || auth.user?.username || '')
const roleLabel = computed(() => auth.user?.roles?.map((r) => r.name).join(', ') || 'none')
// v0's role pill colours: admin purple, operator blue, others grey.
const roleClass = computed(() => {
  const names = auth.user?.roles?.map((r) => r.name) ?? []
  if (auth.user?.roles?.some((r) => r.superuser)) return 'admin'
  if (names.includes('operator')) return 'operator'
  return names.length ? '' : 'off'
})

async function copyToken() {
  const tok = getAccessToken()
  if (!tok) return
  try {
    await navigator.clipboard.writeText(tok)
    toast.success('Access token copied — paste it as a Bearer token in your shell')
  } catch {
    toast.error('Could not copy to clipboard')
  }
  menuOpen.value = false
}
async function rotateToken() {
  try {
    const r = await meApi.rotateToken()
    setAccessToken(r.access_token) // new refresh token arrives as the HttpOnly cookie
    toast.success('Token rotated — any copied token is now revoked')
  } catch (e: any) {
    toast.error(e.message)
  }
  menuOpen.value = false
}

interface NavItem {
  name: string
  label: string
  icon: typeof LayoutDashboard
  count?: number | null
  alert?: number // requests waiting for you
  params?: Record<string, string>
}
const px = useProxiesStore()
// v0's sidebar: Proxies, Operations, Security.
const sections = computed<{ title: string; items: NavItem[] }[]>(() => {
  const E = px.entries
  const lastProxy = typeof route.params.name === 'string' ? route.params.name : ''
  const out: { title: string; items: NavItem[] }[] = [
    {
      title: t('nav.proxies'),
      items: [
        { name: 'proxies', label: t('nav.proxies'), icon: Network, count: px.data?.proxies.length ?? null, params: lastProxy ? { name: lastProxy } : undefined },
        ...(auth.features.certificates !== false && auth.can(P.certsRead) ? [{ name: 'certificates', label: t('cert.nav'), icon: FileBadge }] : []),
      ],
    },
    {
      title: t('nav.operations'),
      items: [
        { name: 'operations-services', label: t('nav.services'), icon: LayoutGrid, count: px.data ? uniq(E.map((e) => e.service)).length : null, alert: (px.data?.requests ?? []).filter((r) => r.can_decide).length },
        { name: 'operations-servers', label: t('nav.servers'), icon: Server, count: px.data ? uniq(E.map((e) => e.machine)).length : null },
      ],
    },
  ]
  const sec: NavItem[] = []
  if (auth.can(P.auditRead)) sec.push({ name: 'audit', label: t('nav.audit'), icon: ClipboardList })
  sec.push({ name: 'dashboard', label: t('nav.yourAccess'), icon: UserRound })
  if (auth.isAdminArea) sec.push({ name: 'admin', label: t('nav.admin'), icon: ShieldEllipsis })
  if (auth.isOwner) sec.push({ name: 'owner', label: t('nav.owner'), icon: Crown })
  out.push({ title: t('nav.security'), items: sec })
  return out
})
const allNav = computed(() => sections.value.flatMap((s) => s.items))

// Glow nav items that appear live (e.g. after an admin grants a privilege).
const { flash: flashNav, isFlashing: navFlashing } = useFlash<string>()
watch(allNav, (next, prev) => {
  const had = new Set((prev ?? []).map((i) => i.name))
  flashNav(next.filter((i) => !had.has(i.name)).map((i) => i.name))
})

function navTo(item: NavItem) {
  router.push({ name: item.name, params: item.params })
}

// Phones and narrow tablets: the sidebar is a drawer behind the menu button.
// Same breakpoint as the CSS below.
const narrowMq = window.matchMedia('(max-width: 1023px)')
const narrow = ref(narrowMq.matches)
const drawerOpen = ref(false)
function onNarrow(e: MediaQueryListEvent) {
  narrow.value = e.matches
  if (!e.matches) drawerOpen.value = false
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && drawerOpen.value) drawerOpen.value = false
}
// Any navigation closes it (nav items, links inside pages, back button).
watch(() => route.fullPath, () => (drawerOpen.value = false))
// The page behind an open drawer doesn't scroll.
watch(drawerOpen, (open) => (document.documentElement.style.overflow = open ? 'hidden' : ''))

const isActive = (name: string) => route.name === name
// Breadcrumbs, as in v0: "lb / proxies / gmr".
const crumbs = computed<{ path: string[]; last: string }>(() => {
  const n = String(route.name ?? '')
  if (n === 'proxies') return { path: [t('nav.proxies').toLowerCase()], last: String(route.params.name || '') }
  if (n.startsWith('operations-')) return { path: [t('nav.operations').toLowerCase()], last: t(n === 'operations-servers' ? 'nav.servers' : 'nav.services').toLowerCase() }
  if (n === 'audit') return { path: [t('nav.security').toLowerCase()], last: t('nav.audit').toLowerCase() }
  const item = allNav.value.find((i) => i.name === n)
  return { path: [], last: item?.label ?? (n === 'settings' ? t('nav.settings') : n) }
})

// The live proxy state (shared with the pages) and "updated 3s ago".
const clock = ref(Date.now())
let tick: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  px.start()
  tick = setInterval(() => (clock.value = Date.now()), 1000)
  window.addEventListener('scroll', onScroll, { passive: true })
  narrowMq.addEventListener('change', onNarrow)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  px.stop()
  if (tick) clearInterval(tick)
  window.removeEventListener('scroll', onScroll)
  narrowMq.removeEventListener('change', onNarrow)
  window.removeEventListener('keydown', onKey)
  document.documentElement.style.overflow = ''
})
const updated = computed(() => (px.receivedAt ? t('nav.updatedAgo', { d: fmtDur((clock.value - px.receivedAt) / 1000) }) : ''))
const refreshing = ref(false)
async function refreshNow() {
  refreshing.value = true
  await px.fetch(true)
  refreshing.value = false
}
const onProxyPages = computed(() => ['proxies', 'operations-services', 'operations-servers'].includes(String(route.name)))

// Recent server changes (the sidebar feed).
const activity = computed(() =>
  (px.data?.activity ?? []).slice(0, 30).map((a) => {
    const d = changeDetail(a.detail)
    return {
      id: a.id,
      kind: actionKind(a.action),
      ok: d.ok !== false,
      targets: d.targets ?? [],
      scope: d.scope ?? '',
      until: d.until ? new Date(d.until).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '',
      title: (d.targets ?? []).join('\n') + (d.error ? '\n\n' + d.error : '') + (d.reason ? '\n\n' + d.reason : ''),
      ago: fmtDur((clock.value - new Date(a.created_at).getTime()) / 1000),
      who: a.user_name || a.ip,
    }
  }),
)

// Change freezes on now, or starting within a day.
const freezes = computed(() =>
  (px.data?.freezes ?? []).filter((f) => f.active || new Date(f.from).getTime() - clock.value < 24 * 3600 * 1000),
)
const whenFreeze = (iso: string) => new Date(iso).toLocaleString([], { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

// Back to top, as in v0.
const showTop = ref(false)
function onScroll() {
  showTop.value = window.scrollY > 400
}
function toTop() {
  window.scrollTo({ top: 0, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' })
}
function toggleTheme() {
  prefs.setTheme(prefs.isDark() ? 'light' : 'dark')
}
function goSettings() {
  menuOpen.value = false
  router.push({ name: 'settings' })
}
async function logout() {
  menuOpen.value = false
  await auth.logout()
  router.push({ name: 'login' })
}
async function stopImpersonating() {
  try {
    await auth.stopImpersonation()
    toast.success(t('impersonate.returned'))
    router.push({ name: 'proxies' })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="app">
    <div v-if="drawerOpen" class="side-backdrop" aria-hidden="true" @click="drawerOpen = false" />
    <aside id="sidebar" class="side" :class="{ open: drawerOpen }" :inert="(narrow && !drawerOpen) || undefined">
      <div class="brand">
        <!-- Mark: a developer theme's crest, an uploaded logo, else the default
             crest (the favicon). Hidden by the "Show logo" toggle (not a theme's). -->
        <template v-if="branding.showMark">
          <img v-if="branding.logoMode === 'theme'" :src="branding.crestUrl" alt="logo" class="logo" />
          <img v-else-if="branding.logoMode === 'upload'" :src="branding.logoDataURL" alt="logo" class="brand-logo" />
          <img v-else :src="DEFAULT_MARK" alt="" class="logo" aria-hidden="true" />
        </template>
        <div v-if="branding.showName" class="brand-text">
          <b v-if="branding.wordmarkIsTheme && branding.theme" class="brand-word blackletter" :style="{ color: branding.theme.mark, '-webkit-text-stroke': `1.6px ${branding.theme.halo}` }">UBonito</b>
          <b v-else>{{ branding.appName }}</b>
          <small>nginx · haproxy</small>
        </div>
      </div>

      <nav class="nav" aria-label="Main">
        <template v-for="sec in sections" :key="sec.title">
          <div class="nav-title cap">{{ sec.title }}</div>
          <button
            v-for="item in sec.items"
            :key="item.name"
            class="nav-item"
            :class="[{ on: isActive(item.name) }, navFlashing(item.name) && 'nav-flash']"
            @click="navTo(item)"
          >
            <component :is="item.icon" :size="17" :stroke-width="1.8" />
            <span>{{ item.label }}</span>
            <span v-if="item.alert" class="count alert" :title="t('rq.toDecide', { n: item.alert }, item.alert)">{{ item.alert }}</span>
            <span v-else-if="item.count != null" class="count">{{ item.count }}</span>
          </button>
        </template>
      </nav>

      <div class="nav-title cap">{{ t('nav.activity') }}</div>
      <ol class="activity">
        <li v-for="a in activity" :key="a.id" class="act" :class="[a.kind.toLowerCase(), { failed: !a.ok }]">
          <span class="act-kind" :class="[a.kind, { failed: !a.ok }]">{{ a.kind }}{{ a.ok ? '' : ' · ' + t('nav.failed') }}</span>
          <span v-if="a.scope" class="act-target" :title="a.title">{{ a.scope }} <em>· {{ t('px.nServers', { n: a.targets.length }, a.targets.length) }}</em></span>
          <span v-else class="act-target" :title="a.title">{{ a.targets[0] || '' }}<em v-if="a.targets.length > 1"> +{{ a.targets.length - 1 }}</em></span>
          <span class="act-meta">{{ t('nav.ago', { d: a.ago }) }} · {{ a.who }}<template v-if="a.until"> · ⏱ {{ t('px.until', { at: a.until }) }}</template></span>
        </li>
        <li v-if="!activity.length" class="empty">{{ t('nav.noChanges') }}</li>
      </ol>

      <div class="side-foot">
        <!-- The repository link lives in the About dialog (click the status line). -->
        <ServerStatus />
      </div>
    </aside>

    <main class="main">
      <!-- Impersonation banner: persistent while acting as another user. -->
      <div v-if="auth.impersonator" class="imp-banner">
        <VenetianMask :size="16" />
        <span>{{ t('impersonate.banner', { user: displayName, admin: auth.impersonator.username }) }}</span>
        <button class="btn" @click="stopImpersonating">{{ t('impersonate.return') }}</button>
      </div>

      <header class="topbar">
        <button
          class="icon-btn menu-btn"
          :aria-label="drawerOpen ? t('nav.closeMenu') : t('nav.openMenu')"
          :title="drawerOpen ? t('nav.closeMenu') : t('nav.openMenu')"
          aria-controls="sidebar"
          :aria-expanded="drawerOpen"
          @click="drawerOpen = !drawerOpen"
        >
          <Menu :size="17" />
        </button>
        <div class="crumbs">lb / <template v-for="c in crumbs.path" :key="c">{{ c }} / </template><b>{{ crumbs.last }}</b></div>
        <div class="top-actions">
          <span class="cap">{{ updated }}</span>
          <button class="icon-btn" :aria-label="t('nav.refresh')" :title="t('nav.refresh')" :disabled="refreshing" @click="refreshNow">
            <Refresh :size="15" :class="{ 'animate-spin': refreshing }" />
          </button>
          <button class="icon-btn" :aria-label="prefs.isDark() ? 'Light' : 'Dark'" :title="prefs.isDark() ? 'Light' : 'Dark'" @click="toggleTheme">
            <Sun v-if="prefs.isDark()" :size="15" />
            <Moon v-else :size="15" />
          </button>

          <div v-if="auth.user" class="user">
            <button class="user-btn" :aria-expanded="menuOpen" @click="menuOpen = !menuOpen">
              <span class="user-name">{{ displayName }}</span>
              <span class="role" :class="roleClass">{{ roleLabel }}</span>
              <ChevronDown :size="14" class="chev" :class="{ 'rotate-180': menuOpen }" />
            </button>

            <div v-if="menuOpen" class="fixed inset-0 z-40" @click="menuOpen = false" />
            <div v-if="menuOpen" class="menu">
              <button class="menu-item" @click="goSettings"><Settings :size="16" /> {{ t('nav.settings') }}</button>
              <div class="menu-div" />
              <button class="menu-item" @click="copyToken"><Copy :size="16" /> Copy access token</button>
              <button class="menu-item" @click="rotateToken"><RefreshCw :size="16" /> Rotate token</button>
              <a class="menu-item" href="/api/docs" target="_blank" rel="noopener" @click="menuOpen = false"><BookOpen :size="16" /> {{ t('nav.apiDocs') }}</a>
              <div class="menu-div" />
              <button class="menu-item" @click="logout"><LogOut :size="16" /> {{ t('nav.signOut') }}</button>
            </div>
          </div>
        </div>
      </header>

      <!-- Change freezes: on now, or starting within a day. -->
      <div v-for="f in freezes" :key="f.id" class="imp-banner freeze" role="status">
        <span>
          <b>{{ f.active ? t('px.freezeOn', { until: whenFreeze(f.until) }) : t('px.freezeSoon', { from: whenFreeze(f.from), until: whenFreeze(f.until) }) }}</b>
          {{ f.reason }}<template v-if="f.rules.length"> · {{ f.rules.join('; ') }}</template> · {{ t('px.freezeAdmins') }}
        </span>
      </div>

      <template v-if="onProxyPages">
        <div v-if="px.error" class="alert" role="alert">{{ t('nav.apiDown') }}<pre>{{ px.error }}</pre></div>
        <template v-for="p in px.data?.proxies ?? []" :key="p.id">
          <div v-if="!p.reachable" class="alert" role="alert">
            {{ t('nav.proxyDown', { name: p.name }) }}<template v-if="p.fetched"> {{ t('nav.lastData', { d: fmtDur((clock - new Date(p.fetched).getTime()) / 1000) }) }}</template>
            <pre>{{ p.error }}</pre>
          </div>
        </template>
      </template>

      <RouterView />
    </main>

    <ChangeDialog />
    <button v-if="showTop" type="button" class="to-top" :aria-label="t('nav.toTop')" :title="t('nav.toTop')" @click="toTop"><ArrowUp :size="17" /></button>
  </div>
</template>

<style scoped>
/* Orbit v0's shell (tag v0-python: admin-host/admin/index.html). */
.app {
  display: grid;
  grid-template-columns: 236px minmax(0, 1fr);
  min-height: 100vh;
}
.side {
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;
  background: var(--side);
  border-right: 1px solid var(--border);
  padding: 16px 12px 16px;
  display: flex;
  flex-direction: column;
}
.brand {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 2px 8px 16px;
  border-bottom: 1px solid var(--border);
  margin-bottom: 6px;
}
.logo {
  width: 28px;
  height: 28px;
  flex: none;
}
.brand-word {
  font-size: 19px !important;
  line-height: 1;
  letter-spacing: 0.01em;
}
.brand-logo {
  height: 28px;
  width: auto;
  max-width: 160px;
  object-fit: contain;
  border-radius: 6px;
  flex: none;
}
.brand-text {
  min-width: 0;
}
.brand-text b {
  display: block;
  font-size: 14px;
}
.brand-text small {
  color: var(--text-3);
  font: 11px var(--font-mono);
}
.nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.nav-title {
  padding: 16px 8px 6px;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 8px;
  color: var(--text-2);
  border: 1px solid transparent;
  background: none;
  text-align: left;
  cursor: pointer;
}
.nav-item svg {
  flex: none;
}
.nav-item:hover {
  background: var(--surface-hover);
  color: var(--text);
}
.nav-item.on {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}
.count {
  margin-left: auto;
  font: 11px var(--font-mono);
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-2);
  border: 1px solid var(--border);
}
.count.alert {
  color: #fff;
  background: var(--warn);
  font-weight: 700;
}
.nav-item.on .count {
  color: var(--accent);
}
.activity {
  list-style: none;
  margin: 0;
  padding: 0 4px;
  display: grid;
  gap: 10px;
  min-height: 0;
  overflow-y: auto;
}
.act {
  border-left: 2px solid var(--ok);
  padding: 1px 0 1px 10px;
  min-width: 0;
}
.act.disable {
  border-color: var(--warn);
}
.act.drain {
  border-color: var(--drain);
}
.act.restore {
  border-color: var(--accent);
}
.act.reset {
  border-color: var(--text-3);
}
.act.failed {
  border-color: var(--crit);
}
.act-target {
  display: block;
  font: 12px var(--font-mono);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.act-target em {
  color: var(--text-3);
  font-style: normal;
}
.act-meta {
  font: 11px var(--font-mono);
  color: var(--text-3);
}
.activity .empty {
  color: var(--text-3);
  font-size: 12.5px;
}
.to-top {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 6;
  width: 40px;
  height: 40px;
  padding: 0;
  border-radius: 50%;
  display: grid;
  place-items: center;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  box-shadow: var(--shadow);
  cursor: pointer;
}
.side-foot {
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.main {
  min-width: 0;
  padding: 16px 28px 48px;
}
.imp-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 16px;
  padding: 10px 14px;
  border-radius: 10px;
  font-weight: 600;
  color: var(--warn-text);
  background: var(--warn-bg);
  border: 1px solid var(--warn-border);
}
.imp-banner.freeze {
  font-weight: 500;
  color: var(--text);
  background: var(--crit-bg);
  border-color: var(--crit-border);
}
.imp-banner.freeze b {
  color: var(--crit);
  margin-right: 6px;
}
.imp-banner span {
  flex: 1;
  min-width: 0;
}
.topbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 18px;
  min-height: 36px;
}
.crumbs {
  font: 11.5px var(--font-mono);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-3);
}
.crumbs b {
  color: var(--text);
  font-weight: 600;
}
.top-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.icon-btn {
  width: 32px;
  height: 32px;
  display: grid;
  place-items: center;
  padding: 0;
  border-radius: 7px;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text);
  cursor: pointer;
}
.icon-btn:hover {
  background: var(--surface-hover);
}
.user {
  position: relative;
  padding-left: 8px;
  margin-left: 4px;
  border-left: 1px solid var(--border);
  min-width: 0;
}
.user-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 6px;
  border: 0;
  border-radius: 7px;
  background: none;
  color: var(--text);
  cursor: pointer;
}
.user-btn:hover {
  background: var(--surface-hover);
}
.user-name {
  font-weight: 600;
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.role {
  font: 600 10.5px var(--font-mono);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  padding: 2px 7px;
  border-radius: 999px;
  color: var(--text-2);
  background: var(--surface-3);
  white-space: nowrap;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.role.admin {
  color: var(--violet);
  background: color-mix(in srgb, var(--violet) 14%, transparent);
}
.role.operator {
  color: var(--accent);
  background: var(--accent-soft);
}
.role.off {
  color: var(--crit);
  background: var(--crit-bg);
}
.chev {
  color: var(--text-3);
  transition: transform var(--speed);
}
.menu {
  position: absolute;
  right: 0;
  z-index: 50;
  margin-top: 6px;
  width: 220px;
  border-radius: 10px;
  border: 1px solid var(--border);
  background: var(--surface);
  padding: 4px;
  box-shadow: var(--shadow);
}
.menu-item {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  border-radius: 7px;
  padding: 7px 10px;
  font-size: 12.5px;
  color: var(--text-2);
  background: none;
  border: none;
  cursor: pointer;
}
.menu-item:hover {
  background: var(--surface-hover);
  color: var(--text);
}
.menu-div {
  margin: 4px 0;
  border-top: 1px solid var(--border);
}

.menu-btn {
  display: none;
  flex: none;
}
.side-backdrop {
  position: fixed;
  inset: 0;
  z-index: 55;
  background: rgba(15, 23, 42, 0.45);
}

/* Phones and narrow tablets: the sidebar is a drawer (menu button in the top
   bar). Insets keep content clear of the notch / home bar when installed
   (viewport-fit=cover); they're 0 everywhere else. */
@media (max-width: 1023px) {
  .app {
    grid-template-columns: minmax(0, 1fr);
  }
  .side {
    position: fixed;
    inset: 0 auto 0 0;
    z-index: 60;
    width: min(272px, 85vw);
    height: 100dvh;
    padding: max(16px, env(safe-area-inset-top)) 12px max(16px, env(safe-area-inset-bottom)) max(12px, env(safe-area-inset-left));
    box-shadow: var(--shadow);
    transform: translateX(-100%);
    visibility: hidden;
    transition:
      transform 0.22s ease,
      visibility 0.22s;
  }
  .side.open {
    transform: none;
    visibility: visible;
  }
  .menu-btn {
    display: grid;
  }
  .topbar {
    justify-content: flex-start;
  }
  .crumbs {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .main {
    padding: max(14px, env(safe-area-inset-top)) max(16px, env(safe-area-inset-right)) calc(40px + env(safe-area-inset-bottom)) max(16px, env(safe-area-inset-left));
  }
  .to-top {
    right: max(16px, env(safe-area-inset-right));
    bottom: max(16px, env(safe-area-inset-bottom));
  }
}
@media (max-width: 640px) {
  .top-actions > .cap,
  .user-name {
    display: none;
  }
  .user {
    padding-left: 4px;
    margin-left: 0;
  }
}
@media (prefers-reduced-motion: reduce) {
  .side {
    transition: none;
  }
}
</style>

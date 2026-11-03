<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterView, useRoute, useRouter } from 'vue-router'
import {
  Search, House, CalendarDays, Star, Plus, ChevronRight, MoreHorizontal, Settings, ShieldEllipsis, Crown,
  ClipboardList, LogOut, Moon, Sun, BookOpen, LifeBuoy, VenetianMask, Menu, Archive, FileText, Trash2, PanelLeft, Bell, BellOff, NotebookPen, Timer, Brush,
  PanelRight, FolderOpen,
} from 'lucide-vue-next'
import SideRail from '@/components/SideRail.vue'
import { rail, railFocus, setRailOpen } from '@/lib/rail'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { useToastStore } from '@/stores/toast'
import { useWorkspacesStore, wireWorkspaceLive } from '@/stores/workspaces'
import { usePagesStore, wirePagesLive } from '@/stores/pages'
import { wireTasksLive } from '@/stores/tasks'
import { wireNotesLive } from '@/stores/notes'
import { wireInboxLive } from '@/stores/inbox'
import { useFocusStore, wireFocusLive, clockOf } from '@/stores/focus'
import { drawingsApi } from '@/components/draw/drawing'
import { emptySketch } from '@/lib/sketch'
import { refreshPush } from '@/lib/push'
import InboxButton from '@/components/InboxButton.vue'
import QuickNoteSheet from '@/components/notes/QuickNoteSheet.vue'
import { pagesApi, workspacesApi, type Workspace } from '@/api/planner'
import { ICONS, MODULE_ROUTES } from '@/lib/icons'
import { P } from '@/lib/privileges'
import ServerStatus from '@/components/ServerStatus.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import Avatar from '@/components/ui/Avatar.vue'
import Popover from '@/components/ui/Popover.vue'
import PageTree from '@/components/PageTree.vue'
import CommandPalette from '@/components/CommandPalette.vue'
import NewWorkspaceSheet from '@/components/NewWorkspaceSheet.vue'
import WorkspaceSettingsSheet from '@/components/WorkspaceSettingsSheet.vue'

const auth = useAuthStore()
const prefs = usePrefsStore()
const branding = useBrandingStore()
const toast = useToastStore()
const wss = useWorkspacesStore()
const focus = useFocusStore()
const pages = usePagesStore()
const router = useRouter()
const route = useRoute()
const { t } = useI18n()

// ---- Data ----
// The API refuses everything else while a password change or two-factor
// setup is pending, so the data loads once those are done.
const ready = computed(() => auth.isAuthenticated && !auth.user?.must_change_password && !auth.mustSetupOtp)
watch(
  ready,
  async (r) => {
    if (!r) return
    wireWorkspaceLive()
    wirePagesLive()
    wireTasksLive()
    wireNotesLive()
    wireInboxLive()
    wireFocusLive()
    void refreshPush().catch(() => {})
    try {
      await Promise.all([wss.load(), wss.loadFavorites(), wss.loadCatalog()])
    } catch (e: any) {
      toast.error(e.message)
    }
  },
  { immediate: true },
)
// Another person in the same tab (impersonating, or back): their workspaces.
watch(
  () => auth.user?.id,
  async (id, was) => {
    if (!ready.value || !id || !was || id === was) return
    try {
      await Promise.all([wss.load(), wss.loadFavorites(), wss.loadCatalog()])
    } catch (e: any) {
      toast.error(e.message)
    }
  },
)

// Which workspaces are open in the sidebar (remembered).
const expanded = ref(new Set<string>(JSON.parse(localStorage.getItem('orbit_ws_open') || '[]') as string[]))
function persistExpanded() {
  localStorage.setItem('orbit_ws_open', JSON.stringify([...expanded.value]))
}
function toggleWs(id: string) {
  if (expanded.value.has(id)) expanded.value.delete(id)
  else {
    expanded.value.add(id)
    if (!pages.byWs[id]) void pages.load(id).catch(() => {})
  }
  persistExpanded()
}
const currentWsId = computed(() => (typeof route.params.ws === 'string' ? route.params.ws : ''))
const currentWs = computed(() => wss.byId(currentWsId.value))
watch(currentWsId, (id) => {
  if (!id) return
  wss.setLast(id)
  expanded.value.add(id)
  persistExpanded()
  if (!pages.byWs[id]) void pages.load(id).catch(() => {})
}, { immediate: true })
// A workspace that doesn't exist (deleted, or someone else's): back home.
watch(
  () => [wss.loaded, currentWsId.value] as const,
  ([loaded, id]) => {
    if (loaded && id && !wss.byId(id)) router.replace({ name: 'home' })
  },
)

const pagesOpen = ref(new Set<string>(JSON.parse(localStorage.getItem('orbit_pages_open') || '[]') as string[]))
function togglePages(id: string) {
  if (pagesOpen.value.has(id)) pagesOpen.value.delete(id)
  else pagesOpen.value.add(id)
  localStorage.setItem('orbit_pages_open', JSON.stringify([...pagesOpen.value]))
}
const showArchived = ref(false)
// Opening a page reveals it in the sidebar: its workspace's Pages tree and
// the pages above it are expanded.
watch(
  () => [route.name, route.params.page, currentWsId.value, pages.byWs[currentWsId.value]?.length] as const,
  ([name, page, ws]) => {
    if (name !== 'ws-page' || typeof page !== 'string' || !ws) return
    if (!pagesOpen.value.has(ws)) {
      pagesOpen.value.add(ws)
      localStorage.setItem('orbit_pages_open', JSON.stringify([...pagesOpen.value]))
    }
    for (const p of pages.path(ws, page).slice(0, -1)) if (!pages.open.has(p.id)) pages.toggle(p.id, true)
  },
  { immediate: true },
)

// ---- Module navigation ----
const moduleRoute: Record<string, string> = MODULE_ROUTES
function moduleIcon(key: string) {
  return ICONS[wss.modules.find((m) => m.key === key)?.icon ?? ''] ?? FileText
}
function isModuleActive(ws: string, key: string): boolean {
  if (currentWsId.value !== ws) return false
  const n = String(route.name)
  return n === moduleRoute[key] || (key === 'pages' && (n === 'ws-page' || n === 'ws-trash'))
}
function goModule(ws: string, key: string) {
  router.push({ name: moduleRoute[key] ?? 'ws', params: { ws } })
}
const canEdit = (w: Workspace) => w.role !== 'viewer' && !w.archived_at && !auth.privileges.read_only

async function addPage(ws: string, parent: string | null) {
  try {
    const p = await pagesApi.create(ws, { parent_id: parent, title: '' })
    if (parent) pages.toggle(parent, true)
    pagesOpen.value.add(ws)
    await pages.load(ws)
    router.push({ name: 'ws-page', params: { ws, page: p.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function movePage(ws: string, id: string, parent: string | null, after: string) {
  try {
    await pagesApi.move(id, parent, after)
  } catch (e: any) {
    toast.error(e.message)
  }
  await pages.load(ws)
}

// ---- Sheets and palette ----
const paletteOpen = ref(false)
const newWsOpen = ref(false)
const settingsFor = ref('')
const settingsTab = ref<'general' | 'modules' | 'members'>('general')
function openSettings(id: string, tab: 'general' | 'modules' | 'members' = 'general') {
  settingsTab.value = tab
  settingsFor.value = id
  wsMenu.value = ''
}
const wsMenu = ref('')
// Muting a shared workspace stops notifications about others' activity in it.
async function toggleMute(id: string) {
  wsMenu.value = ''
  const w = wss.byId(id)
  if (!w) return
  const muted = !w.muted
  w.muted = muted
  try {
    await workspacesApi.mute(id, muted)
    toast.success(t(muted ? 'notif.muted' : 'notif.unmuted', { ws: w.name }))
  } catch (e: any) {
    w.muted = !muted
    toast.error(e.message)
  }
}
const wsMenuAnchor = ref<HTMLElement | null>(null)
function openWsMenu(id: string, e: MouseEvent) {
  wsMenu.value = id
  wsMenuAnchor.value = e.currentTarget as HTMLElement
}
function onNewTask(ws: string) {
  router.push({ name: 'ws-tasks', params: { ws }, query: { new: '1' } })
}
// Views ask for the sheets through window events (no prop drilling).
function onOpenSettings(e: Event) {
  const d = (e as CustomEvent).detail as { ws: string; tab?: 'general' | 'modules' | 'members' }
  openSettings(d.ws, d.tab)
}
function onNewWorkspace() {
  newWsOpen.value = true
}

// ---- User menu ----
const userMenu = ref(false)
const userAnchor = ref<HTMLElement | null>(null)
const displayName = computed(() => auth.user?.display_name || auth.user?.username || '')
function go(name: string) {
  userMenu.value = false
  router.push({ name })
}
async function logout() {
  userMenu.value = false
  await auth.logout()
  router.push({ name: 'login' })
}
async function stopImpersonating() {
  try {
    await auth.stopImpersonation()
    toast.success(t('impersonate.returned'))
    router.push({ name: 'home' })
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Layout: drawer below 1024 px, tab bar below 640 px ----
const narrowMq = window.matchMedia('(max-width: 1023px)')
const phoneMq = window.matchMedia('(max-width: 639px)')
const narrow = ref(narrowMq.matches)
const phone = ref(phoneMq.matches)
// The side panel is a column beside the content on a wide window; on a
// narrower one it slides over it.
const railMq = window.matchMedia('(min-width: 1280px)')
const railWide = ref(railMq.matches)
railMq.addEventListener('change', (e) => (railWide.value = e.matches))
// Only where something's open (a page, a sheet, a diagram, a list…).
const railHere = computed(() => !!railFocus(route))
const drawerOpen = ref(false)
// Desktop: the sidebar can be hidden (⌘\ or the toolbar button), like macOS.
const sideHidden = ref(localStorage.getItem('orbit_side_hidden') === '1')
function toggleSide() {
  if (narrow.value) drawerOpen.value = !drawerOpen.value
  else {
    sideHidden.value = !sideHidden.value
    localStorage.setItem('orbit_side_hidden', sideHidden.value ? '1' : '0')
  }
}
function onNarrow(e: MediaQueryListEvent) {
  narrow.value = e.matches
  if (!e.matches) drawerOpen.value = false
}
function onPhone(e: MediaQueryListEvent) {
  phone.value = e.matches
}
function onKey(e: KeyboardEvent) {
  const mod = e.metaKey || e.ctrlKey
  // A ⌘K the page already handled (the editor links the selected text) isn't ours.
  if (mod && e.key.toLowerCase() === 'k' && !e.defaultPrevented) {
    e.preventDefault()
    paletteOpen.value = !paletteOpen.value
  } else if (mod && e.key === '\\') {
    e.preventDefault()
    toggleSide()
  } else if (e.altKey && !mod && e.code === 'KeyN') {
    // ⌥N / Alt+N: a quick note from anywhere.
    e.preventDefault()
    quickNoteOpen.value = true
  } else if (e.key === 'Escape' && drawerOpen.value) drawerOpen.value = false
}
// A quick note from anywhere: ⌥N, the + on Quick Notes, ⌘K, or a page
// asking for it (orbit:quick-note).
const quickNoteOpen = ref(false)
const onQuickNote = () => (quickNoteOpen.value = true)
// Quick Draw's +: a new sketch (Quick Draw's own canvas), open.
async function newSketch() {
  try {
    const d = await drawingsApi.create('me', '', JSON.stringify(emptySketch(t('sketch.layerN', { n: 1 }))))
    void router.push({ name: 'my-drawing', params: { id: d.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch(() => route.fullPath, () => (drawerOpen.value = false))
watch(drawerOpen, (open) => (document.documentElement.style.overflow = open ? 'hidden' : ''))

// The bottom tab bar (phones): the current workspace's modules, else Today/Calendar.
const tabs = computed(() => {
  const w = currentWs.value
  if (!w) return [
    { key: 'today', label: t('nav.today'), icon: House, to: { name: 'home' }, on: route.name === 'home' },
    { key: 'cal', label: t('nav.calendar'), icon: CalendarDays, to: { name: 'calendar' }, on: route.name === 'calendar' },
    { key: 'notes', label: t('notes.mine.title'), icon: NotebookPen, to: { name: 'my-notes' }, on: route.name === 'my-notes' },
    { key: 'draw', label: t('draw.quick'), icon: Brush, to: { name: 'my-drawings' }, on: route.name === 'my-drawings' || route.name === 'my-drawing' },
    { key: 'files', label: t('myFiles.title'), icon: FolderOpen, to: { name: 'my-files' }, on: route.name === 'my-files' },
  ]
  return w.modules.slice(0, 5).map((k) => ({
    key: k, label: t(`modules.${k}`), icon: moduleIcon(k), to: { name: moduleRoute[k] ?? 'ws', params: { ws: w.id } }, on: isModuleActive(w.id, k),
  }))
})

onMounted(() => {
  narrowMq.addEventListener('change', onNarrow)
  phoneMq.addEventListener('change', onPhone)
  window.addEventListener('keydown', onKey)
  window.addEventListener('orbit:settings', onOpenSettings)
  window.addEventListener('orbit:new-workspace', onNewWorkspace)
  window.addEventListener('orbit:quick-note', onQuickNote)
})
onUnmounted(() => {
  narrowMq.removeEventListener('change', onNarrow)
  phoneMq.removeEventListener('change', onPhone)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('orbit:settings', onOpenSettings)
  window.removeEventListener('orbit:new-workspace', onNewWorkspace)
  window.removeEventListener('orbit:quick-note', onQuickNote)
  document.documentElement.style.overflow = ''
})

const wsStyle = computed(() => (currentWs.value ? { '--ws': currentWs.value.color } : {}))
const isMac = /Mac|iPhone|iPad/.test(navigator.platform)
</script>

<template>
  <div class="app" :class="{ 'side-hidden': sideHidden && !narrow, 'rail-on': rail.open && railWide && railHere }">
    <div v-if="drawerOpen" class="side-backdrop" aria-hidden="true" @click="drawerOpen = false" />
    <aside id="sidebar" class="side material" :class="{ open: drawerOpen }" :inert="(narrow && !drawerOpen) || undefined">
      <div class="side-top">
        <div class="brand">
          <!-- Mark: a developer theme's crest, an uploaded logo, else Orbit's icon.
               Hidden by the "Show logo" toggle (not a theme's). -->
          <template v-if="branding.showMark">
            <img v-if="branding.logoMode === 'theme'" :src="branding.crestUrl" :alt="t('acct.logo')" class="logo" />
            <img v-else-if="branding.logoMode === 'upload'" :src="branding.logoDataURL" :alt="t('acct.logo')" class="brand-logo" />
            <img v-else :src="DEFAULT_MARK" alt="" class="logo" aria-hidden="true" />
          </template>
          <template v-if="branding.showName">
            <b v-if="branding.wordmarkIsTheme && branding.theme" class="brand-word blackletter" :style="{ color: branding.theme.mark, '-webkit-text-stroke': `1.6px ${branding.theme.halo}` }">UBonito</b>
            <b v-else class="truncate">{{ branding.appName }}</b>
          </template>
          <span class="ml-auto" />
          <InboxButton v-if="!narrow" />
          <button v-if="!narrow" class="icon-btn h-7 w-7" :title="t('nav.hideSidebar')" @click="toggleSide"><PanelLeft class="h-4 w-4" /></button>
        </div>
        <button class="search-btn" @click="paletteOpen = true">
          <Search class="h-4 w-4" />
          <span class="flex-1 text-left">{{ t('nav.search') }}</span>
          <kbd>{{ isMac ? '⌘' : 'Ctrl' }} K</kbd>
        </button>
      </div>

      <nav class="side-scroll" :aria-label="t('acct.mainNav')">
        <div class="smart">
          <RouterLink :to="{ name: 'home' }" class="smart-tile" :class="{ on: route.name === 'home' }">
            <span class="tile h-7 w-7" style="--tile: #007aff"><House class="h-4 w-4" :stroke-width="2.4" /></span>
            <span>{{ t('nav.today') }}</span>
          </RouterLink>
          <RouterLink :to="{ name: 'calendar' }" class="smart-tile" :class="{ on: route.name === 'calendar' }">
            <span class="tile h-7 w-7" style="--tile: #ff3b30"><CalendarDays class="h-4 w-4" :stroke-width="2.4" /></span>
            <span>{{ t('nav.calendar') }}</span>
          </RouterLink>
          <div class="smart-tile wide" :class="{ on: route.name === 'my-notes' }">
            <RouterLink :to="{ name: 'my-notes' }" class="flex min-w-0 flex-1 items-center gap-2.5">
              <span class="tile h-7 w-7" style="--tile: #ffcc00"><NotebookPen class="h-4 w-4" :stroke-width="2.4" /></span>
              <span class="truncate">{{ t('notes.mine.title') }}</span>
            </RouterLink>
            <button class="jot" :title="t('notes.mine.action') + (isMac ? ' (⌥N)' : ' (Alt+N)')" :aria-label="t('notes.mine.action')" @click="quickNoteOpen = true">
              <Plus class="h-4 w-4" :stroke-width="2.4" />
            </button>
          </div>
          <div class="smart-tile wide" :class="{ on: route.name === 'my-drawings' || route.name === 'my-drawing' }">
            <RouterLink :to="{ name: 'my-drawings' }" class="flex min-w-0 flex-1 items-center gap-2.5" data-testid="quick-draw">
              <span class="tile h-7 w-7" style="--tile: #ff9500"><Brush class="h-4 w-4" :stroke-width="2.4" /></span>
              <span class="truncate">{{ t('draw.quick') }}</span>
            </RouterLink>
            <button class="jot" :title="t('draw.newSketch')" :aria-label="t('draw.newSketch')" data-testid="quick-draw-new" @click="newSketch">
              <Plus class="h-4 w-4" :stroke-width="2.4" />
            </button>
          </div>
          <div class="smart-tile wide" :class="{ on: route.name === 'my-files' }">
            <RouterLink :to="{ name: 'my-files' }" class="flex min-w-0 flex-1 items-center gap-2.5" data-testid="my-files">
              <span class="tile h-7 w-7" style="--tile: #5ac8fa"><FolderOpen class="h-4 w-4" :stroke-width="2.4" /></span>
              <span class="truncate">{{ t('myFiles.title') }}</span>
            </RouterLink>
          </div>
        </div>

        <!-- A focus session running: how long is left, a click away. -->
        <RouterLink v-if="focus.running" :to="{ name: 'ws-focus', params: { ws: focus.running.workspace_id } }" class="focus-pill" :title="t('focus.open')" data-testid="focus-pill">
          <Timer class="h-4 w-4" />
          <span class="min-w-0 flex-1 truncate">{{ focus.running.task_title || t('focus.running') }}</span>
          <span class="tabular-nums">{{ clockOf(focus.left) }}</span>
        </RouterLink>

        <template v-if="wss.favorites.length">
          <div class="sec-title">{{ t('nav.favorites') }}</div>
          <RouterLink
            v-for="f in wss.favorites"
            :key="f.id"
            :to="{ name: 'ws-page', params: { ws: f.workspace_id, page: f.id } }"
            class="side-item"
            :class="{ on: route.params.page === f.id }"
            :style="{ '--ws': wss.byId(f.workspace_id)?.color }"
          >
            <span v-if="f.icon" class="w-4 text-center text-[13px]">{{ f.icon }}</span>
            <Star v-else class="h-3.5 w-3.5" />
            <span class="truncate">{{ f.title || t('pages.untitled') }}</span>
          </RouterLink>
        </template>

        <div class="sec-title flex items-center">
          <span class="flex-1">{{ t('nav.workspaces') }}</span>
          <button v-if="auth.canCreateWorkspaces" class="add-btn" :title="t('ws.new')" @click="newWsOpen = true"><Plus class="h-3.5 w-3.5" /></button>
        </div>

        <div v-for="w in wss.active" :key="w.id" class="ws-block" :style="{ '--ws': w.color }">
          <div class="ws-row group/ws" :class="{ on: currentWsId === w.id && route.name === 'ws' }" @click="router.push({ name: 'ws', params: { ws: w.id } })">
            <button class="chev" :aria-label="t('nav.expand')" @click.stop="toggleWs(w.id)">
              <ChevronRight class="h-3 w-3 transition-transform" :class="{ 'rotate-90': expanded.has(w.id) }" :stroke-width="2.6" />
            </button>
            <WsIcon :icon="w.icon" :color="w.color" :size="22" :name="w.name" />
            <span class="min-w-0 flex-1 truncate font-medium">{{ w.name }}</span>
            <span v-if="w.member_count > 1" class="text-[11px] text-text-3">{{ w.member_count }}</span>
            <button class="more" :class="{ open: wsMenu === w.id }" :title="t('ws.settings')" @click.stop="openWsMenu(w.id, $event)"><MoreHorizontal class="h-4 w-4" /></button>
          </div>
          <div v-if="expanded.has(w.id)" class="ws-mods">
            <template v-for="m in w.modules" :key="m">
              <div v-if="m === 'pages'">
                <div class="side-item group/pg" :class="{ on: isModuleActive(w.id, 'pages') && route.name === 'ws-pages' }" @click="goModule(w.id, 'pages')">
                  <button class="-ml-1 grid h-4 w-4 place-items-center rounded hover:bg-fill" @click.stop="togglePages(w.id)">
                    <ChevronRight class="h-3 w-3 !text-text-3 transition-transform" :class="{ 'rotate-90': pagesOpen.has(w.id) }" :stroke-width="2.6" />
                  </button>
                  <component :is="moduleIcon(m)" class="h-4 w-4" />
                  <span class="flex-1">{{ t('modules.pages') }}</span>
                  <button v-if="canEdit(w)" class="hidden h-5 w-5 place-items-center rounded hover:bg-fill group-hover/pg:grid" :title="t('pages.new')" @click.stop="addPage(w.id, null)">
                    <Plus class="h-3.5 w-3.5 !text-text-2" />
                  </button>
                </div>
                <div v-if="pagesOpen.has(w.id)" class="pl-3">
                  <PageTree
                    :ws="w.id"
                    :nodes="pages.tree(w.id)"
                    :can-edit="canEdit(w)"
                    @add="(p) => addPage(w.id, p)"
                    @move="(id, p, a) => movePage(w.id, id, p, a)"
                  />
                  <div v-if="pages.byWs[w.id] && !pages.byWs[w.id].length" class="px-3 py-1 text-[12px] text-text-3">{{ t('pages.none') }}</div>
                </div>
              </div>
              <button v-else-if="m !== 'home'" class="side-item" :class="{ on: isModuleActive(w.id, m) }" @click="goModule(w.id, m)">
                <component :is="moduleIcon(m)" class="h-4 w-4" />
                <span>{{ t(`modules.${m}`) }}</span>
              </button>
            </template>
          </div>
        </div>

        <div v-if="wss.loaded && !wss.active.length" class="empty-ws">
          <p>{{ t('nav.noWorkspaces') }}</p>
          <button v-if="auth.canCreateWorkspaces" class="btn-tinted btn-sm mt-2" @click="newWsOpen = true"><Plus class="h-3.5 w-3.5" />{{ t('ws.new') }}</button>
        </div>

        <template v-if="wss.archived.length">
          <button class="sec-title flex w-full items-center gap-1 text-left" @click="showArchived = !showArchived">
            <ChevronRight class="h-3 w-3 transition-transform" :class="{ 'rotate-90': showArchived }" :stroke-width="2.6" />
            {{ t('nav.archived') }} <span class="ml-1 font-normal">{{ wss.archived.length }}</span>
          </button>
          <template v-if="showArchived">
            <div v-for="w in wss.archived" :key="w.id" class="ws-row opacity-70" @click="router.push({ name: 'ws', params: { ws: w.id } })">
              <span class="w-4" />
              <WsIcon :icon="w.icon" :color="w.color" :size="22" :name="w.name" />
              <span class="min-w-0 flex-1 truncate">{{ w.name }}</span>
              <Archive class="h-3.5 w-3.5 text-text-3" />
            </div>
          </template>
        </template>
      </nav>

      <div class="side-foot">
        <button ref="userAnchor" class="user-btn" :aria-expanded="userMenu" @click="userMenu = !userMenu">
          <Avatar :name="displayName" :src="auth.user?.avatar_url" :size="28" />
          <span class="min-w-0 flex-1 text-left">
            <span class="block truncate text-[13px] font-medium">{{ displayName }}</span>
            <span class="block truncate text-[11px] text-text-3">@{{ auth.user?.username }}</span>
          </span>
          <Settings class="h-4 w-4 text-text-3" />
        </button>
        <ServerStatus />
      </div>
    </aside>

    <Popover v-if="userMenu" :anchor="userAnchor" placement="top-start" :width="248" @close="userMenu = false">
      <button class="menu-item" @click="go('settings')"><Settings class="h-4 w-4 text-text-2" />{{ t('nav.settings') }}</button>
      <button v-if="auth.can(P.auditRead)" class="menu-item" @click="go('audit')"><ClipboardList class="h-4 w-4 text-text-2" />{{ t('nav.audit') }}</button>
      <button v-if="auth.isAdminArea" class="menu-item" @click="go('admin')"><ShieldEllipsis class="h-4 w-4 text-text-2" />{{ t('nav.admin') }}</button>
      <button v-if="auth.isOwner" class="menu-item" @click="go('owner')"><Crown class="h-4 w-4 text-text-2" />{{ t('nav.owner') }}</button>
      <div class="menu-sep" />
      <button class="menu-item" @click="prefs.setTheme(prefs.isDark() ? 'light' : 'dark')">
        <component :is="prefs.isDark() ? Sun : Moon" class="h-4 w-4 text-text-2" />{{ prefs.isDark() ? t('cmd.lightMode') : t('cmd.darkMode') }}
      </button>
      <button class="menu-item" data-testid="menu-docs" @click="go('docs')"><LifeBuoy class="h-4 w-4 text-text-2" />{{ t('nav.docs') }}</button>
      <a class="menu-item" href="/api/docs" target="_blank" rel="noopener" @click="userMenu = false"><BookOpen class="h-4 w-4 text-text-2" />{{ t('nav.apiDocs') }}</a>
      <div class="menu-sep" />
      <button class="menu-item" @click="logout"><LogOut class="h-4 w-4 text-text-2" />{{ t('nav.signOut') }}</button>
    </Popover>

    <Popover v-if="wsMenu" :anchor="wsMenuAnchor" placement="right-start" :width="230" @close="wsMenu = ''">
      <button class="menu-item" @click="openSettings(wsMenu)"><Settings class="h-4 w-4 text-text-2" />{{ t('ws.settings') }}</button>
      <button class="menu-item" @click="openSettings(wsMenu, 'modules')"><FileText class="h-4 w-4 text-text-2" />{{ t('ws.modules') }}</button>
      <button class="menu-item" @click="openSettings(wsMenu, 'members')"><Plus class="h-4 w-4 text-text-2" />{{ t('ws.members') }}</button>
      <button v-if="(wss.byId(wsMenu)?.member_count ?? 1) > 1" class="menu-item" @click="toggleMute(wsMenu)">
        <component :is="wss.byId(wsMenu)?.muted ? Bell : BellOff" class="h-4 w-4 text-text-2" />{{ wss.byId(wsMenu)?.muted ? t('notif.unmute') : t('notif.mute') }}
      </button>
      <div class="menu-sep" />
      <button class="menu-item" @click="router.push({ name: 'ws-trash', params: { ws: wsMenu } }); wsMenu = ''"><Trash2 class="h-4 w-4 text-text-2" />{{ t('pages.trash') }}</button>
    </Popover>

    <main class="main" :style="wsStyle">
      <div v-if="auth.impersonator" class="imp-banner">
        <VenetianMask class="h-4 w-4" />
        <span>{{ t('impersonate.banner', { user: displayName, admin: auth.impersonator.username }) }}</span>
        <button class="btn btn-sm" @click="stopImpersonating">{{ t('impersonate.return') }}</button>
      </div>

      <!-- Narrow screens: a slim bar with the menu and search. -->
      <header v-if="narrow || sideHidden" class="topbar">
        <button class="icon-btn" :aria-label="t('nav.openMenu')" aria-controls="sidebar" :aria-expanded="drawerOpen" @click="toggleSide">
          <Menu v-if="narrow" class="h-5 w-5" />
          <PanelLeft v-else class="h-[18px] w-[18px]" />
        </button>
        <div class="flex min-w-0 flex-1 items-center justify-center gap-2 font-semibold">
          <WsIcon v-if="currentWs" :icon="currentWs.icon" :color="currentWs.color" :size="20" :name="currentWs.name" />
          <span class="truncate">{{ currentWs?.name ?? branding.appName }}</span>
        </div>
        <InboxButton placement="bottom-end" />
        <button class="icon-btn" :aria-label="t('nav.search')" @click="paletteOpen = true"><Search class="h-[18px] w-[18px]" /></button>
      </header>

      <div class="content" :class="{ 'with-tabs': phone }">
        <RouterView v-if="ready" />
      </div>

      <nav v-if="phone" class="tabbar material" :aria-label="t('ws.modules')">
        <RouterLink v-for="tb in tabs" :key="tb.key" :to="tb.to" class="tab" :class="{ on: tb.on }">
          <component :is="tb.icon" class="h-[22px] w-[22px]" :stroke-width="tb.on ? 2.3 : 1.8" />
          <span>{{ tb.label }}</span>
        </RouterLink>
      </nav>
    </main>

    <!-- The side panel (as Folio's): a column, or a drawer on narrow windows. -->
    <aside v-if="railHere && rail.open && railWide" class="rail-col" :aria-label="t('outline.title')"><SideRail /></aside>
    <template v-else-if="railHere && rail.open">
      <div class="rail-scrim" @click="setRailOpen(false)" />
      <aside class="rail-drawer material" :aria-label="t('outline.title')"><SideRail /></aside>
    </template>
    <button v-if="railHere && !rail.open && !phone" type="button" class="rail-tab" :title="t('outline.title')" :aria-label="t('outline.title')" data-testid="rail-open" @click="setRailOpen(true)"><PanelRight class="h-4 w-4" /></button>

    <CommandPalette :open="paletteOpen" @close="paletteOpen = false" @new-workspace="newWsOpen = true" @new-task="onNewTask" @quick-note="quickNoteOpen = true" />
    <QuickNoteSheet :open="quickNoteOpen" @close="quickNoteOpen = false" />
    <NewWorkspaceSheet :open="newWsOpen" @close="newWsOpen = false" />
    <WorkspaceSettingsSheet :open="!!settingsFor" :ws-id="settingsFor" :initial-tab="settingsTab" @close="settingsFor = ''" />
  </div>
</template>

<style scoped>
.focus-pill {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 8px 10px 0;
  padding: 8px 12px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--accent) 14%, transparent);
  color: var(--accent-text);
  font-size: 13px;
  font-weight: 600;
}
.app {
  --side-w: 264px;
  display: grid;
  grid-template-columns: var(--side-w) minmax(0, 1fr);
  min-height: 100dvh;
  transition: grid-template-columns 0.3s var(--spring);
}
.app.side-hidden {
  grid-template-columns: 0 minmax(0, 1fr);
}
.app.rail-on {
  grid-template-columns: var(--side-w) minmax(0, 1fr) 290px;
}
.app.side-hidden.rail-on {
  grid-template-columns: 0 minmax(0, 1fr) 290px;
}
.rail-col {
  position: sticky;
  top: 0;
  height: 100dvh;
  overflow-y: auto;
  border-left: 0.5px solid var(--border);
  background: var(--bg);
}
.rail-drawer {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 45;
  width: min(320px, 90vw);
  overflow-y: auto;
  box-shadow: -8px 0 32px rgba(0, 0, 0, 0.15);
}
.rail-scrim {
  position: fixed;
  inset: 0;
  z-index: 44;
  background: var(--backdrop);
}
.rail-tab {
  position: fixed;
  top: 76px;
  right: 0;
  z-index: 30;
  display: grid;
  width: 22px;
  height: 40px;
  place-items: center;
  border-radius: 8px 0 0 8px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border), 0 2px 8px rgba(0, 0, 0, 0.06);
  color: var(--text-2);
}
.rail-tab:hover {
  color: var(--text);
}
.app.side-hidden .side {
  visibility: hidden;
}
.side {
  position: sticky;
  top: 0;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  border-right: 0.5px solid var(--border);
  padding-top: env(safe-area-inset-top);
  overflow: hidden;
  z-index: 30;
}
.side-top {
  padding: 12px 12px 6px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 2px 4px 12px;
  font-size: 15px;
  letter-spacing: -0.01em;
}
.logo {
  width: 24px;
  height: 24px;
  flex: none;
  border-radius: 6px;
}
.brand-word {
  font-size: 19px;
  line-height: 1;
  letter-spacing: 0.01em;
}
.brand-logo {
  height: 24px;
  max-width: 150px;
  object-fit: contain;
  border-radius: 6px;
}
.search-btn {
  display: flex;
  align-items: center;
  gap: 7px;
  width: 100%;
  height: 30px;
  padding: 0 8px;
  border-radius: 8px;
  background: var(--fill);
  color: var(--text-3);
  font-size: 13.5px;
  transition: background 0.15s;
}
.search-btn:hover {
  background: var(--fill-strong);
}
.search-btn kbd {
  font: 500 11px var(--font-sans);
  color: var(--text-3);
}
.side-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 6px 10px 16px;
}
.smart {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin: 4px 2px 6px;
}
.smart-tile {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 9px 10px;
  border-radius: 11px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border), 0 1px 2px rgba(0, 0, 0, 0.04);
  font-size: 13.5px;
  font-weight: 600;
  transition: transform 0.15s var(--ease);
}
html.dark .smart-tile {
  background: rgba(255, 255, 255, 0.07);
}
/* Quick Notes spans the row: its name, and + for a new note. */
.smart-tile.wide {
  grid-column: span 2;
  flex-direction: row;
  align-items: center;
  gap: 10px;
  padding: 7px 7px 7px 10px;
}
.jot {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border-radius: 999px;
  color: var(--text-2);
}
.jot:hover {
  background: var(--fill);
  color: var(--text);
}
.smart-tile.on .jot {
  color: var(--accent-fg, #fff);
}
.smart-tile.on .jot:hover {
  background: rgba(255, 255, 255, 0.2);
}
.smart-tile:active {
  transform: scale(0.97);
}
.smart-tile.on,
html.dark .smart-tile.on {
  background: var(--accent);
  color: var(--accent-fg, #fff);
}
.smart-tile.on .tile {
  --tile: var(--accent-fg, #fff) !important;
  color: var(--accent);
}
.sec-title {
  padding: 16px 8px 5px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3);
}
.add-btn {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 6px;
  color: var(--text-2);
}
.add-btn:hover {
  background: var(--fill);
}
.ws-block + .ws-block {
  margin-top: 2px;
}
.ws-row {
  display: flex;
  align-items: center;
  gap: 7px;
  height: 34px;
  padding: 0 6px 0 2px;
  border-radius: 8px;
  font-size: 14px;
  cursor: default;
}
.ws-row:hover {
  background: var(--surface-hover);
}
.ws-row.on {
  background: var(--fill-strong);
}
.ws-row .chev {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border-radius: 4px;
  color: var(--text-3);
}
.ws-row .chev:hover {
  background: var(--fill);
}
.ws-row .more {
  display: none;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 6px;
  color: var(--text-2);
}
.ws-row:hover .more,
.ws-row .more.open {
  /* Shown on hover — and while its menu is open, which is anchored to it. */
  display: grid;
}
.ws-row .more:hover {
  background: var(--fill);
}
.ws-mods {
  padding: 2px 0 6px 22px;
}
.empty-ws {
  margin: 8px;
  padding: 14px;
  border-radius: 12px;
  border: 1px dashed var(--border-2);
  text-align: center;
  font-size: 12.5px;
  color: var(--text-2);
}
.side-foot {
  padding: 8px 10px calc(8px + env(safe-area-inset-bottom));
  border-top: 0.5px solid var(--border);
}
.user-btn {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 6px;
  border-radius: 9px;
}
.user-btn:hover {
  background: var(--surface-hover);
}

.main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg);
}
.content {
  flex: 1;
  min-height: 0;
}
.content.with-tabs {
  padding-bottom: calc(58px + env(safe-area-inset-bottom));
}
.topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 8px;
  height: calc(48px + env(safe-area-inset-top));
  padding: env(safe-area-inset-top) max(10px, env(safe-area-inset-right)) 0 max(10px, env(safe-area-inset-left));
  background: color-mix(in srgb, var(--bg) 82%, transparent);
  backdrop-filter: saturate(180%) blur(20px);
  -webkit-backdrop-filter: saturate(180%) blur(20px);
  border-bottom: 0.5px solid var(--border);
}
.imp-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  background: var(--warn-bg);
  color: var(--warn-text);
  font-size: 13px;
}
.tabbar {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 25;
  display: flex;
  justify-content: space-around;
  padding: 6px 4px calc(4px + env(safe-area-inset-bottom));
  border-top: 0.5px solid var(--border);
}
.tab {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  font-size: 10.5px;
  font-weight: 500;
  color: var(--text-3);
}
.tab.on {
  color: var(--ws);
}
.side-backdrop {
  position: fixed;
  inset: 0;
  z-index: 29;
  background: var(--backdrop);
}

@media (max-width: 1023px) {
  .app {
    grid-template-columns: minmax(0, 1fr);
  }
  .side {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    width: min(var(--side-w), 85vw);
    background: var(--side-solid);
    transform: translateX(-102%);
    transition: transform 0.32s var(--spring);
    box-shadow: var(--shadow-lg);
  }
  .side.open {
    transform: none;
  }
}
</style>

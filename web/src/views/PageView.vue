<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'
import { Star, MoreHorizontal, ChevronRight, FileText, Plus, Smile, Image, RotateCcw, AlertTriangle, Trash2, Archive, Users } from 'lucide-vue-next'
import { pagesApi, type Page } from '@/api/planner'
import { ApiError, CLIENT_ID } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'
import { locale } from '@/lib/dates'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import PageMenu from '@/components/PageMenu.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import PageEditor from '@/components/editor/PageEditor.vue'
import EmojiPicker from '@/components/editor/EmojiPicker.vue'
import CoverPicker from '@/components/editor/CoverPicker.vue'
import { coverBackground, randomCover } from '@/components/editor/covers'

const route = useRoute()
const router = useRouter()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const wsId = computed(() => String(route.params.ws ?? ''))
const pageId = computed(() => String(route.params.page ?? ''))
const ws = computed(() => wss.byId(wsId.value))

// ---- The page ----
const page = ref<Page | null>(null)
const loading = ref(true)
const missing = ref(false)
const invalid = ref(false)
const title = ref('')
const titleEl = ref<HTMLTextAreaElement | null>(null)
const editorRef = ref<InstanceType<typeof PageEditor> | null>(null)

const canEditWs = computed(() => !!ws.value && ws.value.role !== 'viewer' && !ws.value.archived_at && !auth.privileges.read_only)
const editable = computed(() => !!page.value && page.value.can_edit && !page.value.trashed_at && canEditWs.value && !invalid.value)
const viewOnly = computed(() => !!page.value && !page.value.trashed_at && !ws.value?.archived_at && !editable.value && !invalid.value)
const favorite = computed(() => pages.find(pageId.value)?.favorite ?? page.value?.favorite ?? false)
const coverBg = computed(() => coverBackground(page.value?.cover))
// The loaded page is the one in the URL (not the previous one, mid-switch).
const ready = computed(() => !!page.value && page.value.id === pageId.value)

// ---- Saving ----
// The document saves ~800ms after the last change, with the version it was
// edited from; a 409 means someone else saved first (see `conflict`). Each
// load starts a new session so a late answer for a previous page is ignored.
let session = 0
const version = ref(0)
let editGen = 0 // bumped on every edit
let savedGen = 0 // the edit generation the server has
const dirty = ref(false)
const saving = ref(false)
const failed = ref(false)
const justSaved = ref(false)
const conflict = ref<Page | null>(null)
let inflight: Promise<void> | null = null
let contentTimer: ReturnType<typeof setTimeout> | undefined
let savedTimer: ReturnType<typeof setTimeout> | undefined

// Header fields (title, icon, cover) save on their own, without a version.
let titleDirty = false
let titleTimer: ReturnType<typeof setTimeout> | undefined
const metaSaving = ref(0)

function clearTimers() {
  clearTimeout(contentTimer)
  clearTimeout(titleTimer)
  clearTimeout(savedTimer)
}

async function load(id: string) {
  clearTimers()
  const s = ++session
  inflight = null
  saving.value = false
  failed.value = false
  dirty.value = false
  conflict.value = null
  invalid.value = false
  missing.value = false
  titleDirty = false
  editGen = savedGen = 0
  loading.value = true
  try {
    const p = await pagesApi.get(id)
    if (s !== session) return
    page.value = p
    version.value = p.version
    title.value = p.title
    await nextTick()
    autosize()
    // A brand-new page: start with its title.
    if (!p.title && !p.content && editable.value) titleEl.value?.focus()
  } catch (e: any) {
    if (s !== session) return
    page.value = null
    if (e instanceof ApiError && (e.status === 404 || e.status === 403)) missing.value = true
    else toast.error(e.message)
  } finally {
    if (s === session) loading.value = false
  }
}
watch(pageId, (id) => id && void load(id), { immediate: true })

function onEditorChange() {
  editGen++
  dirty.value = true
  justSaved.value = false
  schedule()
}
function schedule(ms = 800) {
  clearTimeout(contentTimer)
  contentTimer = setTimeout(() => void saveContent(), ms)
}

// saveContent saves the document if it has unsaved edits. `over` is the
// version to overwrite ("Keep mine" after a conflict).
function saveContent(over?: number): Promise<void> {
  clearTimeout(contentTimer)
  if (inflight) return inflight
  const p = page.value
  const ed = editorRef.value
  if (!p || !ed || !editable.value) return Promise.resolve()
  if (editGen === savedGen || (conflict.value && over === undefined)) return Promise.resolve()
  const s = session
  const gen = editGen
  const content = ed.getContent()
  saving.value = true
  const run = (async () => {
    try {
      const saved = await pagesApi.update(p.id, { content, version: over ?? version.value })
      if (s !== session || !page.value) return
      version.value = saved.version
      savedGen = gen
      failed.value = false
      conflict.value = null
      page.value = { ...page.value, content, version: saved.version, updated_at: saved.updated_at, updated_by: saved.updated_by }
      justSaved.value = true
      clearTimeout(savedTimer)
      savedTimer = setTimeout(() => (justSaved.value = false), 2500)
    } catch (e: any) {
      if (s !== session) return
      if (e instanceof ApiError && e.status === 409 && e.body?.page) {
        conflict.value = e.body.page as Page
      } else if (e instanceof ApiError && e.status === 409) {
        // Moved to the trash meanwhile: the reload shows the banner.
        void refresh()
      } else {
        failed.value = true
        schedule(5000)
      }
    } finally {
      if (s === session) {
        saving.value = false
        inflight = null
        dirty.value = editGen !== savedGen
        // Typed while it was saving: save the rest soon.
        if (dirty.value && !conflict.value && !failed.value) schedule(300)
      }
    }
  })()
  inflight = run
  return run
}

async function saveTitle() {
  clearTimeout(titleTimer)
  const p = page.value
  if (!titleDirty || !p || !editable.value) return
  titleDirty = false
  const value = title.value.trim()
  await saveMeta(p.id, { title: value }, () => (titleDirty = true))
}

// saveMeta saves header fields (the sidebar tree was patched as they
// changed); favourites, which carry titles and icons, reload.
async function saveMeta(id: string, body: { title?: string; icon?: string; cover?: string }, onFail?: () => void) {
  metaSaving.value++
  try {
    const saved = await pagesApi.update(id, body)
    if (page.value?.id === id) page.value = { ...page.value, updated_at: saved.updated_at, updated_by: saved.updated_by }
    if (favorite.value) void wss.loadFavorites()
  } catch (e: any) {
    onFail?.()
    toast.error(e.message)
  } finally {
    metaSaving.value--
  }
}

// flush saves whatever is pending (⌘S, leaving the page, closing the tab).
async function flush() {
  clearTimeout(contentTimer)
  const titleJob = titleDirty ? saveTitle() : Promise.resolve()
  for (let i = 0; i < 3 && page.value; i++) {
    if (inflight) await inflight
    if (editGen === savedGen || conflict.value || !editable.value) break
    await saveContent()
  }
  await titleJob
}
const flushSoon = () => Promise.race([flush().catch(() => {}), new Promise((r) => setTimeout(r, 4000))])

onBeforeRouteLeave(async () => {
  await flushSoon()
})
onBeforeRouteUpdate(async (to, from) => {
  if (to.params.page !== from.params.page) await flushSoon()
})

// ---- Conflicts ----
function loadTheirs() {
  const p = conflict.value
  if (!p || !page.value) return
  editorRef.value?.setContent(p.content)
  version.value = p.version
  savedGen = editGen
  dirty.value = false
  failed.value = false
  conflict.value = null
  page.value = { ...page.value, content: p.content, version: p.version, updated_at: p.updated_at, updated_by: p.updated_by }
}
function keepMine() {
  const p = conflict.value
  if (!p) return
  conflict.value = null
  if (editGen === savedGen) editGen++
  void saveContent(p.version)
}

// ---- Live updates ----
// Someone else saved: refetch, and take their document when we have no edits
// of our own (with edits, our next save gets the 409 and the user chooses).
let liveTimer: ReturnType<typeof setTimeout> | undefined
const offLive = onLiveEvent('ws', (e) => {
  if (e.what !== 'pages' || String(e.ws) !== wsId.value || e.origin === CLIENT_ID || !page.value) return
  clearTimeout(liveTimer)
  liveTimer = setTimeout(() => void refresh(), 300)
})

async function refresh() {
  const cur = page.value
  if (!cur) return
  const s = session
  let p: Page
  try {
    p = await pagesApi.get(cur.id)
  } catch (e: any) {
    if (s === session && e instanceof ApiError && (e.status === 404 || e.status === 403)) {
      page.value = null
      missing.value = true
    }
    return
  }
  if (s !== session || !page.value) return
  const editingTitle = titleDirty || document.activeElement === titleEl.value
  if (!editingTitle) title.value = p.title
  const next: Page = { ...page.value, title: editingTitle ? page.value.title : p.title, icon: p.icon, cover: p.cover, can_edit: p.can_edit, trashed_at: p.trashed_at, favorite: p.favorite }
  if (p.version > version.value && editGen === savedGen && !inflight && !conflict.value) {
    editorRef.value?.setContent(p.content)
    version.value = p.version
    Object.assign(next, { content: p.content, version: p.version, updated_at: p.updated_at, updated_by: p.updated_by })
  } else if (p.version === version.value) {
    Object.assign(next, { updated_at: p.updated_at, updated_by: p.updated_by })
  }
  page.value = next
  await nextTick()
  autosize()
}

// ---- Header ----
function autosize() {
  const el = titleEl.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${el.scrollHeight}px`
}
function onTitleInput() {
  // A title is one line: pasted line breaks become spaces.
  if (title.value.includes('\n')) title.value = title.value.replace(/\s*\n+\s*/g, ' ')
  autosize()
  if (!page.value) return
  titleDirty = true
  pages.patch({ id: page.value.id, title: title.value })
  clearTimeout(titleTimer)
  titleTimer = setTimeout(() => void saveTitle(), 600)
}
function onTitleEnter(e: KeyboardEvent) {
  if (e.isComposing) return
  e.preventDefault()
  void saveTitle()
  editorRef.value?.focusStart()
}
function onTitleDown(e: KeyboardEvent) {
  // ↓ at the end of the title continues into the document.
  const el = titleEl.value
  if (el && el.selectionStart === el.value.length && el.selectionEnd === el.value.length) {
    e.preventDefault()
    editorRef.value?.focusStart()
  }
}

const iconOpen = ref(false)
const iconAnchor = ref<HTMLElement | null>(null)
function openIcon(e: MouseEvent) {
  if (!editable.value) return
  iconAnchor.value = e.currentTarget as HTMLElement
  iconOpen.value = !iconOpen.value
}
function setIcon(icon: string) {
  if (!page.value) return
  page.value = { ...page.value, icon }
  pages.patch({ id: page.value.id, icon })
  void saveMeta(page.value.id, { icon })
}

const coverOpen = ref(false)
const coverAnchor = ref<HTMLElement | null>(null)
function setCover(cover: string) {
  if (!page.value) return
  page.value = { ...page.value, cover }
  void saveMeta(page.value.id, { cover })
}
function openCover(e: MouseEvent) {
  coverAnchor.value = e.currentTarget as HTMLElement
  coverOpen.value = !coverOpen.value
}

async function toggleFavorite() {
  const p = page.value
  if (!p) return
  const on = !favorite.value
  try {
    await pagesApi.favorite(p.id, on)
    if (page.value?.id === p.id) page.value = { ...page.value, favorite: on }
    pages.patch({ id: p.id, favorite: on })
    await wss.loadFavorites()
  } catch (e: any) {
    toast.error(e.message)
  }
}

const menuOpen = ref(false)
const menuBtn = ref<HTMLElement | null>(null)

// Breadcrumbs: the workspace, then the page's ancestors (long chains fold).
const ancestors = computed(() => pages.path(wsId.value, pageId.value).filter((p) => p.id !== pageId.value))
const crumbs = computed(() => {
  const a = ancestors.value
  if (a.length <= 2) return a.map((p) => ({ p, gap: false }))
  return [{ p: a[0], gap: false }, { p: a[a.length - 1], gap: true }]
})

// ---- Status: "Saving…", "Saved", or who edited when ----
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

function ago(iso: string): string {
  const d = new Date(iso)
  const sec = (d.getTime() - now.value) / 1000
  if (Math.abs(sec) < 45) return t('editor.justNow')
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  const min = sec / 60
  if (Math.abs(min) < 60) return rtf.format(Math.round(min), 'minute')
  const h = min / 60
  if (Math.abs(h) < 24) return rtf.format(Math.round(h), 'hour')
  const days = h / 24
  if (Math.abs(days) < 7) return rtf.format(Math.round(days), 'day')
  const sameYear = d.getFullYear() === new Date(now.value).getFullYear()
  return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year: sameYear ? undefined : 'numeric' }).format(d)
}
function memberName(uid: string): string {
  const m = (wss.members[wsId.value] ?? []).find((x) => x.user_id === uid)
  return m ? m.display_name || m.username : t('editor.someone')
}
const status = computed(() => {
  if (!page.value) return ''
  if (saving.value || metaSaving.value || (dirty.value && !conflict.value && !failed.value)) return t('common.saving')
  if (failed.value) return t('editor.saveFailed')
  if (dirty.value) return t('editor.unsaved')
  if (justSaved.value) return t('common.saved')
  const when = ago(page.value.updated_at)
  const by = page.value.updated_by
  if (!by || by === auth.user?.id || (ws.value?.member_count ?? 1) <= 1) return t('pages.edited', { when })
  return t('editor.editedBy', { when, name: memberName(by) })
})

// Names for "Edited by" in shared workspaces.
watch(
  ws,
  (w) => {
    if (w && w.member_count > 1 && !wss.members[w.id]) void wss.loadMembers(w.id).catch(() => {})
  },
  { immediate: true },
)

// ---- Trash ----
async function restore() {
  const p = page.value
  if (!p) return
  try {
    await pagesApi.restore(p.id)
    await pages.load(wsId.value)
    void wss.loadFavorites()
    toast.success(t('pages.restored'))
    await load(p.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Pages inside ----
const children = computed(() =>
  (pages.byWs[wsId.value] ?? []).filter((p) => p.parent_id === pageId.value && !p.trashed_at).sort((a, b) => a.rank - b.rank),
)
async function addInside() {
  const p = page.value
  if (!p) return
  try {
    const child = await pagesApi.create(wsId.value, { parent_id: p.id, title: '' })
    await pages.load(wsId.value)
    pages.toggle(p.id, true)
    router.push({ name: 'ws-page', params: { ws: wsId.value, page: child.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Keyboard and page lifecycle ----
function onKey(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && !e.shiftKey && !e.altKey && e.key.toLowerCase() === 's') {
    e.preventDefault()
    void flush()
  }
}
function onBeforeUnload(e: BeforeUnloadEvent) {
  if (editGen === savedGen && !titleDirty) return
  void flush()
  e.preventDefault()
  e.returnValue = ''
}
function onVisibility() {
  if (document.visibilityState === 'hidden') void flush()
}

onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('beforeunload', onBeforeUnload)
  window.addEventListener('resize', autosize)
  document.addEventListener('visibilitychange', onVisibility)
  clock = setInterval(() => (now.value = Date.now()), 30_000)
})
onUnmounted(() => {
  offLive()
  clearTimers()
  clearTimeout(liveTimer)
  clearInterval(clock)
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('beforeunload', onBeforeUnload)
  window.removeEventListener('resize', autosize)
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div class="page-view">
    <!-- Bar: breadcrumbs, status, favourite, actions -->
    <div class="page-bar px-4 sm:px-8">
      <nav class="crumbs" :aria-label="t('editor.breadcrumbs')">
        <RouterLink v-if="ws" :to="{ name: 'ws-pages', params: { ws: ws.id } }" class="crumb">
          <WsIcon :icon="ws.icon" :color="ws.color" :size="16" :name="ws.name" />
          <span class="truncate">{{ ws.name }}</span>
        </RouterLink>
        <template v-for="c in crumbs" :key="c.p.id">
          <ChevronRight class="crumb-sep" />
          <template v-if="c.gap">
            <span class="crumb-gap">…</span>
            <ChevronRight class="crumb-sep" />
          </template>
          <RouterLink :to="{ name: 'ws-page', params: { ws: wsId, page: c.p.id } }" class="crumb">
            <span v-if="c.p.icon" class="crumb-emoji">{{ c.p.icon }}</span>
            <span class="truncate">{{ c.p.title || t('pages.untitled') }}</span>
          </RouterLink>
        </template>
        <template v-if="page && ready">
          <ChevronRight class="crumb-sep" />
          <span class="crumb current" aria-current="page">
            <span v-if="page.icon" class="crumb-emoji">{{ page.icon }}</span>
            <span class="truncate">{{ title.trim() || t('pages.untitled') }}</span>
          </span>
        </template>
      </nav>
      <div v-if="page && ready" class="flex shrink-0 items-center gap-0.5">
        <span class="status" :class="{ 'text-warn-text': failed }" aria-live="polite">{{ status }}</span>
        <span v-if="viewOnly" class="pill ml-1.5"><Users class="h-3 w-3" />{{ t('editor.viewOnly') }}</span>
        <button
          v-if="!page.trashed_at"
          class="icon-btn ml-1"
          :class="{ 'fav-on': favorite }"
          :title="favorite ? t('pages.unfavorite') : t('pages.favorite')"
          :aria-label="favorite ? t('pages.unfavorite') : t('pages.favorite')"
          :aria-pressed="favorite"
          @click="toggleFavorite"
        >
          <Star class="h-[18px] w-[18px]" :fill="favorite ? 'currentColor' : 'none'" />
        </button>
        <button v-if="canEditWs && !page.trashed_at" ref="menuBtn" class="icon-btn" :title="t('editor.options')" :aria-label="t('editor.options')" @click="menuOpen = !menuOpen">
          <MoreHorizontal class="h-[18px] w-[18px]" />
        </button>
      </div>
    </div>

    <!-- Not found -->
    <div v-if="missing" class="empty-state mx-auto max-w-md px-4 pt-24">
      <FileText class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
      <p class="text-[15px] text-text-2">{{ t('editor.notFound') }}</p>
      <RouterLink v-if="ws" class="btn-tinted btn-sm mt-4" :to="{ name: 'ws-pages', params: { ws: wsId } }">{{ t('editor.backToPages') }}</RouterLink>
    </div>

    <!-- Loading -->
    <div v-else-if="!page || !ready" class="page-col px-4 pt-16 sm:px-8" aria-busy="true">
      <div v-if="loading" class="skeleton">
        <div class="sk sk-title" />
        <div class="sk w-[92%]" />
        <div class="sk w-[78%]" />
        <div class="sk w-[85%]" />
      </div>
    </div>

    <template v-else>
      <!-- Banners -->
      <div v-if="page.trashed_at || conflict || invalid || ws?.archived_at" class="page-col px-4 pt-2 sm:px-8">
        <div v-if="page.trashed_at" class="banner banner-crit">
          <Trash2 class="h-4 w-4 shrink-0" />
          <span class="flex-1">{{ t('editor.trashed') }}</span>
          <button v-if="canEditWs" class="btn btn-sm" @click="restore"><RotateCcw class="h-3.5 w-3.5" />{{ t('pages.restore') }}</button>
        </div>
        <div v-else-if="ws?.archived_at" class="banner">
          <Archive class="h-4 w-4 shrink-0" />
          <span class="flex-1">{{ t('ws.archivedBanner') }}</span>
        </div>
        <div v-if="conflict" class="banner banner-warn" role="alert">
          <AlertTriangle class="h-4 w-4 shrink-0" />
          <span class="flex-1">{{ t('editor.conflict') }}</span>
          <span class="flex shrink-0 gap-1.5">
            <button class="btn btn-sm" @click="loadTheirs">{{ t('editor.loadTheirs') }}</button>
            <button class="btn-primary btn-sm" @click="keepMine">{{ t('editor.keepMine') }}</button>
          </span>
        </div>
        <div v-if="invalid" class="banner banner-warn">
          <AlertTriangle class="h-4 w-4 shrink-0" />
          <span class="flex-1">{{ t('editor.invalid') }}</span>
        </div>
      </div>

      <!-- Cover -->
      <div v-if="coverBg" class="cover" :style="{ background: coverBg }">
        <div v-if="editable" class="cover-actions">
          <button class="cover-btn" type="button" @click="openCover">{{ t('editor.changeCover') }}</button>
          <button class="cover-btn" type="button" @click="setCover('')">{{ t('editor.remove') }}</button>
        </div>
      </div>

      <div class="px-4 sm:px-8">
        <div class="page-col">
          <header class="page-head" :class="{ 'has-cover': !!coverBg, 'has-icon': !!page.icon }">
            <button
              v-if="page.icon"
              class="page-icon"
              type="button"
              :disabled="!editable"
              :title="editable ? t('editor.changeIcon') : undefined"
              :aria-label="t('editor.changeIcon')"
              @click="openIcon"
            >
              {{ page.icon }}
            </button>
            <div v-if="editable && (!page.icon || !coverBg)" class="page-add">
              <button v-if="!page.icon" class="btn-ghost btn-sm text-text-3" type="button" @click="openIcon"><Smile class="h-4 w-4" />{{ t('editor.addIcon') }}</button>
              <button v-if="!coverBg" class="btn-ghost btn-sm text-text-3" type="button" @click="setCover(randomCover())"><Image class="h-4 w-4" />{{ t('editor.addCover') }}</button>
            </div>
            <textarea
              ref="titleEl"
              v-model="title"
              class="page-title"
              rows="1"
              :placeholder="t('pages.untitled')"
              :readonly="!editable"
              :aria-label="t('common.name')"
              maxlength="300"
              spellcheck="true"
              @input="onTitleInput"
              @blur="saveTitle"
              @keydown.enter="onTitleEnter"
              @keydown.down="onTitleDown"
            />
          </header>

          <PageEditor :key="page.id" ref="editorRef" :content="page.content" :editable="editable" @change="onEditorChange" @invalid="invalid = true" />

          <!-- Pages inside -->
          <section v-if="children.length || editable" class="subpages">
            <h2 v-if="children.length" class="cap mb-1.5 px-2">{{ t('editor.subpages') }}</h2>
            <RouterLink v-for="c in children" :key="c.id" :to="{ name: 'ws-page', params: { ws: wsId, page: c.id } }" class="sub-row">
              <span v-if="c.icon" class="sub-icon">{{ c.icon }}</span>
              <FileText v-else class="h-[18px] w-[18px] shrink-0 text-text-3" :stroke-width="1.8" />
              <span class="truncate" :class="{ 'text-text-3': !c.title }">{{ c.title || t('pages.untitled') }}</span>
            </RouterLink>
            <button v-if="editable" class="sub-row sub-add" type="button" @click="addInside">
              <Plus class="h-[18px] w-[18px] shrink-0" />
              <span>{{ t('pages.addSub') }}</span>
            </button>
          </section>
        </div>
      </div>
    </template>

    <PageMenu v-if="menuOpen && page" :page-id="page.id" :ws="wsId" :anchor="menuBtn" @close="menuOpen = false" />
    <EmojiPicker v-if="iconOpen" :anchor="iconAnchor" :removable="!!page?.icon" @pick="setIcon" @close="iconOpen = false" />
    <CoverPicker v-if="coverOpen" :anchor="coverAnchor" :current="page?.cover ?? ''" @pick="setCover" @close="coverOpen = false" />
  </div>
</template>

<style scoped>
.page-view {
  min-height: 100%;
  padding-bottom: 96px;
}
.page-col {
  max-width: 740px;
  margin: 0 auto;
}

/* ---- Bar ---- */
.page-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 48px;
}
.crumbs {
  display: flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
  flex: 1;
  overflow: hidden;
  font-size: 13.5px;
}
.crumb {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  max-width: 220px;
  padding: 3px 6px;
  border-radius: 6px;
  color: var(--text-2);
  white-space: nowrap;
  transition: background 0.12s var(--ease), color 0.12s var(--ease);
}
a.crumb:hover {
  background: var(--surface-hover);
  color: var(--text);
}
.crumb.current {
  color: var(--text);
  font-weight: 500;
}
.crumb:first-child {
  flex-shrink: 0;
  max-width: 160px;
}
.crumb-emoji {
  font-size: 14px;
  line-height: 1;
}
.crumb-sep {
  width: 14px;
  height: 14px;
  flex: none;
  color: var(--text-faint);
}
.crumb-gap {
  padding: 0 4px;
  color: var(--text-3);
}
.status {
  font-size: 12.5px;
  color: var(--text-3);
  white-space: nowrap;
  max-width: 260px;
  overflow: hidden;
  text-overflow: ellipsis;
}
@media (max-width: 640px) {
  .crumb {
    max-width: 120px;
  }
  .crumb:not(.current):not(:first-child),
  .crumb-gap,
  .crumb-gap + .crumb-sep {
    display: none;
  }
  .crumbs > .crumb-sep:not(:last-of-type) {
    display: none;
  }
  .status {
    max-width: 92px;
  }
}
.fav-on {
  color: var(--yellow);
}
.fav-on:hover {
  color: var(--yellow);
}

/* ---- Banners ---- */
.banner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 10px;
  margin-top: 8px;
  padding: 9px 12px;
  border-radius: 11px;
  font-size: 13.5px;
  background: var(--fill-2);
  color: var(--text-2);
}
.banner-warn {
  background: var(--warn-bg);
  color: var(--warn-text);
}
.banner-crit {
  background: var(--crit-bg);
  color: var(--crit-text);
}
.banner .btn,
.banner .btn-primary {
  white-space: nowrap;
}

/* ---- Cover ---- */
.cover {
  position: relative;
  height: clamp(130px, 26vh, 250px);
  margin-top: 8px;
}
.cover-actions {
  position: absolute;
  right: 16px;
  bottom: 12px;
  display: flex;
  gap: 6px;
  opacity: 0;
  transition: opacity 0.18s var(--ease);
}
.cover:hover .cover-actions,
.cover:focus-within .cover-actions {
  opacity: 1;
}
@media (hover: none) {
  .cover-actions {
    opacity: 1;
  }
}
.cover-btn {
  padding: 4px 10px;
  border: 0;
  border-radius: 7px;
  font-size: 12.5px;
  font-weight: 500;
  color: #1d1d1f;
  background: rgba(255, 255, 255, 0.78);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
  cursor: pointer;
}
.cover-btn:hover {
  background: rgba(255, 255, 255, 0.95);
}
html.dark .cover-btn {
  color: #f5f5f7;
  background: rgba(44, 44, 46, 0.78);
}
html.dark .cover-btn:hover {
  background: rgba(58, 58, 60, 0.95);
}

/* ---- Head: icon, add buttons, title ---- */
.page-head {
  position: relative;
  padding-top: 40px;
}
.page-head.has-cover {
  padding-top: 18px;
}
.page-head.has-cover.has-icon {
  padding-top: 0;
}
.page-icon {
  display: block;
  font-size: 64px;
  line-height: 1.1;
  padding: 2px 6px;
  margin-left: -6px;
  border: 0;
  border-radius: 12px;
  background: transparent;
  cursor: pointer;
  transition: background 0.12s var(--ease);
}
.page-icon:hover:not(:disabled) {
  background: var(--surface-hover);
}
.page-icon:disabled {
  cursor: default;
}
.has-cover .page-icon {
  margin-top: -40px;
  position: relative;
}
.page-add {
  display: flex;
  gap: 4px;
  margin-left: -10px;
  margin-top: 6px;
  min-height: 28px;
  opacity: 0;
  transition: opacity 0.18s var(--ease);
}
.page-head:hover .page-add,
.page-head:focus-within .page-add {
  opacity: 1;
}
@media (hover: none) {
  .page-add {
    opacity: 1;
  }
}
.page-title {
  display: block;
  width: 100%;
  margin: 6px 0 14px;
  padding: 0;
  border: 0;
  outline: none;
  resize: none;
  overflow: hidden;
  background: transparent;
  color: var(--text);
  font: 700 34px/1.18 var(--font-display);
  letter-spacing: -0.025em;
  caret-color: var(--ws);
}
.page-title::placeholder {
  color: var(--text-faint);
}
.page-title:focus-visible {
  box-shadow: none;
}
@media (max-width: 640px) {
  .page-title {
    font-size: 28px;
  }
  .page-icon {
    font-size: 52px;
  }
}

/* ---- Pages inside ---- */
.subpages {
  margin-top: 28px;
  padding-top: 16px;
  border-top: 0.5px solid var(--border);
}
.sub-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  min-height: 36px;
  padding: 6px 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text);
  font-size: 15px;
  text-align: left;
  cursor: pointer;
  transition: background 0.12s var(--ease);
}
.sub-row:hover {
  background: var(--surface-hover);
}
.sub-icon {
  width: 18px;
  text-align: center;
  font-size: 16px;
  line-height: 1;
}
.sub-add {
  color: var(--text-3);
}
.sub-add:hover {
  color: var(--text-2);
}

/* ---- Loading ---- */
.skeleton .sk {
  height: 14px;
  margin-top: 14px;
  border-radius: 7px;
  background: var(--fill-2);
  animation: sk-pulse 1.4s ease-in-out infinite;
}
.skeleton .sk-title {
  height: 34px;
  width: 55%;
  margin: 0 0 28px;
  border-radius: 9px;
}
@keyframes sk-pulse {
  50% {
    opacity: 0.5;
  }
}
</style>

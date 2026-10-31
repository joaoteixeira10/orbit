<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, Search, FileText, Trash2, LayoutGrid, List, Star, Upload, Download, Waypoints, Undo2 } from 'lucide-vue-next'
import RecentChanges from '@/components/pages/RecentChanges.vue'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { pagesApi, type PageSummary } from '@/api/planner'
import { relDay } from '@/lib/dates'
import { fileName } from '@/lib/markdown'
import ImportSheet from '@/components/pages/ImportSheet.vue'
import { downloadAuthed } from '@/lib/api'

// A workspace's pages at a glance: a grid (or list) of every page, newest
// edits first or by title, with a filter. The sidebar tree is for browsing
// the hierarchy; this is for finding. Pages come in from .md files and go
// out as a zip of them.
const route = useRoute()
const router = useRouter()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const changesOpen = ref(false)
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
watch(ws, () => pages.load(ws.value).catch(() => {}))
onMounted(() => pages.load(ws.value).catch(() => {}))

const q = ref('')
const sort = ref<'recent' | 'title'>((localStorage.getItem('orbit_pages_sort') as 'recent' | 'title') || 'recent')
const layout = ref<'grid' | 'list'>((localStorage.getItem('orbit_pages_layout') as 'grid' | 'list') || 'grid')
watch(sort, (v) => localStorage.setItem('orbit_pages_sort', v))
watch(layout, (v) => localStorage.setItem('orbit_pages_layout', v))

// Tags: the workspace's (most used first), one chosen to show its pages
// (?tag=, where a tag under a page's title leads). "#word" in the filter
// looks for tags too.
const tag = computed(() => (typeof route.query.tag === 'string' ? route.query.tag : ''))
const tagCounts = computed(() => {
  const by = new Map<string, { tag: string; n: number }>()
  for (const p of pages.byWs[ws.value] ?? []) {
    for (const x of p.tags ?? []) {
      const k = x.toLowerCase()
      const had = by.get(k)
      by.set(k, { tag: had?.tag ?? x, n: (had?.n ?? 0) + 1 })
    }
  }
  return [...by.values()].sort((a, b) => b.n - a.n || a.tag.localeCompare(b.tag))
})
const pickTag = (x: string) => router.replace({ query: { ...route.query, tag: x.toLowerCase() === tag.value.toLowerCase() ? undefined : x } })
const hasTag = (p: PageSummary, x: string) => (p.tags ?? []).some((y) => y.toLowerCase() === x.toLowerCase())

const list = computed<PageSummary[]>(() => {
  const all = [...(pages.byWs[ws.value] ?? [])].filter((p) => !tag.value || hasTag(p, tag.value))
  const ql = q.value.trim().toLowerCase()
  const shown = !ql
    ? all
    : ql.startsWith('#')
      ? all.filter((p) => (p.tags ?? []).some((y) => y.toLowerCase().includes(ql.slice(1))))
      : all.filter((p) => (p.title || t('pages.untitled')).toLowerCase().includes(ql) || (p.tags ?? []).some((y) => y.toLowerCase().includes(ql)))
  return shown.sort((a, b) => (sort.value === 'title' ? (a.title || '').localeCompare(b.title || '') : b.updated_at.localeCompare(a.updated_at)))
})
const parentTitle = (p: PageSummary) => (p.parent_id ? pages.find(p.parent_id)?.title : '')

// Import: Markdown files, a Notion export, Apple Notes (ImportSheet).
const importing = ref(false)
async function exportAll() {
  try {
    await downloadAuthed(`/workspaces/${ws.value}/export`, `${fileName(workspace.value?.name ?? 'pages')}.zip`)
  } catch (e: any) {
    toast.error(e.message)
  }
}

async function create() {
  try {
    const p = await pagesApi.create(ws.value, { title: '' })
    await pages.load(ws.value)
    router.push({ name: 'ws-page', params: { ws: ws.value, page: p.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="pages-view">
    <header class="mb-6 flex flex-wrap items-end gap-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.pages') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }} · {{ (pages.byWs[ws] ?? []).length }}</p>
      </div>
      <button v-if="editable" type="button" class="btn btn-ghost" data-testid="open-changes" @click="changesOpen = true"><Undo2 class="h-4 w-4" />{{ t('changes.title') }}</button>
      <RouterLink :to="{ name: 'ws-graph', params: { ws } }" class="btn btn-ghost" data-testid="open-graph"><Waypoints class="h-4 w-4" />{{ t('graph.title') }}</RouterLink>
      <button v-if="editable" class="btn-primary" @click="create"><Plus class="h-4 w-4" />{{ t('pages.new') }}</button>
    </header>

    <div class="mb-5 flex flex-wrap items-center gap-2">
      <label class="search h-9 min-w-[220px] flex-1 sm:max-w-xs">
        <Search class="h-4 w-4" />
        <input v-model="q" :placeholder="t('nav.search')" />
      </label>
      <div class="seg">
        <button :class="{ on: sort === 'recent' }" @click="sort = 'recent'">{{ t('pages.sortRecent') }}</button>
        <button :class="{ on: sort === 'title' }" @click="sort = 'title'">{{ t('pages.sortTitle') }}</button>
      </div>
      <div class="seg">
        <button :class="{ on: layout === 'grid' }" :aria-label="'grid'" @click="layout = 'grid'"><LayoutGrid class="h-4 w-4" /></button>
        <button :class="{ on: layout === 'list' }" :aria-label="'list'" @click="layout = 'list'"><List class="h-4 w-4" /></button>
      </div>
      <span class="ml-auto" />
      <template v-if="editable">
        <button class="btn btn-ghost" :title="t('pages.importHint')" data-testid="pages-import" @click="importing = true"><Upload class="h-4 w-4" />{{ t('pages.importMd') }}</button>
      </template>
      <button class="btn btn-ghost" @click="exportAll"><Download class="h-4 w-4" />{{ t('pages.exportAll') }}</button>
      <RouterLink :to="{ name: 'ws-trash', params: { ws } }" class="btn btn-ghost"><Trash2 class="h-4 w-4" />{{ t('pages.trash') }}</RouterLink>
    </div>

    <div v-if="tagCounts.length" class="tag-bar" data-testid="tag-bar">
      <button v-for="x in tagCounts" :key="x.tag" type="button" class="tag-pill" :class="{ on: x.tag.toLowerCase() === tag.toLowerCase() }" :aria-pressed="x.tag.toLowerCase() === tag.toLowerCase()" data-testid="tag-pill" @click="pickTag(x.tag)">
        #{{ x.tag }} <span class="n">{{ x.n }}</span>
      </button>
    </div>

    <div v-if="list.length && layout === 'grid'" class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-6">
      <RouterLink v-for="p in list" :key="p.id" :to="{ name: 'ws-page', params: { ws, page: p.id } }" class="tile-card card">
        <div class="thumb">
          <span v-if="p.icon" class="text-[34px]">{{ p.icon }}</span>
          <FileText v-else class="h-8 w-8 text-text-faint" :stroke-width="1.5" />
        </div>
        <div class="p-3">
          <div class="flex items-center gap-1">
            <span class="truncate text-[14px] font-semibold">{{ p.title || t('pages.untitled') }}</span>
            <Star v-if="p.favorite" class="h-3 w-3 shrink-0 text-warn" fill="currentColor" />
          </div>
          <div class="truncate text-[12px] text-text-3">{{ parentTitle(p) ? parentTitle(p) + ' · ' : '' }}{{ relDay(p.updated_at.slice(0, 10)) }}</div>
          <div v-if="p.tags?.length" class="mt-1 truncate text-[11.5px] text-text-3">{{ p.tags.map((x) => '#' + x).join(' ') }}</div>
        </div>
      </RouterLink>
    </div>

    <div v-else-if="list.length" class="group">
      <RouterLink v-for="p in list" :key="p.id" :to="{ name: 'ws-page', params: { ws, page: p.id } }" class="group-row hover:bg-surface-hover">
        <span class="w-6 text-center text-[17px]"><template v-if="p.icon">{{ p.icon }}</template><FileText v-else class="mx-auto h-4 w-4 text-text-3" /></span>
        <span class="min-w-0 flex-1 truncate text-[14.5px]">{{ p.title || t('pages.untitled') }}</span>
        <span class="hidden truncate text-[12.5px] text-text-3 sm:block">{{ parentTitle(p) }}</span>
        <span class="w-28 shrink-0 text-right text-[12.5px] text-text-3">{{ relDay(p.updated_at.slice(0, 10)) }}</span>
      </RouterLink>
    </div>

    <div v-else class="empty">
      <FileText class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
      <p class="title-3">{{ q ? t('cmd.nothing') : t('pages.empty') }}</p>
      <p v-if="!q" class="subhead mx-auto mt-1 max-w-sm">{{ t('pages.emptyHint') }}</p>
      <button v-if="editable && !q" class="btn-primary mt-4" @click="create"><Plus class="h-4 w-4" />{{ t('pages.new') }}</button>
    </div>
    <ImportSheet :open="importing" :ws="ws" @close="importing = false" />
    <RecentChanges :open="changesOpen" :ws="ws" @close="changesOpen = false" />
  </div>
</template>

<style scoped>
.tag-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: -6px 0 16px;
}
.tag-pill {
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--fill-2);
  color: var(--text-2);
  font-size: 12.5px;
}
.tag-pill .n {
  color: var(--text-3);
  font-size: 11.5px;
}
.tag-pill.on {
  background: var(--accent);
  color: var(--accent-fg);
}
.tag-pill.on .n {
  color: inherit;
  opacity: 0.8;
}
.pages-view {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
}
.tile-card {
  overflow: hidden;
  transition: transform 0.18s var(--spring), box-shadow 0.18s;
}
.tile-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow);
}
.thumb {
  display: grid;
  place-items: center;
  height: 92px;
  background: linear-gradient(160deg, color-mix(in srgb, var(--ws) 14%, var(--surface-2)), var(--surface-2));
  border-bottom: 0.5px solid var(--border);
}
.empty {
  padding: 72px 16px;
  text-align: center;
}
</style>

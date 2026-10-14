<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Plus, Search, FileText, Trash2, LayoutGrid, List, Star } from 'lucide-vue-next'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { pagesApi, type PageSummary } from '@/api/planner'
import { relDay } from '@/lib/dates'

// A workspace's pages at a glance: a grid (or list) of every page, newest
// edits first or by title, with a filter. The sidebar tree is for browsing
// the hierarchy; this is for finding.
const route = useRoute()
const router = useRouter()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
watch(ws, () => pages.load(ws.value).catch(() => {}))
onMounted(() => pages.load(ws.value).catch(() => {}))

const q = ref('')
const sort = ref<'recent' | 'title'>((localStorage.getItem('orbit_pages_sort') as 'recent' | 'title') || 'recent')
const layout = ref<'grid' | 'list'>((localStorage.getItem('orbit_pages_layout') as 'grid' | 'list') || 'grid')
watch(sort, (v) => localStorage.setItem('orbit_pages_sort', v))
watch(layout, (v) => localStorage.setItem('orbit_pages_layout', v))

const list = computed<PageSummary[]>(() => {
  const all = [...(pages.byWs[ws.value] ?? [])]
  const ql = q.value.trim().toLowerCase()
  const shown = ql ? all.filter((p) => (p.title || t('pages.untitled')).toLowerCase().includes(ql)) : all
  return shown.sort((a, b) => (sort.value === 'title' ? (a.title || '').localeCompare(b.title || '') : b.updated_at.localeCompare(a.updated_at)))
})
const parentTitle = (p: PageSummary) => (p.parent_id ? pages.find(p.parent_id)?.title : '')

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
      <RouterLink :to="{ name: 'ws-trash', params: { ws } }" class="btn btn-ghost ml-auto"><Trash2 class="h-4 w-4" />{{ t('pages.trash') }}</RouterLink>
    </div>

    <div v-if="list.length && layout === 'grid'" class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4">
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
  </div>
</template>

<style scoped>
.pages-view {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
  max-width: 1180px;
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

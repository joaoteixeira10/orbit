<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Columns3, FileText, History, Plus, RotateCcw, Search, X } from 'lucide-vue-next'
import Avatar from '@/components/ui/Avatar.vue'
import Confirm from '@/components/ui/Confirm.vue'
import Popover from '@/components/ui/Popover.vue'
import DiffView from './DiffView.vue'
import { pagesApi, type PageVersion } from '@/api/planner'
import { changeCount, diffMarkdown } from '@/lib/diff'
import { locale } from '@/lib/dates'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// A page's history, over the page: every saved version (one person's saves
// close together are one), newest first, with who saved it; one picked
// shows as it read then, what changed since the one before marked. An
// editor puts one back (Restore). Compare: up to five versions — or other
// pages as they are now — side by side, each marked against the first.
const props = defineProps<{ open: boolean; ws: string; pageId: string; title: string; editable: boolean }>()
const emit = defineEmits<{ close: []; restore: [versionId: string] }>()
const { t } = useI18n()
const wss = useWorkspacesStore()
const pages = usePagesStore()
const toast = useToastStore()
const loc = locale()

const versions = ref<PageVersion[]>([])
const loading = ref(false)
const texts = ref<Record<string, string>>({}) // a version's (or a page's) text, once read
async function load() {
  loading.value = true
  try {
    versions.value = await pagesApi.versions(props.pageId)
    selected.value = versions.value[0]?.id ?? ''
    picked.value = []
    others.value = []
    mode.value = 'one'
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loading.value = false
  }
}
watch(
  () => props.open && props.pageId,
  (o) => {
    if (o) void load()
    document.body.style.overflow = props.open ? 'hidden' : ''
  },
  { immediate: true },
)
onUnmounted(() => (document.body.style.overflow = ''))
async function textOf(id: string, page = false): Promise<string> {
  if (texts.value[id] === undefined) {
    const x = page ? await pagesApi.get(id) : await pagesApi.version(id)
    texts.value = { ...texts.value, [id]: x.content ?? '' }
  }
  return texts.value[id]
}

const who = (uid: string) => {
  const m = (wss.members[props.ws] ?? []).find((x) => x.user_id === uid)
  return m ? m.display_name || m.username : t('editor.someone')
}
const day = (iso: string) => new Intl.DateTimeFormat(loc, { weekday: 'short', day: 'numeric', month: 'long', year: 'numeric' }).format(new Date(iso))
const time = (iso: string) => new Intl.DateTimeFormat(loc, { hour: '2-digit', minute: '2-digit' }).format(new Date(iso))
const groups = computed(() => {
  const out: { day: string; items: PageVersion[] }[] = []
  for (const v of versions.value) {
    const d = day(v.updated_at)
    if (out[out.length - 1]?.day !== d) out.push({ day: d, items: [] })
    out[out.length - 1].items.push(v)
  }
  return out
})

// ---- One version ----
const selected = ref('')
const changes = ref(true)
const idx = computed(() => versions.value.findIndex((v) => v.id === selected.value))
const current = computed(() => versions.value[idx.value])
const before = computed(() => versions.value[idx.value + 1])
const shown = ref<{ after: string; before: string | null } | null>(null)
watch(
  [selected, changes, () => versions.value.length],
  async () => {
    const v = current.value
    if (!v) return (shown.value = null)
    try {
      const after = await textOf(v.id)
      const prev = changes.value && before.value ? await textOf(before.value.id) : null
      if (v.id === selected.value) shown.value = { after, before: prev }
    } catch (e: any) {
      toast.error(e.message)
    }
  },
  { immediate: true },
)
const counts = computed(() => (shown.value && shown.value.before != null ? changeCount(diffMarkdown(shown.value.before, shown.value.after)) : null))
const confirming = ref(false)

// ---- Compare ----
const MAX = 5
const mode = ref<'one' | 'compare'>('one')
const picked = ref<string[]>([]) // versions
const others = ref<{ id: string; title: string }[]>([]) // pages as they are now
const full = computed(() => picked.value.length + others.value.length >= MAX)
function togglePick(id: string) {
  if (picked.value.includes(id)) picked.value = picked.value.filter((x) => x !== id)
  else if (!full.value) picked.value = [...picked.value, id]
}
const pageEl = ref<HTMLElement | null>(null)
const pagePicking = ref(false)
const pageQ = ref('')
const pageChoices = computed(() => {
  const q = pageQ.value.trim().toLowerCase()
  return (pages.byWs[props.ws] ?? [])
    .filter((p) => !others.value.some((o) => o.id === p.id) && (!q || p.title.toLowerCase().includes(q)))
    .slice(0, 30)
})
function addPage(p: { id: string; title: string }) {
  pagePicking.value = false
  if (!full.value) others.value = [...others.value, { id: p.id, title: p.title || t('pages.untitled') }]
  mode.value = 'compare'
}
const columns = ref<{ key: string; label: string; sub: string; text: string }[]>([])
watch(
  [mode, picked, others],
  async () => {
    if (mode.value !== 'compare') return
    try {
      const vs = versions.value.filter((v) => picked.value.includes(v.id)).reverse() // oldest first
      const cols = []
      for (const v of vs) cols.push({ key: v.id, label: v.title || props.title || t('pages.untitled'), sub: `${day(v.updated_at)} ${time(v.updated_at)} · ${who(v.user_id)}`, text: await textOf(v.id) })
      for (const p of others.value) cols.push({ key: p.id, label: p.title, sub: t('history.now'), text: await textOf(p.id, true) })
      columns.value = cols
    } catch (e: any) {
      toast.error(e.message)
    }
  },
  { deep: true },
)
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div v-if="open" class="hist-backdrop" data-testid="page-history">
        <div class="hist-panel" role="dialog" aria-modal="true" :aria-label="t('history.title')">
          <header class="hist-head">
            <History class="h-5 w-5 text-text-3" />
            <div class="min-w-0 flex-1">
              <h2 class="title-3 truncate">{{ t('history.title') }}</h2>
              <p class="truncate text-[12.5px] text-text-3">{{ title || t('pages.untitled') }}</p>
            </div>
            <div class="seg shrink-0" role="group">
              <button type="button" :class="{ on: mode === 'one' }" @click="mode = 'one'">{{ t('history.one') }}</button>
              <button type="button" :class="{ on: mode === 'compare' }" data-testid="history-compare" :disabled="picked.length + others.length < 2" @click="mode = 'compare'">
                <Columns3 class="mr-1 inline h-3.5 w-3.5" />{{ t('history.compare', { n: picked.length + others.length }) }}
              </button>
            </div>
            <button type="button" class="icon-btn h-8 w-8 rounded-full bg-fill" :aria-label="t('common.close')" data-testid="history-close" @click="emit('close')"><X class="h-4 w-4" /></button>
          </header>

          <div class="hist-body">
            <!-- The versions -->
            <aside class="hist-list">
              <p v-if="loading" class="p-4 text-[13px] text-text-3">{{ t('common.loading') }}</p>
              <p v-else-if="!versions.length" class="p-4 text-[13px] text-text-3">{{ t('history.none') }}</p>
              <template v-for="g in groups" :key="g.day">
                <h3 class="cap px-3 pb-1 pt-3">{{ g.day }}</h3>
                <div
                  v-for="v in g.items"
                  :key="v.id"
                  class="v-row"
                  :class="{ on: v.id === selected && mode === 'one' }"
                  role="button"
                  tabindex="0"
                  data-testid="history-version"
                  @click="(selected = v.id), (mode = 'one')"
                  @keydown.enter="(selected = v.id), (mode = 'one')"
                >
                  <input
                    type="checkbox"
                    class="shrink-0"
                    :checked="picked.includes(v.id)"
                    :disabled="full && !picked.includes(v.id)"
                    :aria-label="t('history.pick')"
                    data-testid="history-pick"
                    @click.stop
                    @change="togglePick(v.id)"
                  />
                  <Avatar :name="who(v.user_id)" :size="22" />
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-1.5 text-[13px] font-medium">
                      <span>{{ time(v.updated_at) }}</span>
                      <span v-if="v === versions[0]" class="badge">{{ t('history.current') }}</span>
                      <span v-else-if="v.restored" class="badge">{{ t('history.restoredFrom', { n: v.restored }) }}</span>
                    </div>
                    <div class="truncate text-[12px] text-text-3">{{ who(v.user_id) }}</div>
                  </div>
                </div>
              </template>
              <div class="p-3">
                <button ref="pageEl" type="button" class="btn btn-sm w-full" :disabled="full" data-testid="history-add-page" @click="pagePicking = true"><Plus class="h-3.5 w-3.5" />{{ t('history.addPage') }}</button>
                <div v-if="others.length" class="mt-2 flex flex-wrap gap-1">
                  <span v-for="o in others" :key="o.id" class="chip"><FileText class="h-3 w-3" />{{ o.title }}<button type="button" :aria-label="t('editor.remove')" @click="others = others.filter((x) => x.id !== o.id)"><X class="h-3 w-3" /></button></span>
                </div>
              </div>
            </aside>

            <!-- One version, as it read -->
            <section v-if="mode === 'one'" class="hist-main">
              <template v-if="current && shown">
                <div class="hist-tools">
                  <div class="min-w-0 flex-1 text-[13px]">
                    <span class="font-semibold">{{ day(current.updated_at) }}, {{ time(current.updated_at) }}</span>
                    <span class="text-text-3"> · {{ who(current.user_id) }}</span>
                    <span v-if="counts" class="ml-2 text-[12px]" data-testid="history-counts"
                      ><span class="text-[#248a3d]">+{{ counts.added }}</span> <span class="text-[#d70015]">−{{ counts.removed }}</span></span
                    >
                  </div>
                  <label v-if="before" class="flex items-center gap-1.5 text-[12.5px] text-text-2"><input v-model="changes" type="checkbox" data-testid="history-changes" />{{ t('history.changes') }}</label>
                  <button v-if="editable && idx > 0" type="button" class="btn-primary btn-sm" data-testid="history-restore" @click="confirming = true"><RotateCcw class="h-3.5 w-3.5" />{{ t('history.restore') }}</button>
                </div>
                <h1 v-if="current.title" class="mb-2 text-[24px] font-bold">{{ current.title }}</h1>
                <DiffView :after="shown.after" :before="shown.before" />
              </template>
            </section>

            <!-- Side by side -->
            <section v-else class="hist-cols" data-testid="history-columns">
              <div v-for="(c, i) in columns" :key="c.key" class="h-col">
                <div class="h-col-head">
                  <div class="truncate text-[13px] font-semibold">{{ c.label }}</div>
                  <div class="truncate text-[11.5px] text-text-3">{{ i === 0 ? t('history.base') : c.sub }}</div>
                </div>
                <DiffView :after="c.text" :before="i === 0 ? null : columns[0].text" />
              </div>
            </section>
          </div>
        </div>

        <Popover v-if="pagePicking" :anchor="pageEl" :width="280" placement="top-start" @close="pagePicking = false">
          <div class="p-2">
            <label class="relative mb-1 block">
              <Search class="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-3" />
              <input v-model="pageQ" class="field !pl-8 text-[13px]" :placeholder="t('history.findPage')" />
            </label>
            <div class="max-h-64 overflow-y-auto">
              <button v-for="p in pageChoices" :key="p.id" type="button" class="menu-item w-full" data-testid="history-page" @click="addPage(p)">
                <span v-if="p.icon" class="w-4 text-center">{{ p.icon }}</span>
                <FileText v-else class="h-4 w-4 text-text-3" />
                <span class="truncate">{{ p.title || t('pages.untitled') }}</span>
              </button>
            </div>
          </div>
        </Popover>
        <Confirm
          :open="confirming"
          :title="t('history.restoreTitle')"
          :message="t('history.restoreBody')"
          :confirm-label="t('history.restore')"
          @confirm="(confirming = false), emit('restore', selected)"
          @close="confirming = false"
        />
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.hist-backdrop {
  position: fixed;
  inset: 0;
  z-index: 50; /* as dialogs: one opened from here comes later, on top */
  display: flex;
  padding: clamp(0px, 2.5vw, 28px);
  background: var(--backdrop);
}
.hist-panel {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border-radius: 16px;
  background: var(--bg);
  box-shadow: 0 12px 48px rgba(0, 0, 0, 0.25);
}
.hist-head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 0.5px solid var(--border);
}
.hist-body {
  display: flex;
  flex: 1;
  min-height: 0;
}
.hist-list {
  width: 270px;
  flex-shrink: 0;
  overflow-y: auto;
  border-right: 0.5px solid var(--border);
  background: var(--grouped, var(--fill-2));
}
.v-row {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 1px 6px;
  padding: 6px 8px;
  border-radius: 8px;
  cursor: pointer;
}
.v-row:hover {
  background: var(--fill-2);
}
.v-row.on {
  background: var(--accent-bg, var(--fill));
}
.badge {
  padding: 0 6px;
  border-radius: 999px;
  background: var(--fill);
  font-size: 10.5px;
  font-weight: 500;
  color: var(--text-2);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--fill);
  font-size: 11.5px;
}
.hist-main {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 16px 28px 40px;
}
.hist-tools {
  position: sticky;
  top: -16px;
  z-index: 1;
  display: flex;
  align-items: center;
  gap: 10px;
  margin: -16px -28px 12px;
  padding: 10px 28px;
  border-bottom: 0.5px solid var(--border);
  background: color-mix(in srgb, var(--bg) 92%, transparent);
  backdrop-filter: blur(8px);
}
.hist-cols {
  display: flex;
  flex: 1;
  min-width: 0;
  gap: 1px;
  overflow-x: auto;
  background: var(--border);
}
.h-col {
  min-width: 300px;
  flex: 1;
  overflow-y: auto;
  padding: 0 14px 30px;
  background: var(--bg);
}
.h-col-head {
  position: sticky;
  top: 0;
  z-index: 1;
  margin: 0 -14px 8px;
  padding: 10px 14px;
  border-bottom: 0.5px solid var(--border);
  background: color-mix(in srgb, var(--bg) 94%, transparent);
  backdrop-filter: blur(8px);
}
@media (max-width: 760px) {
  .hist-panel {
    border-radius: 0;
  }
  .hist-body {
    flex-direction: column;
  }
  .hist-list {
    width: auto;
    max-height: 36vh;
    border-right: 0;
    border-bottom: 0.5px solid var(--border);
  }
}
</style>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Columns3, Download, FileSpreadsheet, FileText, Link, LoaderCircle, MoreHorizontal, Plus, Table2, Trash2, Upload, ClipboardList } from 'lucide-vue-next'
import FormsSheet from '@/components/forms/FormsSheet.vue'
import Popover from '@/components/ui/Popover.vue'
import DbCell from './DbCell.vue'
import PropMenu from './PropMenu.vue'
import ViewTools from './ViewTools.vue'
import { pagesApi } from '@/api/planner'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { todayStr } from '@/lib/dates'
import { aggsFor, boardOf, newId, rowsOf, total, typeOf, withFormulas, type Agg, type Database, type DBOption, type DBProp, type DBRow, type DBView, type PropValue } from '@/lib/database'
import { locale } from '@/lib/dates'
import { fileName } from '@/lib/markdown'
import { isGoogleSheet, sheetsApi } from '@/lib/sheetsApi'
import type { SheetData } from '@/lib/sheet'

// A database page's table: its rows (the pages inside it) with a column per
// property, through the chosen view — a table or a board by a select
// property, filtered and sorted. Values are edited in place; a row's name
// opens its page. Everything is saved at once (and seen by the others).
const props = defineProps<{ ws: string; pageId: string; editable: boolean }>()
const { t } = useI18n()
const router = useRouter()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()

const page = computed(() => pages.find(props.pageId))
const db = computed<Database>(() => page.value?.db ?? { props: [], views: [] })
const formsOpen = ref(false)
const members = computed(() => wss.members[props.ws] ?? [])
const names = computed(() => new Map(members.value.map((m) => [m.user_id, m.display_name || m.username])))
// The rows, their formulas worked out (so filters and sorts see them).
const rows = computed<DBRow[]>(() =>
  withFormulas(
    db.value,
    (pages.byWs[props.ws] ?? [])
      .filter((p) => p.parent_id === props.pageId && !p.trashed_at)
      .map((p) => ({ id: p.id, title: p.title, rank: p.rank, props: p.props ?? {} })),
    names.value,
    todayStr(),
  ),
)
const ctx = computed(() => ({ me: auth.user?.id ?? '', today: todayStr() }))

async function saveDB(next: Database) {
  try {
    await pages.saveDB(props.pageId, next)
  } catch (e: any) {
    toast.error(e.message)
  }
}
const clone = (): Database => JSON.parse(JSON.stringify(db.value))

// ---- Views (the chosen one remembered per browser) ----
const key = computed(() => `orbit_db_view_${props.pageId}`)
const viewId = ref('')
watch(
  () => props.pageId,
  () => {
    try {
      viewId.value = localStorage.getItem(key.value) ?? ''
    } catch {
      viewId.value = ''
    }
  },
  { immediate: true },
)
const views = computed<DBView[]>(() => (db.value.views.length ? db.value.views : [{ id: '_', name: t('db.table'), type: 'table' }]))
const view = computed(() => views.value.find((v) => v.id === viewId.value) ?? views.value[0])
function pickView(id: string) {
  viewId.value = id
  try {
    localStorage.setItem(key.value, id)
  } catch {
    /* private mode */
  }
}
function saveView(v: DBView) {
  const d = clone()
  const i = d.views.findIndex((x) => x.id === v.id)
  if (i >= 0) d.views[i] = v
  else d.views.push(v.id === '_' ? { ...v, id: newId() } : v)
  void saveDB(d)
}
function addView(type: 'table' | 'board') {
  const d = clone()
  const group = d.props.find((p) => p.type === 'select')?.id
  const v: DBView = { id: newId(), name: type === 'board' ? t('db.board') : t('db.table'), type, ...(type === 'board' && group ? { group_by: group } : {}) }
  if (!d.views.length) d.views.push({ id: newId(), name: t('db.table'), type: 'table' })
  d.views.push(v)
  void saveDB(d).then(() => pickView(v.id))
  addOpen.value = false
}
const addAnchor = ref<HTMLElement | null>(null)
const addOpen = ref(false)
const viewMenu = ref<{ anchor: HTMLElement; id: string } | null>(null)
const viewName = ref('')
function openViewMenu(id: string, e: MouseEvent) {
  viewMenu.value = { anchor: e.currentTarget as HTMLElement, id }
  viewName.value = views.value.find((v) => v.id === id)?.name ?? ''
}
function closeViewMenu() {
  const m = viewMenu.value
  viewMenu.value = null
  const v = m && views.value.find((x) => x.id === m.id)
  if (v && viewName.value.trim() && viewName.value.trim() !== v.name) saveView({ ...v, name: viewName.value.trim().slice(0, 100) })
}
function deleteView(id: string) {
  const d = clone()
  d.views = d.views.filter((v) => v.id !== id)
  viewMenu.value = null
  void saveDB(d)
}

const shown = computed(() => rowsOf(db.value, view.value, rows.value, ctx.value))
const columns = computed(() => db.value.props.filter((p) => !(view.value.hidden ?? []).includes(p.id)))
const board = computed(() => (view.value.type === 'board' ? boardOf(db.value, view.value, shown.value, t('db.noValue')) : []))
const selects = computed(() => db.value.props.filter((p) => p.type === 'select'))

// ---- Totals (the table's last row; each view keeps its own) ----
const totals = computed(() => view.value.totals ?? {})
const hasTotals = computed(() => Object.keys(totals.value).length > 0)
function setTotal(prop: string, agg: string) {
  const next = { ...totals.value }
  if (agg) next[prop] = agg as Agg
  else delete next[prop]
  saveView({ ...view.value, totals: next })
}
function totalText(prop: string): string {
  const agg = totals.value[prop]
  if (!agg) return ''
  const v = total(shown.value, prop, agg)
  return v === null ? '—' : new Intl.NumberFormat(locale(), { maximumFractionDigits: 2 }).format(v)
}

// ---- Properties ----
const propMenu = ref<{ anchor: HTMLElement; prop: DBProp; isNew: boolean } | null>(null)
function openProp(p: DBProp, e: MouseEvent) {
  if (props.editable) propMenu.value = { anchor: e.currentTarget as HTMLElement, prop: p, isNew: false }
}
function newProp(e: MouseEvent) {
  propMenu.value = { anchor: e.currentTarget as HTMLElement, prop: { id: newId(), name: '', type: 'text' }, isNew: true }
}
function saveProp(p: DBProp) {
  const d = clone()
  const i = d.props.findIndex((x) => x.id === p.id)
  if (i >= 0) d.props[i] = p
  else d.props.push(p)
  void saveDB(d)
}
function hideProp(id: string) {
  saveView({ ...view.value, hidden: [...new Set([...(view.value.hidden ?? []), id])] })
}
function removeProp(id: string) {
  const d = clone()
  d.props = d.props.filter((p) => p.id !== id)
  for (const v of d.views) {
    v.filters = v.filters?.filter((f) => f.prop !== id)
    v.sorts = v.sorts?.filter((s) => s.prop !== id)
    v.hidden = v.hidden?.filter((h) => h !== id)
    if (v.group_by === id) delete v.group_by
  }
  void saveDB(d)
}
function addOption(propId: string, o: DBOption) {
  const d = clone()
  const p = d.props.find((x) => x.id === propId)
  if (!p) return
  p.options = [...(p.options ?? []), o]
  void saveDB(d)
}

// ---- Rows ----
async function setValue(row: DBRow, prop: string, v: PropValue | null) {
  try {
    await pages.setProp(row.id, prop, v)
  } catch (e: any) {
    toast.error(e.message)
  }
}
const draft = ref('')
const adding = ref<string | null>(null) // the board column (option id, "" none) or "table"
const addInput = ref<HTMLInputElement | HTMLInputElement[] | null>(null)
async function startAdd(where: string) {
  adding.value = where
  draft.value = ''
  await nextTick()
  ;(Array.isArray(addInput.value) ? addInput.value[0] : addInput.value)?.focus()
}
async function addRow(group?: string) {
  const title = draft.value.trim()
  draft.value = ''
  if (!title) {
    adding.value = null
    return
  }
  // A row made in a board column, or under a filter "is", gets that value.
  const values: Record<string, PropValue> = {}
  if (group && view.value.group_by) values[view.value.group_by] = group
  for (const f of view.value.filters ?? []) {
    if (f.op === 'is' && f.prop !== 'title' && f.value !== undefined && f.value !== 'me') values[f.prop] = f.value as PropValue
    if (f.op === 'checked') values[f.prop] = true
  }
  try {
    await pagesApi.create(props.ws, { parent_id: props.pageId, title, props: Object.keys(values).length ? values : undefined })
    await pages.load(props.ws)
  } catch (e: any) {
    toast.error(e.message)
  }
}
const open = (id: string) => router.push({ name: 'ws-page', params: { ws: props.ws, page: id } })

// ---- Spreadsheets: the view's rows out (Excel, CSV), rows in (an Excel
// file, a CSV, a Google Sheets link) ----
const fileMenu = ref(false)
const fileAnchor = ref<HTMLElement | null>(null)
const fileEl = ref<HTMLInputElement | null>(null)
const googleLink = ref('')
const googleOpen = ref(false)
const googleEl = ref<HTMLInputElement | null>(null)
async function openGoogle() {
  googleOpen.value = true
  await nextTick()
  googleEl.value?.focus()
}
const progress = ref<{ done: number; total: number } | null>(null)
async function download(kind: 'xlsx' | 'csv') {
  fileMenu.value = false
  const [{ sheetOf }, { csvOf, writeXlsx }, { Book, addr }, { dayOf }] = await Promise.all([import('@/lib/dbSheet'), import('@/lib/xlsx'), import('@/lib/sheet'), import('@/lib/formula')])
  const title = page.value?.title || t('db.database')
  const data = sheetOf(shown.value, columns.value, names.value, { name: t('db.name'), tab: title })
  const tab = data.tabs[0]
  const blob =
    kind === 'xlsx'
      ? new Blob([writeXlsx(data, new Book(data, dayOf(todayStr()), false)) as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
      : new Blob([csvOf(tab, (r, c) => (tab.cells[addr(r, c)]?.v ?? '').replace(/^'/, ''))], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = Object.assign(document.createElement('a'), { href: url, download: `${fileName(title)}.${kind}` })
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function fromFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  ;(e.target as HTMLInputElement).value = ''
  if (!f) return
  fileMenu.value = false
  await bringIn(async () => {
    const { readCsv, readXlsx } = await import('@/lib/xlsx')
    return /\.xlsx$/i.test(f.name) ? readXlsx(new Uint8Array(await f.arrayBuffer())) : readCsv(await f.text(), 'CSV')
  })
}
async function fromGoogle() {
  const link = googleLink.value.trim()
  if (!isGoogleSheet(link)) {
    toast.error(t('db.sheet.notGoogle'))
    return
  }
  fileMenu.value = false
  googleOpen.value = false
  googleLink.value = ''
  await bringIn(async () => {
    const { readXlsx } = await import('@/lib/xlsx')
    return readXlsx((await sheetsApi.google(props.ws, link)).bytes)
  })
}
// bringIn adds a spreadsheet's rows: its new columns become properties
// first, then a page each, in order.
async function bringIn(read: () => Promise<SheetData>) {
  if (progress.value) return
  progress.value = { done: 0, total: 0 }
  try {
    const { gridOf, planImport, MAX_IMPORT_ROWS } = await import('@/lib/dbSheet')
    const grid = gridOf(await read(), locale().startsWith('pt'), todayStr())
    if (!grid.rows.length) {
      toast.info(t('db.sheet.empty'))
      return
    }
    const people = members.value.map((m) => ({ id: m.user_id, names: [m.display_name, m.username] }))
    const plan = planImport(db.value, grid, people, [t('db.name'), 'Name', 'Nome', 'Title', 'Título'])
    if (JSON.stringify(plan.db) !== JSON.stringify(db.value)) await pages.saveDB(props.pageId, plan.db)
    progress.value = { done: 0, total: plan.rows.length }
    for (const r of plan.rows) {
      await pagesApi.create(props.ws, { parent_id: props.pageId, title: r.title, props: Object.keys(r.props).length ? r.props : undefined })
      progress.value = { done: progress.value.done + 1, total: plan.rows.length }
    }
    await pages.load(props.ws)
    toast.success(t('db.sheet.imported', { n: plan.rows.length, props: plan.added.length }, plan.rows.length))
    if (grid.rows.length > MAX_IMPORT_ROWS) toast.info(t('db.sheet.capped', { n: MAX_IMPORT_ROWS }))
  } catch (e: any) {
    await pages.load(props.ws).catch(() => undefined)
    toast.error(t('db.sheet.failed', { error: e?.message ?? String(e) }))
  } finally {
    progress.value = null
  }
}

// Board: drag a card to another column.
const dragging = ref('')
function drop(col: string) {
  const r = rows.value.find((x) => x.id === dragging.value)
  dragging.value = ''
  if (r && view.value.group_by && (r.props[view.value.group_by] ?? '') !== col) void setValue(r, view.value.group_by, col || null)
}
</script>

<template>
  <section class="db" data-testid="database">
    <!-- Views and tools -->
    <div class="db-bar">
      <div class="db-views" role="tablist" :aria-label="t('db.views')">
        <button
          v-for="v in views"
          :key="v.id"
          type="button"
          role="tab"
          class="db-view"
          :class="{ on: v.id === view.id }"
          :aria-selected="v.id === view.id"
          @click="pickView(v.id)"
          @dblclick="editable && v.id !== '_' && openViewMenu(v.id, $event)"
          @contextmenu.prevent="editable && v.id !== '_' && openViewMenu(v.id, $event)"
        >
          <component :is="v.type === 'board' ? Columns3 : Table2" class="h-3.5 w-3.5" />{{ v.name }}
        </button>
        <button v-if="editable" ref="addAnchor" type="button" class="icon-btn h-7 w-7" :title="t('db.addView')" :aria-label="t('db.addView')" data-testid="db-add-view" @click="addOpen = !addOpen"><Plus class="h-4 w-4" /></button>
      </div>
      <div class="flex items-center gap-1.5">
        <button v-if="editable && view.id !== '_'" type="button" class="icon-btn h-7 w-7" :aria-label="t('db.viewOptions')" @click="openViewMenu(view.id, $event)"><MoreHorizontal class="h-4 w-4" /></button>
        <span v-if="progress" class="flex items-center gap-1 text-[12px] text-text-3" data-testid="db-importing"><LoaderCircle class="h-3.5 w-3.5 animate-spin" />{{ progress.total ? t('db.sheet.progress', { done: progress.done, total: progress.total }) : t('db.sheet.reading') }}</span>
        <button ref="fileAnchor" type="button" class="icon-btn h-7 w-7" :title="t('db.sheet.menu')" :aria-label="t('db.sheet.menu')" data-testid="db-sheet-menu" @click="fileMenu = !fileMenu"><FileSpreadsheet class="h-4 w-4" /></button>
        <button type="button" class="icon-btn h-7 w-7" :title="t('forms.title')" :aria-label="t('forms.title')" data-testid="db-forms" @click="formsOpen = true"><ClipboardList class="h-4 w-4" /></button>
        <ViewTools :db="db" :view="view" :editable="editable" :members="members" @update="saveView" />
        <button v-if="editable" type="button" class="btn-primary btn-sm" data-testid="db-new-row" @click="startAdd(view.type === 'board' ? board[0]?.key ?? '' : 'table')"><Plus class="h-3.5 w-3.5" />{{ t('db.new') }}</button>
      </div>
    </div>

    <Popover v-if="addOpen" :anchor="addAnchor" :width="200" @close="addOpen = false">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full" @click="addView('table')"><Table2 class="h-4 w-4 text-text-2" />{{ t('db.table') }}</button>
        <button type="button" class="menu-item w-full" data-testid="db-add-board" @click="addView('board')"><Columns3 class="h-4 w-4 text-text-2" />{{ t('db.board') }}</button>
      </div>
    </Popover>
    <Popover v-if="fileMenu" :anchor="fileAnchor" :width="290" placement="bottom-end" @close="(fileMenu = false), (googleOpen = false)">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full" data-testid="db-xlsx" @click="download('xlsx')"><Download class="h-4 w-4 text-text-2" />{{ t('db.sheet.downloadXlsx') }}</button>
        <button type="button" class="menu-item w-full" data-testid="db-csv" @click="download('csv')"><Download class="h-4 w-4 text-text-2" />{{ t('db.sheet.downloadCsv') }}</button>
        <template v-if="editable">
          <div class="menu-sep" />
          <input ref="fileEl" type="file" class="hidden" accept=".xlsx,.csv,.tsv,.txt,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" data-testid="db-import-input" @change="fromFile" />
          <button type="button" class="menu-item w-full" :disabled="!!progress" @click="fileEl?.click()"><Upload class="h-4 w-4 text-text-2" />{{ t('db.sheet.importFile') }}</button>
          <button v-if="!googleOpen" type="button" class="menu-item w-full" :disabled="!!progress" data-testid="db-google" @click="openGoogle"><Link class="h-4 w-4 text-text-2" />{{ t('db.sheet.importGoogle') }}</button>
          <form v-else class="flex gap-1.5 px-1 py-1" @submit.prevent="fromGoogle">
            <input v-model="googleLink" ref="googleEl" class="field min-w-0 flex-1 !py-1 text-[13px]" :placeholder="t('db.sheet.googlePlaceholder')" :aria-label="t('db.sheet.importGoogle')" data-testid="db-google-link" />
            <button type="submit" class="btn-primary btn-sm" :disabled="!googleLink.trim()">{{ t('db.sheet.bring') }}</button>
          </form>
          <p class="px-2.5 pb-1 pt-1.5 text-[11.5px] leading-snug text-text-3">{{ t('db.sheet.hint') }}</p>
        </template>
      </div>
    </Popover>
    <Popover v-if="viewMenu" :anchor="viewMenu.anchor" :width="240" @close="closeViewMenu">
      <div class="space-y-2 p-2.5">
        <input v-model="viewName" class="field !py-1.5 text-[13.5px]" :aria-label="t('db.viewName')" maxlength="100" @keydown.enter.prevent="closeViewMenu" />
        <label v-if="views.find((v) => v.id === viewMenu!.id)?.type === 'board'" class="flex items-center gap-2 text-[13px] text-text-2">
          {{ t('db.groupBy') }}
          <select class="input ml-auto !w-auto !py-1 text-[13px]" :value="views.find((v) => v.id === viewMenu!.id)?.group_by ?? ''" @change="saveView({ ...views.find((v) => v.id === viewMenu!.id)!, group_by: ($event.target as HTMLSelectElement).value || undefined })">
            <option value="">—</option>
            <option v-for="p in selects" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </label>
        <button v-if="db.views.length > 1" type="button" class="menu-item danger w-full" @click="deleteView(viewMenu!.id)"><Trash2 class="h-4 w-4" />{{ t('db.deleteView') }}</button>
      </div>
    </Popover>

    <!-- Table -->
    <div v-if="view.type === 'table'" class="db-scroll">
      <table class="db-table">
        <thead>
          <tr>
            <th class="name-col">{{ t('db.name') }}</th>
            <th v-for="p in columns" :key="p.id">
              <button type="button" class="th-btn" :disabled="!editable" data-testid="db-col" @click="openProp(p, $event)">{{ p.name }}</button>
            </th>
            <th v-if="editable" class="add-col">
              <button type="button" class="icon-btn h-6 w-6" :title="t('db.addProp')" :aria-label="t('db.addProp')" data-testid="db-add-prop" @click="newProp"><Plus class="h-4 w-4" /></button>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in shown" :key="r.id" data-testid="db-row">
            <td class="name-col">
              <button type="button" class="row-name" @click="open(r.id)">
                <FileText class="h-3.5 w-3.5 shrink-0 text-text-3" />
                <span class="truncate">{{ r.title || t('pages.untitled') }}</span>
              </button>
            </td>
            <td v-for="p in columns" :key="p.id">
              <DbCell :prop="p" :value="r.props[p.id]" :editable="editable" :members="members" @set="(v) => setValue(r, p.id, v)" @add-option="(o) => addOption(p.id, o)" />
            </td>
            <td v-if="editable" />
          </tr>
          <tr v-if="editable">
            <td :colspan="columns.length + 2" class="add-row">
              <input v-if="adding === 'table'" ref="addInput" v-model="draft" class="field !py-1 text-[13.5px]" :placeholder="t('db.rowName')" maxlength="300" data-testid="db-row-input" @keydown.enter="addRow()" @keydown.esc="adding = null" @blur="draft.trim() ? addRow() : (adding = null)" />
              <button v-else type="button" class="btn-plain !text-[13px]" @click="startAdd('table')"><Plus class="h-4 w-4" />{{ t('db.new') }}</button>
            </td>
          </tr>
        </tbody>
        <tfoot v-if="shown.length && (editable || hasTotals)">
          <tr data-testid="db-totals">
            <td v-for="col in [{ id: 'title', name: t('db.name') }, ...columns]" :key="col.id" class="total-cell">
              <label class="total">
                <span v-if="totals[col.id]" class="total-agg">{{ t(`db.aggs.${totals[col.id]}`) }}</span>
                <span class="total-val">{{ totalText(col.id) }}</span>
                <select
                  v-if="editable"
                  class="total-pick"
                  :value="totals[col.id] ?? ''"
                  :aria-label="t('db.calculate', { name: col.name })"
                  :data-testid="`db-total-${col.id === 'title' ? 'title' : col.name}`"
                  @change="setTotal(col.id, ($event.target as HTMLSelectElement).value)"
                >
                  <option value="">{{ totals[col.id] ? t('db.noTotal') : t('db.calculate', { name: col.name }) }}</option>
                  <option v-for="a in aggsFor(typeOf(db, col.id))" :key="a" :value="a">{{ t(`db.aggs.${a}`) }}</option>
                </select>
              </label>
            </td>
            <td v-if="editable" />
          </tr>
        </tfoot>
      </table>
      <p v-if="!shown.length && rows.length" class="px-2 py-3 text-[13px] text-text-3">{{ t('db.noneMatch') }}</p>
    </div>

    <!-- Board -->
    <div v-else class="db-board">
      <p v-if="!view.group_by" class="px-1 text-[13px] text-text-3">{{ t('db.pickGroup') }}</p>
      <div v-for="c in board" :key="c.key" class="db-col" data-testid="db-board-col" @dragover.prevent @drop.prevent="drop(c.key)">
        <h4 class="db-col-head">
          <span class="h-2.5 w-2.5 rounded-full" :style="{ background: c.color || 'var(--text-faint)' }" />{{ c.name }}
          <span class="ml-auto font-normal text-text-3">{{ c.rows.length }}</span>
        </h4>
        <div
          v-for="r in c.rows"
          :key="r.id"
          class="db-card"
          :draggable="editable"
          data-testid="db-card"
          @dragstart="dragging = r.id"
          @dragend="dragging = ''"
          @click="open(r.id)"
        >
          <div class="text-[13.5px] font-medium">{{ r.title || t('pages.untitled') }}</div>
          <div class="mt-1 flex flex-wrap gap-1">
            <template v-for="p in columns" :key="p.id">
              <DbCell v-if="p.id !== view.group_by && r.props[p.id] !== undefined" :prop="p" :value="r.props[p.id]" :editable="false" :members="members" />
            </template>
          </div>
        </div>
        <input v-if="adding === c.key" ref="addInput" v-model="draft" class="field mt-1 !py-1 text-[13.5px]" :placeholder="t('db.rowName')" maxlength="300" @keydown.enter="addRow(c.key)" @keydown.esc="adding = null" @blur="draft.trim() ? addRow(c.key) : (adding = null)" />
        <button v-else-if="editable" type="button" class="btn-plain mt-1 !text-[13px]" @click="startAdd(c.key)"><Plus class="h-4 w-4" />{{ t('db.new') }}</button>
      </div>
    </div>

    <PropMenu
      v-if="propMenu"
      :anchor="propMenu.anchor"
      :prop="propMenu.prop"
      :is-new="propMenu.isNew"
      :others="[{ id: 'title', name: t('db.name'), type: 'text' }, ...db.props.filter((p) => p.id !== propMenu!.prop.id)]"
      @save="saveProp"
      @hide="hideProp(propMenu.prop.id)"
      @remove="removeProp(propMenu.prop.id)"
      @close="propMenu = null"
    />
      <FormsSheet :open="formsOpen" :ws="ws" :page-id="pageId" :db="db" :editable="editable" @close="formsOpen = false" />
  </section>
</template>

<style scoped>
.db {
  margin: 4px 0 24px;
}
.db-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 0.5px solid var(--border);
}
.db-views {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
}
.db-view {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 9px;
  border-radius: 7px;
  font-size: 13px;
  color: var(--text-2);
}
.db-view:hover {
  background: var(--fill-2);
}
.db-view.on {
  background: var(--fill-2);
  color: var(--text);
  font-weight: 600;
}
.db-scroll {
  overflow-x: auto;
}
.db-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
}
.db-table th {
  padding: 6px 4px;
  border-bottom: 0.5px solid var(--border);
  text-align: left;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-3);
  white-space: nowrap;
}
.db-table td {
  min-width: 120px;
  max-width: 280px;
  padding: 1px 2px;
  border-bottom: 0.5px solid var(--line-soft);
  vertical-align: middle;
}
.db-table .name-col {
  min-width: 200px;
}
.db-table .add-col {
  width: 32px;
  min-width: 0;
}
.th-btn {
  padding: 2px 6px;
  border-radius: 5px;
}
.th-btn:not(:disabled):hover {
  background: var(--fill-2);
  color: var(--text);
}
.row-name {
  display: flex;
  min-width: 0;
  width: 100%;
  align-items: center;
  gap: 6px;
  padding: 5px 6px;
  border-radius: 6px;
  font-weight: 500;
  text-align: left;
}
.row-name:hover {
  background: var(--fill-2);
}
.add-row {
  padding: 6px 4px !important;
}
.total-cell {
  border-bottom: 0 !important;
  border-top: 0.5px solid var(--border);
}
.total {
  position: relative;
  display: flex;
  justify-content: flex-end;
  align-items: baseline;
  gap: 6px;
  min-height: 26px;
  padding: 4px 6px;
  border-radius: 6px;
  font-size: 12.5px;
}
.total:hover {
  background: var(--fill-2);
}
.total-agg {
  font-size: 11px;
  color: var(--text-3);
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.total-val {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.total-pick {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.db-board {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-top: 12px;
}
.db-col {
  flex: 0 0 240px;
  padding: 8px;
  border-radius: 12px;
  background: var(--grouped);
}
.db-col-head {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 2px 4px 8px;
  font-size: 13px;
  font-weight: 600;
}
.db-card {
  margin-bottom: 6px;
  padding: 8px 10px;
  border-radius: 9px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border), var(--shadow-sm);
  cursor: pointer;
}
</style>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlignCenter, AlignLeft, AlignRight, Bold, Italic, Plus, Redo2, Rows3, Undo2 } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import FormulaAssist from '@/components/ui/FormulaAssist.vue'
import { Book, MAX_COLS, MAX_ROWS, addr, newTab, reshape, series, show, toTSV, fromTSV, type Cell, type NumFmt, type SheetData, type Tab } from '@/lib/sheet'
import { colName, dayOf, formatRef, shiftFormula } from '@/lib/formula'
import { locale, todayStr } from '@/lib/dates'
import { usePrefsStore } from '@/stores/prefs'

// A spreadsheet, editable: tabs of cells in a grid that scrolls both ways
// (only what's in view is drawn), a formula bar, a toolbar. It edits like
// Excel:
//   click, drag, ⇧-click or ⇧-arrows select; arrows, Tab, Enter move
//   typing replaces a cell (Enter / F2 / a double click edits it); Esc
//   cancels; in a formula a click on a cell (or a drag) puts its reference in
//   Delete clears; ⌘C ⌘X ⌘V copy and paste (with Excel and Google Sheets
//   too; formulas move their references); ⌘Z ⌘Y undo; ⌘B ⌘I; ⌘A all;
//   ⌘D / ⌘R fill down / right; the square at a selection's corner drags a
//   fill (numbers continue a series); a column's edge resizes it; a row or
//   column header's menu inserts and deletes (references follow).
// data is the sheet (changed in place); change says it changed (to save).
const data = defineModel<SheetData>({ required: true })
const props = defineProps<{ editable: boolean }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const prefs = usePrefsStore() // Sheets may keep their own look (light or dark)

const ROW_H = 26
const HEAD_H = 26
const ROW_HEAD_W = 46
const COL_W = 100
const loc = locale()
const comma = loc.startsWith('pt')
const words = computed(() => ({ yes: t('sheets.true'), no: t('sheets.false') }))

// ---- The tab shown ----
const tabId = ref(data.value.tabs[0].id)
watch(
  () => data.value.tabs.map((x) => x.id).join(),
  () => {
    if (!data.value.tabs.some((x) => x.id === tabId.value)) tabId.value = data.value.tabs[0].id
  },
)
const tab = computed<Tab>(() => data.value.tabs.find((x) => x.id === tabId.value) ?? data.value.tabs[0])

// The values, worked out again after each change.
const rev = ref(0)
const book = computed(() => {
  void rev.value
  return new Book(data.value, dayOf(todayStr()), comma)
})
function changed() {
  rev.value++
  emit('change')
}
watch(data, () => rev.value++)

// ---- Geometry ----
const widthOf = (c: number) => tab.value.widths?.[colName(c)] ?? COL_W
const colX = computed(() => {
  const xs = [0]
  for (let c = 0; c < tab.value.cols; c++) xs.push(xs[c] + widthOf(c))
  return xs
})
const freeze = computed(() => Math.min(tab.value.freeze ?? 0, tab.value.rows))
const totalW = computed(() => colX.value[tab.value.cols])
const bodyH = computed(() => (tab.value.rows - freeze.value) * ROW_H)

const scroller = ref<HTMLElement | null>(null)
const view = reactive({ top: 0, left: 0, w: 800, h: 500 })
function onScroll() {
  const el = scroller.value
  if (!el) return
  view.top = el.scrollTop
  view.left = el.scrollLeft
  // Near the bottom or the right: more rows / columns.
  if (props.editable && el.scrollTop + el.clientHeight > bodyH.value - ROW_H * 10 && tab.value.rows < MAX_ROWS) tab.value.rows = Math.min(MAX_ROWS, tab.value.rows + 100)
  if (props.editable && el.scrollLeft + el.clientWidth > totalW.value - 200 && tab.value.cols < MAX_COLS) tab.value.cols = Math.min(MAX_COLS, tab.value.cols + 10)
}
let ro: ResizeObserver | null = null
onMounted(() => {
  ro = new ResizeObserver(() => {
    if (scroller.value) Object.assign(view, { w: scroller.value.clientWidth, h: scroller.value.clientHeight })
  })
  if (scroller.value) ro.observe(scroller.value)
})
onBeforeUnmount(() => ro?.disconnect())

// What's in view: rows (below the frozen ones) and columns.
const rowsShown = computed(() => {
  const first = Math.max(freeze.value, freeze.value + Math.floor(view.top / ROW_H) - 4)
  const last = Math.min(tab.value.rows - 1, freeze.value + Math.ceil((view.top + view.h) / ROW_H) + 4)
  const out: number[] = []
  for (let r = first; r <= last; r++) out.push(r)
  return out
})
const frozenRows = computed(() => Array.from({ length: freeze.value }, (_, i) => i))
const colsShown = computed(() => {
  const xs = colX.value
  let first = 0
  while (first < tab.value.cols - 1 && xs[first + 1] < view.left) first++
  const out: number[] = []
  for (let c = Math.max(0, first - 1); c < tab.value.cols && xs[c] < view.left + view.w + 200; c++) out.push(c)
  return out
})

// ---- Cells ----
const cellAt = (r: number, c: number): Cell | undefined => tab.value.cells[addr(r, c)]
function shown(r: number, c: number) {
  return show(book.value.value(tab.value.id, r, c), cellAt(r, c), loc, words.value)
}

// ---- Selection ----
const sel = reactive({ r: 0, c: 0, r2: 0, c2: 0 }) // r,c: the active cell; r2,c2: the other corner
const box = computed(() => ({ r1: Math.min(sel.r, sel.r2), r2: Math.max(sel.r, sel.r2), c1: Math.min(sel.c, sel.c2), c2: Math.max(sel.c, sel.c2) }))
const activeAddr = computed(() => addr(sel.r, sel.c))
function select(r: number, c: number, extend = false) {
  r = Math.max(0, Math.min(tab.value.rows - 1, r))
  c = Math.max(0, Math.min(tab.value.cols - 1, c))
  if (extend) Object.assign(sel, { r2: r, c2: c })
  else Object.assign(sel, { r, c, r2: r, c2: c })
  reveal(extend ? r : sel.r, extend ? c : sel.c)
}
function reveal(r: number, c: number) {
  const el = scroller.value
  if (!el) return
  if (r >= freeze.value) {
    const y = (r - freeze.value) * ROW_H
    if (y < el.scrollTop) el.scrollTop = y
    else if (y + ROW_H > el.scrollTop + el.clientHeight) el.scrollTop = y + ROW_H - el.clientHeight
  }
  const x = colX.value[c]
  const w = widthOf(c)
  if (x < el.scrollLeft) el.scrollLeft = x
  else if (x + w > el.scrollLeft + el.clientWidth) el.scrollLeft = x + w - el.clientWidth
}
watch(tabId, () => {
  Object.assign(sel, { r: 0, c: 0, r2: 0, c2: 0 })
  stopEdit(false)
  if (scroller.value) scroller.value.scrollTop = scroller.value.scrollLeft = 0
})

// ---- Undo ----
type Step = { tab: string; before: Record<string, Cell | null>; after: Record<string, Cell | null> } | { whole: { before: string; after: string } }
const undos: Step[] = []
const redos: Step[] = []
const canUndo = ref(false)
const canRedo = ref(false)
function track() {
  canUndo.value = undos.length > 0
  canRedo.value = redos.length > 0
}
function setCells(tabIdOf: string, cells: Record<string, Cell | null>) {
  const tb = data.value.tabs.find((x) => x.id === tabIdOf)
  if (!tb) return
  for (const [a, cell] of Object.entries(cells)) {
    if (!cell || (cell.v === '' && !cell.b && !cell.i && !cell.nf && !cell.al && !cell.fg && !cell.bg)) delete tb.cells[a]
    else tb.cells[a] = cell
  }
}
// write changes cells (one undo step).
function write(cells: Record<string, Cell | null>) {
  if (!props.editable) return
  const before: Record<string, Cell | null> = {}
  for (const a of Object.keys(cells)) before[a] = tab.value.cells[a] ? { ...tab.value.cells[a] } : null
  setCells(tab.value.id, cells)
  undos.push({ tab: tab.value.id, before, after: cells })
  if (undos.length > 200) undos.shift()
  redos.length = 0
  track()
  changed()
}
// whole takes a snapshot around a change of the sheet's shape.
function whole(fn: () => void) {
  if (!props.editable) return
  const before = JSON.stringify(data.value)
  fn()
  undos.push({ whole: { before, after: JSON.stringify(data.value) } })
  redos.length = 0
  track()
  changed()
}
function replay(step: Step, back: boolean) {
  if ('whole' in step) data.value = JSON.parse(back ? step.whole.before : step.whole.after)
  else {
    setCells(step.tab, back ? step.before : step.after)
    tabId.value = step.tab
  }
  changed()
}
function undo() {
  const s = undos.pop()
  if (!s) return
  replay(s, true)
  redos.push(s)
  track()
}
function redo() {
  const s = redos.pop()
  if (!s) return
  replay(s, false)
  undos.push(s)
  track()
}

// ---- Editing a cell ----
const editing = ref(false)
const draft = ref('')
const mode = ref<'enter' | 'edit'>('edit') // enter: typing started it, arrows leave it
const editor = ref<HTMLTextAreaElement | null>(null)
const barInput = ref<HTMLInputElement | null>(null)
const inBar = ref(false)
function startEdit(text: string | null, how: 'enter' | 'edit', where: 'cell' | 'bar' = 'cell') {
  if (!props.editable) return
  draft.value = text ?? cellAt(sel.r, sel.c)?.v ?? ''
  mode.value = how
  editing.value = true
  inBar.value = where === 'bar'
  caret.value = draft.value.length
  void nextTick(() => {
    const el = where === 'bar' ? barInput.value : editor.value
    el?.focus()
    const n = draft.value.length
    el?.setSelectionRange(n, n)
  })
}
function stopEdit(save: boolean) {
  if (!editing.value) return
  editing.value = false
  if (save) {
    const cur = cellAt(sel.r, sel.c)
    if ((cur?.v ?? '') !== draft.value) write({ [activeAddr.value]: { ...(cur ?? {}), v: draft.value } })
  }
  void nextTick(() => grid.value?.focus({ preventScroll: true }))
}
function commitMove(dr: number, dc: number) {
  stopEdit(true)
  select(sel.r + dr, sel.c + dc)
}
// In a formula, a reference can go where an operand would (read at the
// moment: the caret moves without the text changing).
function pointing(): boolean {
  if (!editing.value || !draft.value.startsWith('=')) return false
  const el = inBar.value ? barInput.value : editor.value
  const at = el?.selectionStart ?? draft.value.length
  return /[=(,;+\-*/^&<>:]\s*$/.test(draft.value.slice(0, at)) || refAtCaret.value !== null
}
const refAtCaret = ref<{ from: number; to: number } | null>(null) // the reference put in by clicking (a drag replaces it)
// Function suggestions and argument hints while writing a formula.
const assist = ref<InstanceType<typeof FormulaAssist> | null>(null)
const caret = ref(0)
function trackCaret(e: Event) {
  caret.value = (e.target as HTMLInputElement | HTMLTextAreaElement).selectionStart ?? draft.value.length
}
function applyPick(edit: { from: number; to: number; insert: string }) {
  const el = inBar.value ? barInput.value : editor.value
  draft.value = draft.value.slice(0, edit.from) + edit.insert + draft.value.slice(edit.to)
  const at = edit.from + edit.insert.length
  caret.value = at
  refAtCaret.value = null
  void nextTick(() => {
    el?.focus()
    el?.setSelectionRange(at, at)
  })
}
function onEditorKey(e: KeyboardEvent) {
  if (e.isComposing) return
  if (assist.value?.key(e)) return
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    stopEdit(false)
  } else if (e.key === 'Enter' && !e.altKey) {
    e.preventDefault()
    commitMove(e.shiftKey ? -1 : 1, 0)
  } else if (e.key === 'Enter' && e.altKey) {
    // Alt-Enter: a new line in the cell, as in Excel.
    e.preventDefault()
    const el = e.target as HTMLTextAreaElement
    const at = el.selectionStart
    draft.value = draft.value.slice(0, at) + '\n' + draft.value.slice(el.selectionEnd)
    void nextTick(() => el.setSelectionRange(at + 1, at + 1))
  } else if (e.key === 'Tab') {
    e.preventDefault()
    commitMove(0, e.shiftKey ? -1 : 1)
  } else if (mode.value === 'enter' && ['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key) && !pointing()) {
    e.preventDefault()
    const d = { ArrowUp: [-1, 0], ArrowDown: [1, 0], ArrowLeft: [0, -1], ArrowRight: [0, 1] }[e.key]!
    commitMove(d[0], d[1])
  } else refAtCaret.value = null
}
function onEditorInput(e?: Event) {
  refAtCaret.value = null
  if (e) trackCaret(e)
}

// ---- The grid's keys (not editing) ----
const grid = ref<HTMLElement | null>(null)
const mod = (e: KeyboardEvent | MouseEvent) => e.metaKey || e.ctrlKey
function edge(r: number, c: number, dr: number, dc: number): [number, number] {
  // ⌘-arrow: to the edge of the data (as Excel).
  const filled = (rr: number, cc: number) => !!cellAt(rr, cc)?.v
  let rr = r
  let cc = c
  const inside = (a: number, b: number) => a >= 0 && b >= 0 && a < tab.value.rows && b < tab.value.cols
  const step = () => {
    rr += dr
    cc += dc
  }
  if (inside(rr + dr, cc + dc) && filled(rr, cc) && filled(rr + dr, cc + dc)) {
    while (inside(rr + dr, cc + dc) && filled(rr + dr, cc + dc)) step()
  } else {
    step()
    while (inside(rr, cc) && !filled(rr, cc) && inside(rr + dr, cc + dc)) step()
  }
  return [rr, cc]
}
function onKey(e: KeyboardEvent) {
  if (editing.value || e.isComposing) return
  const k = e.key
  const arrows: Record<string, [number, number]> = { ArrowUp: [-1, 0], ArrowDown: [1, 0], ArrowLeft: [0, -1], ArrowRight: [0, 1] }
  if (arrows[k]) {
    e.preventDefault()
    const [dr, dc] = arrows[k]
    const fromR = e.shiftKey ? sel.r2 : sel.r
    const fromC = e.shiftKey ? sel.c2 : sel.c
    const [r, c] = mod(e) ? edge(fromR, fromC, dr, dc) : [fromR + dr, fromC + dc]
    select(r, c, e.shiftKey)
    return
  }
  if (k === 'Tab') {
    e.preventDefault()
    select(sel.r, sel.c + (e.shiftKey ? -1 : 1))
  } else if (k === 'Enter') {
    e.preventDefault()
    if (e.shiftKey) select(sel.r - 1, sel.c)
    else startEdit(null, 'edit')
  } else if (k === 'F2') {
    e.preventDefault()
    startEdit(null, 'edit')
  } else if (k === 'Home') {
    e.preventDefault()
    select(mod(e) ? 0 : sel.r, 0, e.shiftKey)
  } else if (k === 'PageDown' || k === 'PageUp') {
    e.preventDefault()
    const n = Math.max(1, Math.floor(view.h / ROW_H) - 1)
    select(sel.r + (k === 'PageDown' ? n : -n), sel.c, e.shiftKey)
  } else if (k === 'Delete' || k === 'Backspace') {
    e.preventDefault()
    clearBox()
  } else if (mod(e) && !e.altKey) {
    const key = k.toLowerCase()
    const acts: Record<string, () => void> = {
      z: () => (e.shiftKey ? redo() : undo()),
      y: redo,
      b: () => toggle('b'),
      i: () => toggle('i'),
      a: () => Object.assign(sel, { r: 0, c: 0, r2: tab.value.rows - 1, c2: tab.value.cols - 1 }),
      d: () => fillFrom('down'),
      r: () => fillFrom('right'),
    }
    if (acts[key]) {
      e.preventDefault()
      acts[key]()
    }
  } else if (k.length === 1 && !e.altKey) {
    // Typing replaces the cell.
    e.preventDefault()
    startEdit(k, 'enter')
  }
}

// ---- Formatting ----
function eachCell(fn: (a: string, cell: Cell | undefined) => Cell | null) {
  const out: Record<string, Cell | null> = {}
  const b = box.value
  for (let r = b.r1; r <= Math.min(b.r2, b.r1 + 5000); r++) for (let c = b.c1; c <= b.c2; c++) out[addr(r, c)] = fn(addr(r, c), cellAt(r, c))
  write(out)
}
function toggle(k: 'b' | 'i') {
  const on = !cellAt(sel.r, sel.c)?.[k]
  eachCell((_a, cell) => ({ ...(cell ?? { v: '' }), [k]: on || undefined }))
}
function align(al: 'l' | 'c' | 'r') {
  const same = cellAt(sel.r, sel.c)?.al === al
  eachCell((_a, cell) => ({ ...(cell ?? { v: '' }), al: same ? undefined : al }))
}
function format(nf: NumFmt | '') {
  eachCell((_a, cell) => ({ ...(cell ?? { v: '' }), nf: nf || undefined }))
}
function clearBox() {
  eachCell((_a, cell) => (cell ? { ...cell, v: '' } : null))
}
const active = computed(() => cellAt(sel.r, sel.c))
function toggleFreeze() {
  whole(() => (tab.value.freeze = tab.value.freeze ? 0 : Math.max(1, sel.r)))
}

// ---- Copy, cut, paste (Excel's text; between Orbit sheets with formulas) ----
let clip: { text: string; cells: (Cell | undefined)[][]; r: number; c: number; cut: boolean; tab: string } | null = null
function onCopy(e: ClipboardEvent, cut = false) {
  if (editing.value) return
  e.preventDefault()
  const b = box.value
  const cells: (Cell | undefined)[][] = []
  const text: string[][] = []
  for (let r = b.r1; r <= b.r2; r++) {
    cells.push([])
    text.push([])
    for (let c = b.c1; c <= b.c2; c++) {
      cells[r - b.r1].push(cellAt(r, c) ? { ...cellAt(r, c)! } : undefined)
      text[r - b.r1].push(shown(r, c).text)
    }
  }
  const tsv = toTSV(text)
  e.clipboardData?.setData('text/plain', tsv)
  clip = { text: tsv, cells, r: b.r1, c: b.c1, cut, tab: tab.value.id }
}
function onCut(e: ClipboardEvent) {
  if (!props.editable) return
  onCopy(e, true)
}
function onPaste(e: ClipboardEvent) {
  if (editing.value || !props.editable) return
  const text = e.clipboardData?.getData('text/plain') ?? ''
  if (!text) return
  e.preventDefault()
  const out: Record<string, Cell | null> = {}
  let rows = 0
  let cols = 0
  if (clip && clip.text === text) {
    // From this sheet: inputs and looks, formulas moved.
    const dr = sel.r - clip.r
    const dc = sel.c - clip.c
    clip.cells.forEach((row, i) =>
      row.forEach((cell, j) => {
        const v = cell?.v.startsWith('=') && !clip!.cut ? shiftFormula(cell.v, dr, dc) : (cell?.v ?? '')
        out[addr(sel.r + i, sel.c + j)] = cell ? { ...cell, v } : null
      }),
    )
    rows = clip.cells.length
    cols = clip.cells[0]?.length ?? 0
    if (clip.cut && clip.tab === tab.value.id) {
      for (let i = 0; i < rows; i++) for (let j = 0; j < cols; j++) {
        const a = addr(clip.r + i, clip.c + j)
        if (!(a in out)) out[a] = null
      }
      clip = null
    }
  } else {
    const grid2 = fromTSV(text)
    grid2.forEach((row, i) => row.forEach((v, j) => (out[addr(sel.r + i, sel.c + j)] = { ...(cellAt(sel.r + i, sel.c + j) ?? {}), v })))
    rows = grid2.length
    cols = Math.max(...grid2.map((r) => r.length))
  }
  if (sel.r + rows > tab.value.rows) tab.value.rows = Math.min(MAX_ROWS, sel.r + rows + 10)
  if (sel.c + cols > tab.value.cols) tab.value.cols = Math.min(MAX_COLS, sel.c + cols)
  write(out)
  Object.assign(sel, { r2: sel.r + rows - 1, c2: sel.c + cols - 1 })
}

// ---- Filling (⌘D, ⌘R, the fill handle) ----
function fillRange(src: { r1: number; r2: number; c1: number; c2: number }, to: { r1: number; r2: number; c1: number; c2: number }) {
  const out: Record<string, Cell | null> = {}
  const down = to.r2 > src.r2 || to.r1 < src.r1
  if (down) {
    for (let c = src.c1; c <= src.c2; c++) {
      const inputs: string[] = []
      for (let r = src.r1; r <= src.r2; r++) inputs.push(cellAt(r, c)?.v ?? '')
      const n = src.r2 - src.r1 + 1
      const targets = to.r2 > src.r2 ? Array.from({ length: to.r2 - src.r2 }, (_, i) => src.r2 + 1 + i) : Array.from({ length: src.r1 - to.r1 }, (_, i) => src.r1 - 1 - i)
      const lits = series(inputs, targets.length)
      targets.forEach((r, i) => {
        const from = to.r2 > src.r2 ? src.r1 + (i % n) : src.r2 - (i % n)
        const base = cellAt(from, c)
        const v = base?.v.startsWith('=') ? shiftFormula(base.v, r - from, 0) : to.r2 > src.r2 ? lits[i] : (base?.v ?? '')
        out[addr(r, c)] = base || v ? { ...(base ?? {}), v } : null
      })
    }
  } else {
    for (let r = src.r1; r <= src.r2; r++) {
      const inputs: string[] = []
      for (let c = src.c1; c <= src.c2; c++) inputs.push(cellAt(r, c)?.v ?? '')
      const n = src.c2 - src.c1 + 1
      const targets = to.c2 > src.c2 ? Array.from({ length: to.c2 - src.c2 }, (_, i) => src.c2 + 1 + i) : Array.from({ length: src.c1 - to.c1 }, (_, i) => src.c1 - 1 - i)
      const lits = series(inputs, targets.length)
      targets.forEach((c, i) => {
        const from = to.c2 > src.c2 ? src.c1 + (i % n) : src.c2 - (i % n)
        const base = cellAt(r, from)
        const v = base?.v.startsWith('=') ? shiftFormula(base.v, 0, c - from) : to.c2 > src.c2 ? lits[i] : (base?.v ?? '')
        out[addr(r, c)] = base || v ? { ...(base ?? {}), v } : null
      })
    }
  }
  write(out)
}
function fillFrom(dir: 'down' | 'right') {
  const b = box.value
  if (dir === 'down' && b.r2 > b.r1) fillRange({ ...b, r2: b.r1 }, b)
  if (dir === 'right' && b.c2 > b.c1) fillRange({ ...b, c2: b.c1 }, b)
}

// ---- The mouse: selecting, pointing in a formula, the fill handle, widths ----
type Drag = { kind: 'select' } | { kind: 'point'; r: number; c: number } | { kind: 'fill'; src: typeof box.value } | { kind: 'width'; col: number; x0: number; w0: number }
let drag: Drag | null = null
const fillTo = ref<{ r1: number; r2: number; c1: number; c2: number } | null>(null)
function cellFrom(e: MouseEvent): { r: number; c: number } | null {
  const el = (e.target as HTMLElement).closest<HTMLElement>('[data-r]')
  return el ? { r: Number(el.dataset.r), c: Number(el.dataset.c) } : null
}
function onDown(e: MouseEvent) {
  if (e.button !== 0) return
  const hit = cellFrom(e)
  if (!hit) return
  if (pointing()) {
    // A reference into the formula being written.
    e.preventDefault()
    putRef(hit.r, hit.c, hit.r, hit.c)
    drag = { kind: 'point', r: hit.r, c: hit.c }
    return
  }
  if (editing.value) stopEdit(true)
  e.preventDefault()
  grid.value?.focus({ preventScroll: true })
  select(hit.r, hit.c, e.shiftKey)
  drag = { kind: 'select' }
}
function onMove(e: MouseEvent) {
  if (!drag) return
  if (drag.kind === 'width') {
    const w = Math.max(30, Math.min(800, drag.w0 + e.clientX - drag.x0))
    tab.value.widths = { ...(tab.value.widths ?? {}), [colName(drag.col)]: Math.round(w) }
    return
  }
  const hit = cellFrom(e)
  if (!hit) return
  if (drag.kind === 'select') select(hit.r, hit.c, true)
  else if (drag.kind === 'point') putRef(drag.r, drag.c, hit.r, hit.c)
  else if (drag.kind === 'fill') {
    const s = drag.src
    const down = Math.abs(hit.r - (hit.r > s.r2 ? s.r2 : s.r1)) * ROW_H >= Math.abs(colX.value[hit.c] - colX.value[hit.c > s.c2 ? s.c2 : s.c1])
    if (down) fillTo.value = hit.r > s.r2 ? { ...s, r2: hit.r } : hit.r < s.r1 ? { ...s, r1: hit.r } : null
    else fillTo.value = hit.c > s.c2 ? { ...s, c2: hit.c } : hit.c < s.c1 ? { ...s, c1: hit.c } : null
  }
}
function onUp() {
  if (drag?.kind === 'fill' && fillTo.value) {
    fillRange(drag.src, fillTo.value)
    Object.assign(sel, { r: fillTo.value.r1, c: fillTo.value.c1, r2: fillTo.value.r2, c2: fillTo.value.c2 })
  }
  if (drag?.kind === 'width') changed()
  fillTo.value = null
  drag = null
}
// Copy and paste come to the page (a grid has no text selection): they're
// the grid's while it has the keyboard.
const mine = () => !!grid.value && document.activeElement === grid.value
const docCopy = (e: ClipboardEvent) => mine() && onCopy(e)
const docCut = (e: ClipboardEvent) => mine() && onCut(e)
const docPaste = (e: ClipboardEvent) => mine() && onPaste(e)
onMounted(() => {
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
  document.addEventListener('copy', docCopy)
  document.addEventListener('cut', docCut)
  document.addEventListener('paste', docPaste)
})
onBeforeUnmount(() => {
  window.removeEventListener('mousemove', onMove)
  window.removeEventListener('mouseup', onUp)
  document.removeEventListener('copy', docCopy)
  document.removeEventListener('cut', docCut)
  document.removeEventListener('paste', docPaste)
})
function startFill(e: MouseEvent) {
  if (!props.editable) return
  e.preventDefault()
  e.stopPropagation()
  drag = { kind: 'fill', src: { ...box.value } }
}
function startWidth(c: number, e: MouseEvent) {
  if (!props.editable) return
  e.preventDefault()
  e.stopPropagation()
  drag = { kind: 'width', col: c, x0: e.clientX, w0: widthOf(c) }
}
// putRef writes a reference (to r1,c1 … r2,c2) where the caret is in the
// formula being written, or over the one just put there.
function putRef(r1: number, c1: number, r2: number, c2: number) {
  const el = inBar.value ? barInput.value : editor.value
  const a = { col: Math.min(c1, c2), row: Math.min(r1, r2), absCol: false, absRow: false }
  const b = { col: Math.max(c1, c2), row: Math.max(r1, r2), absCol: false, absRow: false }
  const ref = formatRef(a.col === b.col && a.row === b.row ? { a } : { a, b })
  const from = refAtCaret.value?.from ?? el?.selectionStart ?? draft.value.length
  const to = refAtCaret.value?.to ?? el?.selectionEnd ?? draft.value.length
  draft.value = draft.value.slice(0, from) + ref + draft.value.slice(to)
  refAtCaret.value = { from, to: from + ref.length }
  caret.value = from + ref.length
  void nextTick(() => {
    el?.focus()
    el?.setSelectionRange(from + ref.length, from + ref.length)
  })
}
function onDouble(e: MouseEvent) {
  const hit = cellFrom(e)
  if (!hit || pointing()) return
  select(hit.r, hit.c)
  startEdit(null, 'edit')
}

// ---- Rows and columns: the header menu ----
const headMenu = ref<{ anchor: HTMLElement; axis: 'row' | 'col'; at: number } | null>(null)
function openHead(axis: 'row' | 'col', at: number, e: MouseEvent) {
  e.preventDefault()
  // A header click selects the row or column (and the menu is on its right click / the button).
  if (axis === 'row') Object.assign(sel, { r: at, r2: at, c: 0, c2: tab.value.cols - 1 })
  else Object.assign(sel, { c: at, c2: at, r: 0, r2: tab.value.rows - 1 })
  if (e.type === 'contextmenu' && props.editable) headMenu.value = { anchor: e.currentTarget as HTMLElement, axis, at }
}
function reshapeAt(axis: 'row' | 'col', at: number, n: number) {
  headMenu.value = null
  whole(() => {
    const next = reshape(data.value, tab.value.id, axis, at, n)
    data.value.tabs = next.tabs
  })
}

// ---- Tabs ----
const renaming = ref('')
const tabName = ref('')
const tabMenu = ref<{ anchor: HTMLElement; id: string } | null>(null)
function addTab() {
  whole(() => {
    const tb = newTab(t('sheets.tabName', { n: data.value.tabs.length + 1 }))
    data.value.tabs.push(tb)
    tabId.value = tb.id
  })
}
function startRename(id: string) {
  if (!props.editable) return
  tabMenu.value = null
  renaming.value = id
  tabName.value = data.value.tabs.find((x) => x.id === id)?.name ?? ''
}
function finishRename() {
  const id = renaming.value
  renaming.value = ''
  const name = tabName.value.trim().slice(0, 31)
  const tb = data.value.tabs.find((x) => x.id === id)
  if (!tb || !name || name === tb.name || data.value.tabs.some((x) => x.id !== id && x.name.toLowerCase() === name.toLowerCase())) return
  whole(() => (tb.name = name))
}
function deleteTab(id: string) {
  tabMenu.value = null
  if (data.value.tabs.length < 2) return
  whole(() => (data.value.tabs = data.value.tabs.filter((x) => x.id !== id)))
}

// ---- The selection drawn ----
const rectOf = (r1: number, r2: number, c1: number, c2: number, frozen: boolean) => {
  const top = frozen ? r1 * ROW_H : (r1 - freeze.value) * ROW_H
  return { left: `${colX.value[c1]}px`, top: `${top}px`, width: `${colX.value[c2 + 1] - colX.value[c1]}px`, height: `${(r2 - r1 + 1) * ROW_H}px` }
}
// The part of a box in the body (below the frozen rows), or in the frozen part.
function part(b: { r1: number; r2: number; c1: number; c2: number }, frozen: boolean) {
  const r1 = frozen ? b.r1 : Math.max(b.r1, freeze.value)
  const r2 = frozen ? Math.min(b.r2, freeze.value - 1) : b.r2
  return r2 < r1 ? null : rectOf(r1, r2, b.c1, b.c2, frozen)
}
const editorStyle = computed(() => {
  const frozen = sel.r < freeze.value
  const s = rectOf(sel.r, sel.r, sel.c, sel.c, frozen)
  return { ...s, minWidth: s.width, width: 'auto' }
})
const FORMATS: { key: NumFmt | ''; label: string }[] = [
  { key: '', label: 'general' },
  { key: 'num', label: 'num' },
  { key: 'int', label: 'int' },
  { key: 'cur', label: 'cur' },
  { key: 'pct', label: 'pct' },
  { key: 'date', label: 'date' },
]
defineExpose({ tabId })
</script>

<template>
  <div class="sheet-ed" :class="prefs.editorClass('sheets')" data-testid="sheet-editor">
    <!-- Toolbar -->
    <div v-if="editable" class="sh-tools">
      <button type="button" class="icon-btn h-7 w-7" :disabled="!canUndo" :title="t('sheets.undo')" :aria-label="t('sheets.undo')" @click="undo"><Undo2 class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-7 w-7" :disabled="!canRedo" :title="t('sheets.redo')" :aria-label="t('sheets.redo')" @click="redo"><Redo2 class="h-4 w-4" /></button>
      <span class="sh-sep" />
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: active?.b }" :title="t('sheets.bold')" :aria-label="t('sheets.bold')" @click="toggle('b')"><Bold class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: active?.i }" :title="t('sheets.italic')" :aria-label="t('sheets.italic')" @click="toggle('i')"><Italic class="h-4 w-4" /></button>
      <span class="sh-sep" />
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: active?.al === 'l' }" :aria-label="t('sheets.alignLeft')" @click="align('l')"><AlignLeft class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: active?.al === 'c' }" :aria-label="t('sheets.alignCenter')" @click="align('c')"><AlignCenter class="h-4 w-4" /></button>
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: active?.al === 'r' }" :aria-label="t('sheets.alignRight')" @click="align('r')"><AlignRight class="h-4 w-4" /></button>
      <span class="sh-sep" />
      <select class="input !w-auto !py-0.5 text-[12.5px]" :value="active?.nf ?? ''" :aria-label="t('sheets.format')" data-testid="sheet-format" @change="format(($event.target as HTMLSelectElement).value as NumFmt | '')">
        <option v-for="f in FORMATS" :key="f.key" :value="f.key">{{ t(`sheets.fmt.${f.label}`) }}</option>
      </select>
      <button type="button" class="icon-btn h-7 w-7" :class="{ on: freeze > 0 }" :title="t('sheets.freeze')" :aria-label="t('sheets.freeze')" @click="toggleFreeze"><Rows3 class="h-4 w-4" /></button>
    </div>

    <!-- Formula bar -->
    <div class="sh-bar">
      <span class="sh-addr" data-testid="sheet-addr">{{ box.r1 === box.r2 && box.c1 === box.c2 ? activeAddr : `${addr(box.r1, box.c1)}:${addr(box.r2, box.c2)}` }}</span>
      <span class="sh-fx">fx</span>
      <input
        ref="barInput"
        class="sh-input"
        :value="editing ? draft : (active?.v ?? '')"
        :readonly="!editable"
        :aria-label="t('sheets.formulaBar')"
        spellcheck="false"
        data-testid="sheet-bar"
        @focus="!editing && startEdit(null, 'edit', 'bar')"
        @input="((draft = ($event.target as HTMLInputElement).value), onEditorInput($event))"
        @keydown="onEditorKey"
        @keyup="trackCaret"
        @click="trackCaret"
      />
    </div>

    <!-- The grid: headers above and to the left, cells scrolling under them -->
    <div
      ref="grid"
      class="sh-grid"
      tabindex="0"
      :style="{ '--rowh': `${ROW_H}px`, '--headh': `${HEAD_H}px`, '--rowheadw': `${ROW_HEAD_W}px` }"
      @keydown="onKey"
    >
      <div class="sh-corner" />
      <!-- Column letters, and the frozen rows under them -->
      <div class="sh-top" :style="{ height: `${HEAD_H + freeze * ROW_H}px` }">
        <div class="sh-slide" :style="{ width: `${totalW}px`, transform: `translateX(${-view.left}px)` }">
          <div
            v-for="c in colsShown"
            :key="c"
            class="sh-colhead"
            :class="{ on: c >= box.c1 && c <= box.c2 }"
            :style="{ left: `${colX[c]}px`, width: `${widthOf(c)}px` }"
            @mousedown="openHead('col', c, $event)"
            @contextmenu="openHead('col', c, $event)"
          >
            {{ colName(c) }}
            <span class="sh-resize" @mousedown="startWidth(c, $event)" />
          </div>
          <div class="sh-frozen" :style="{ top: `${HEAD_H}px` }" @mousedown="onDown" @dblclick="onDouble">
            <template v-for="r in frozenRows" :key="r">
              <div
                v-for="c in colsShown"
                :key="c"
                class="sh-cell"
                :class="[`al-${shown(r, c).align}`, { b: cellAt(r, c)?.b, i: cellAt(r, c)?.i, err: shown(r, c).error }]"
                :style="{ left: `${colX[c]}px`, top: `${r * ROW_H}px`, width: `${widthOf(c)}px` }"
                :data-r="r"
                :data-c="c"
              >
                {{ shown(r, c).text }}
              </div>
            </template>
            <div v-if="freeze && part(box, true)" class="sh-sel" :style="part(box, true)!" />
          </div>
        </div>
      </div>
      <!-- Row numbers -->
      <div class="sh-left" :style="{ top: `${HEAD_H + freeze * ROW_H}px` }">
        <div class="sh-slide-v" :style="{ height: `${bodyH}px`, transform: `translateY(${-view.top}px)` }">
          <div
            v-for="r in rowsShown"
            :key="r"
            class="sh-rowhead"
            :class="{ on: r >= box.r1 && r <= box.r2 }"
            :style="{ top: `${(r - freeze) * ROW_H}px` }"
            @mousedown="openHead('row', r, $event)"
            @contextmenu="openHead('row', r, $event)"
          >
            {{ r + 1 }}
          </div>
        </div>
      </div>
      <div v-if="freeze" class="sh-left-frozen" :style="{ height: `${freeze * ROW_H}px` }">
        <div v-for="r in frozenRows" :key="r" class="sh-rowhead" :class="{ on: r >= box.r1 && r <= box.r2 }" :style="{ top: `${r * ROW_H}px` }" @mousedown="openHead('row', r, $event)" @contextmenu="openHead('row', r, $event)">{{ r + 1 }}</div>
      </div>
      <!-- The cells -->
      <div ref="scroller" class="sh-body" :style="{ top: `${HEAD_H + freeze * ROW_H}px` }" data-testid="sheet-body" @scroll="onScroll" @mousedown="onDown" @dblclick="onDouble">
        <div class="sh-sizer" :style="{ width: `${totalW}px`, height: `${bodyH}px` }">
          <template v-for="r in rowsShown" :key="r">
            <div
              v-for="c in colsShown"
              :key="c"
              class="sh-cell"
              :class="[`al-${shown(r, c).align}`, { b: cellAt(r, c)?.b, i: cellAt(r, c)?.i, err: shown(r, c).error }]"
              :style="{ left: `${colX[c]}px`, top: `${(r - freeze) * ROW_H}px`, width: `${widthOf(c)}px` }"
              :data-r="r"
              :data-c="c"
              :data-addr="addr(r, c)"
            >
              {{ shown(r, c).text }}
            </div>
          </template>
          <div v-if="part(box, false)" class="sh-sel" :class="{ multi: box.r1 !== box.r2 || box.c1 !== box.c2 }" :style="part(box, false)!" data-testid="sheet-sel">
            <span v-if="editable && !editing" class="sh-handle" data-testid="sheet-fill" @mousedown="startFill" />
          </div>
          <div v-if="fillTo && part(fillTo, false)" class="sh-fillto" :style="part(fillTo, false)!" />
          <div v-if="sel.r >= freeze" class="sh-active" :style="rectOf(sel.r, sel.r, sel.c, sel.c, false)" />
          <textarea
            v-if="editing && !inBar && sel.r >= freeze"
            ref="editor"
            v-model="draft"
            class="sh-editor"
            :style="editorStyle"
            rows="1"
            spellcheck="false"
            :aria-label="activeAddr"
            data-testid="sheet-cell-editor"
            @keydown="onEditorKey"
            @input="onEditorInput"
            @keyup="trackCaret"
            @click="trackCaret"
            @blur="!pointing() && stopEdit(true)"
          />
        </div>
      </div>
      <textarea
        v-if="editing && !inBar && sel.r < freeze"
        ref="editor"
        v-model="draft"
        class="sh-editor frozen"
        :style="{ ...editorStyle, transform: `translate(${ROW_HEAD_W - view.left}px, ${HEAD_H}px)` }"
        rows="1"
        spellcheck="false"
        :aria-label="activeAddr"
        @keydown="onEditorKey"
        @input="onEditorInput"
        @keyup="trackCaret"
        @click="trackCaret"
        @blur="!pointing() && stopEdit(true)"
      />
    </div>
    <FormulaAssist ref="assist" :el="inBar ? barInput : editor" :text="draft" :caret="caret" :on="editing && draft.startsWith('=')" @pick="applyPick" />

    <!-- Tabs -->
    <div class="sh-tabs">
      <template v-for="tb in data.tabs" :key="tb.id">
        <input v-if="renaming === tb.id" v-model="tabName" class="sh-tab-input" maxlength="31" autofocus @keydown.enter="finishRename" @keydown.esc="renaming = ''" @blur="finishRename" />
        <button
          v-else
          type="button"
          class="sh-tab"
          :class="{ on: tb.id === tab.id }"
          data-testid="sheet-tab"
          @click="tabId = tb.id"
          @dblclick="startRename(tb.id)"
          @contextmenu.prevent="editable && (tabMenu = { anchor: $event.currentTarget as HTMLElement, id: tb.id })"
        >
          {{ tb.name }}
        </button>
      </template>
      <button v-if="editable" type="button" class="icon-btn h-7 w-7" :title="t('sheets.addTab')" :aria-label="t('sheets.addTab')" data-testid="sheet-add-tab" @click="addTab"><Plus class="h-4 w-4" /></button>
    </div>

    <Popover v-if="headMenu" :anchor="headMenu.anchor" :width="220" @close="headMenu = null">
      <div class="p-1.5" data-testid="sheet-head-menu">
        <template v-if="headMenu.axis === 'row'">
          <button type="button" class="menu-item w-full" @click="reshapeAt('row', headMenu.at, 1)">{{ t('sheets.rowAbove') }}</button>
          <button type="button" class="menu-item w-full" @click="reshapeAt('row', headMenu.at + 1, 1)">{{ t('sheets.rowBelow') }}</button>
          <button type="button" class="menu-item danger w-full" @click="reshapeAt('row', box.r1, -(box.r2 - box.r1 + 1))">{{ t('sheets.deleteRows', box.r2 - box.r1 + 1) }}</button>
        </template>
        <template v-else>
          <button type="button" class="menu-item w-full" @click="reshapeAt('col', headMenu.at, 1)">{{ t('sheets.colLeft') }}</button>
          <button type="button" class="menu-item w-full" @click="reshapeAt('col', headMenu.at + 1, 1)">{{ t('sheets.colRight') }}</button>
          <button type="button" class="menu-item danger w-full" @click="reshapeAt('col', box.c1, -(box.c2 - box.c1 + 1))">{{ t('sheets.deleteCols', box.c2 - box.c1 + 1) }}</button>
        </template>
      </div>
    </Popover>
    <Popover v-if="tabMenu" :anchor="tabMenu.anchor" :width="200" placement="top-start" @close="tabMenu = null">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full" @click="startRename(tabMenu.id)">{{ t('sheets.renameTab') }}</button>
        <button type="button" class="menu-item danger w-full" :disabled="data.tabs.length < 2" @click="deleteTab(tabMenu.id)">{{ t('sheets.deleteTab') }}</button>
      </div>
    </Popover>
  </div>
</template>

<style scoped>
.sheet-ed {
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  border-radius: var(--r-lg);
  background: var(--bg);
  box-shadow: 0 0 0 0.5px var(--border);
  overflow: hidden;
}
.sh-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
  padding: 6px 8px;
  border-bottom: 0.5px solid var(--border);
}
.sh-tools .on {
  background: var(--accent-bg);
  color: var(--accent-text);
}
.sh-sep {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: var(--border);
}
.sh-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-bottom: 0.5px solid var(--border);
  font-size: 13px;
}
.sh-addr {
  min-width: 64px;
  padding: 2px 6px;
  border-radius: 5px;
  background: var(--fill-2);
  font: 600 12px var(--font-mono);
  text-align: center;
}
.sh-fx {
  font-style: italic;
  color: var(--text-3);
}
.sh-input {
  flex: 1;
  min-width: 0;
  padding: 3px 4px;
  background: transparent;
  font: 13px var(--font-mono);
  outline: none;
}
.sh-grid {
  position: relative;
  flex: 1;
  min-height: 260px;
  overflow: hidden;
  outline: none;
  font-size: 13px;
  user-select: none;
}
.sh-corner,
.sh-top,
.sh-left,
.sh-left-frozen {
  position: absolute;
  z-index: 3;
  background: var(--grouped);
  overflow: hidden;
}
.sh-corner {
  left: 0;
  top: 0;
  width: var(--rowheadw);
  height: var(--headh);
  z-index: 4;
  border-right: 0.5px solid var(--border);
  border-bottom: 0.5px solid var(--border);
}
.sh-top {
  left: var(--rowheadw);
  right: 0;
  top: 0;
  background: var(--bg);
}
.sh-slide,
.sh-slide-v {
  position: relative;
  height: 100%;
}
.sh-colhead {
  position: absolute;
  top: 0;
  height: var(--headh);
  display: grid;
  place-items: center;
  border-right: 0.5px solid var(--border);
  border-bottom: 0.5px solid var(--border);
  background: var(--grouped);
  font-size: 11.5px;
  color: var(--text-3);
  cursor: default;
}
.sh-colhead.on,
.sh-rowhead.on {
  background: var(--accent-bg);
  color: var(--accent-text);
  font-weight: 600;
}
.sh-resize {
  position: absolute;
  top: 0;
  right: -3px;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 1;
}
.sh-frozen {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  box-shadow: 0 1px 0 var(--border-strong, var(--border));
}
.sh-left {
  left: 0;
  bottom: 0;
  width: var(--rowheadw);
  top: calc(var(--headh) + 0px);
}
.sh-left-frozen {
  left: 0;
  width: var(--rowheadw);
  top: var(--headh);
  z-index: 4;
}
.sh-rowhead {
  position: absolute;
  left: 0;
  width: 100%;
  height: var(--rowh);
  display: grid;
  place-items: center;
  border-bottom: 0.5px solid var(--border);
  border-right: 0.5px solid var(--border);
  font-size: 11.5px;
  color: var(--text-3);
}
.sh-body {
  position: absolute;
  left: var(--rowheadw);
  right: 0;
  bottom: 0;
  overflow: auto;
}
.sh-sizer {
  position: relative;
}
.sh-cell {
  position: absolute;
  height: var(--rowh);
  padding: 0 6px;
  line-height: var(--rowh);
  border-right: 0.5px solid var(--line-soft);
  border-bottom: 0.5px solid var(--line-soft);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-variant-numeric: tabular-nums;
}
.sh-cell.al-r {
  text-align: right;
}
.sh-cell.al-c {
  text-align: center;
}
.sh-cell.b {
  font-weight: 700;
}
.sh-cell.i {
  font-style: italic;
}
.sh-cell.err {
  color: var(--crit-text);
}
.sh-sel {
  position: absolute;
  pointer-events: none;
  box-shadow: inset 0 0 0 1.5px var(--accent);
}
.sh-sel.multi {
  background: color-mix(in srgb, var(--accent) 10%, transparent);
}
.sh-active {
  position: absolute;
  pointer-events: none;
  box-shadow: inset 0 0 0 2px var(--accent);
}
.sh-fillto {
  position: absolute;
  pointer-events: none;
  outline: 1.5px dashed var(--accent);
  outline-offset: -1px;
}
.sh-handle {
  position: absolute;
  right: -4px;
  bottom: -4px;
  width: 8px;
  height: 8px;
  border: 1.5px solid var(--bg);
  background: var(--accent);
  pointer-events: auto;
  cursor: crosshair;
}
.sh-editor {
  position: absolute;
  z-index: 2;
  height: auto;
  min-height: var(--rowh);
  max-width: 600px;
  padding: 3px 6px;
  border: 0;
  resize: none;
  outline: none;
  box-shadow: 0 0 0 2px var(--accent), var(--shadow);
  background: var(--surface);
  color: var(--text);
  font: inherit;
  line-height: 20px;
  field-sizing: content;
}
.sh-editor.frozen {
  z-index: 5;
}
.sh-tabs {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 4px 8px;
  border-top: 0.5px solid var(--border);
  overflow-x: auto;
  background: var(--grouped);
}
.sh-tab {
  padding: 3px 10px;
  border-radius: 6px;
  font-size: 12.5px;
  color: var(--text-2);
  white-space: nowrap;
}
.sh-tab.on {
  background: var(--surface);
  color: var(--text);
  font-weight: 600;
  box-shadow: 0 0 0 0.5px var(--border);
}
.sh-tab-input {
  width: 120px;
  padding: 2px 6px;
  border-radius: 6px;
  font-size: 12.5px;
  background: var(--surface);
  box-shadow: 0 0 0 2px var(--accent);
  outline: none;
}
</style>

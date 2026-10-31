<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Download, ExternalLink, File as FileIcon, LoaderCircle, X } from 'lucide-vue-next'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { fetchAuthedBlob } from '@/lib/api'
import { downloadUpload } from '@/lib/uploads'
import { filesApi, sizeText, type FileUpload, type FileVersion } from '@/lib/filesApi'
import { onRoute } from '@/lib/fileOps'
import { openPdf, playType, viewKind, type ViewKind } from '@/lib/fileView'
import type { PptDeck } from '@/lib/pptx'
import type { SheetData } from '@/lib/sheet'
import { locale } from '@/lib/dates'
import { ago } from '@/components/notes/noteModel'

// A file shown in place: a PDF's pages, a Word document as a page to read,
// a spreadsheet's tabs, a presentation's slides, a picture, a video or a
// song, text. Its versions to choose from; Download for the real thing; what
// it's on, a click away. In an overlay (closable) or a page of its own.
const props = defineProps<{ id: string; closable?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const router = useRouter()
const loc = locale()

const meta = ref<FileUpload | null>(null)
const versions = ref<FileVersion[]>([])
const version = ref('') // the version shown (its id)
const error = ref('')
const loading = ref(false)
const kind = ref<ViewKind>('none')
const url = ref('') // a blob: URL (picture, video, audio)
const html = ref('') // a Word document, Markdown
const text = ref('')
const deck = shallowRef<PptDeck | null>(null)
const sheet = shallowRef<SheetData | null>(null)
const tab = ref(0)
const pages = ref<{ n: number; w: number; h: number }[]>([])
const pdfEl = ref<HTMLElement | null>(null)
let pdfDoc: Awaited<ReturnType<typeof openPdf>> | null = null
let observer: IntersectionObserver | null = null

const shown = computed(() => versions.value.find((v) => v.id === version.value) ?? meta.value)

function reset() {
  if (url.value) URL.revokeObjectURL(url.value)
  url.value = html.value = text.value = error.value = ''
  deck.value = sheet.value = null
  pages.value = []
  tab.value = 0
  observer?.disconnect()
  void pdfDoc?.loadingTask.destroy()
  pdfDoc = null
}
onBeforeUnmount(reset)

watch(
  () => props.id,
  async (id) => {
    reset()
    meta.value = null
    versions.value = []
    try {
      meta.value = await filesApi.file(id)
      version.value = meta.value.id
      if (meta.value.versions > 1) versions.value = await filesApi.versions(id)
      await show()
    } catch (e: any) {
      error.value = e.message
    }
  },
  { immediate: true },
)
watch(version, (v, old) => {
  if (old && v !== old) {
    reset()
    void show()
  }
})

async function show() {
  const f = shown.value
  if (!f) return
  kind.value = viewKind(f.name, f.type)
  if (kind.value === 'none') return
  loading.value = true
  try {
    const path = new URL(f.url, location.origin).pathname.slice('/api/v1'.length) + '?exact=1'
    const blob = await fetchAuthedBlob(path)
    const bytes = () => blob.arrayBuffer().then((b) => new Uint8Array(b))
    switch (kind.value) {
      case 'image':
      case 'video':
      case 'audio':
        url.value = URL.createObjectURL(new Blob([blob], { type: playType(f.name, f.type) }))
        break
      case 'pdf':
        await showPdf(await bytes())
        break
      case 'docx':
        html.value = (await import('@/lib/docx')).docxToHtml(await bytes())
        break
      case 'pptx':
        deck.value = (await import('@/lib/pptx')).readPptx(await bytes())
        break
      case 'sheet':
        sheet.value = (await import('@/lib/xlsx')).readXlsx(await bytes(), 200_000)
        break
      case 'csv':
        sheet.value = (await import('@/lib/xlsx')).readCsv(await blob.text(), f.name.replace(/\.[^.]+$/, '').slice(0, 31) || 'Sheet 1')
        break
      case 'markdown':
        html.value = DOMPurify.sanitize(await marked.parse(await blob.text()))
        break
      case 'text':
        text.value = await blob.text()
        break
    }
  } catch (e: any) {
    error.value = t('viewer.cantShow', { error: e?.message ?? String(e) })
  } finally {
    loading.value = false
  }
  if (pages.value.length) await watchPages()
}

// ---- A PDF: each page drawn when it comes into view ----
async function showPdf(bytes: Uint8Array) {
  pdfDoc = await openPdf(bytes)
  const out: { n: number; w: number; h: number }[] = []
  for (let i = 1; i <= pdfDoc.numPages; i++) {
    const vp = (await pdfDoc.getPage(i)).getViewport({ scale: 1 })
    out.push({ n: i, w: vp.width, h: vp.height })
  }
  pages.value = out
}
// Pages are drawn as they come near (they're laid out once the spinner goes).
async function watchPages() {
  await nextTick()
  observer = new IntersectionObserver(
    (es) => {
      for (const e of es) {
        if (!e.isIntersecting) continue
        observer?.unobserve(e.target)
        void drawPage(e.target as HTMLCanvasElement)
      }
    },
    { root: null, rootMargin: '600px 0px' },
  )
  for (const c of pdfEl.value?.querySelectorAll('canvas') ?? []) observer.observe(c)
}
async function drawPage(c: HTMLCanvasElement) {
  if (!pdfDoc) return
  const page = await pdfDoc.getPage(Number(c.dataset.page))
  const base = page.getViewport({ scale: 1 })
  const scale = (Math.min(c.clientWidth || 900, 1200) * Math.min(window.devicePixelRatio || 1, 2)) / base.width
  const vp = page.getViewport({ scale })
  c.width = Math.round(vp.width)
  c.height = Math.round(vp.height)
  await page.render({ canvas: c, canvasContext: c.getContext('2d')!, viewport: vp }).promise
  c.classList.add('drawn')
}

// ---- A spreadsheet: the tab chosen, as its cells read ----
interface Grid {
  cols: string[]
  rows: { text: string; align: string; bold: boolean; italic: boolean }[][]
}
const table = shallowRef<Grid | null>(null)
watch([sheet, tab], async ([d, i]) => {
  const tb = d?.tabs[i]
  if (!d || !tb) return void (table.value = null)
  const [{ Book, parseAddr, addr, show, colName }, { dayOf }, { todayStr }] = await Promise.all([import('@/lib/sheet'), import('@/lib/formula'), import('@/lib/dates')])
  const book = new Book(d, dayOf(todayStr()), loc.startsWith('pt'))
  let rows = 0
  let cols = 0
  for (const a of Object.keys(tb.cells)) {
    const p = parseAddr(a)
    if (p && tb.cells[a].v !== '') {
      rows = Math.max(rows, p.r + 1)
      cols = Math.max(cols, p.c + 1)
    }
  }
  rows = Math.min(rows, 2000)
  cols = Math.min(cols, 60)
  table.value = {
    cols: Array.from({ length: cols }, (_, c) => colName(c)),
    rows: Array.from({ length: rows }, (_, r) =>
      Array.from({ length: cols }, (_, c) => {
        const cell = tb.cells[addr(r, c)]
        const v = show(book.value(tb.id, r, c), cell, loc, { yes: '✓', no: '' })
        return { text: v.text, align: v.align === 'r' ? 'right' : v.align === 'c' ? 'center' : 'left', bold: !!cell?.b, italic: !!cell?.i }
      }),
    ),
  }
})

// ---- A slide's text size, in the slide's own width (container units) ----
const pt = (size: number | undefined, title: boolean) => `${(((size ?? (title ? 40 : 20)) / (deck.value?.width || 720)) * 100).toFixed(3)}cqw`

function download() {
  const f = shown.value
  if (f) void downloadUpload(f.url + (f.id !== meta.value?.id ? '?exact=1' : ''), f.name)
}
function openOn() {
  const m = meta.value
  if (!m?.on_kind || !m.on_id) return
  emit('close')
  void router.push(onRoute(m.workspace_id, m.on_kind, m.on_id))
}
const sub = computed(() => {
  const m = shown.value
  if (!m) return ''
  const on = meta.value?.on_kind ? t(`drive.on.${meta.value.on_kind}`, { title: meta.value.on_title || t('pages.untitled') }) : ''
  return [on, sizeText(m.size, loc), ago(m.created_at, loc)].filter(Boolean).join(' · ')
})
</script>

<template>
  <div class="viewer" data-testid="file-viewer">
    <header class="v-bar">
      <FileIcon class="h-5 w-5 shrink-0 text-text-3" />
      <div class="min-w-0 flex-1">
        <div class="truncate text-[15px] font-semibold" data-testid="viewer-name">{{ meta?.name ?? '' }}</div>
        <div class="truncate text-[12px] text-text-3">{{ sub }}</div>
      </div>
      <select v-if="versions.length > 1" v-model="version" class="input !w-auto !py-1 text-[12.5px]" :aria-label="t('drive.versions')" data-testid="viewer-version">
        <option v-for="(v, i) in versions" :key="v.id" :value="v.id">v{{ versions.length - i }} · {{ ago(v.created_at, loc) }}{{ v.current ? ` · ${t('drive.current')}` : '' }}</option>
      </select>
      <button v-if="meta?.on_id" type="button" class="btn btn-sm" data-testid="viewer-open-on" @click="openOn"><ExternalLink class="h-3.5 w-3.5" />{{ t(`drive.openOn.${meta.on_kind}`) }}</button>
      <button type="button" class="btn btn-sm" data-testid="viewer-download" @click="download"><Download class="h-3.5 w-3.5" />{{ t('drive.download') }}</button>
      <button v-if="closable" type="button" class="icon-btn h-8 w-8" :aria-label="t('common.close')" data-testid="viewer-close" @click="emit('close')"><X class="h-4 w-4" /></button>
    </header>

    <div class="v-body">
      <p v-if="error" class="v-note text-crit-text">{{ error }}</p>
      <div v-else-if="loading" class="v-note"><LoaderCircle class="h-5 w-5 animate-spin" /></div>
      <div v-else-if="kind === 'none' && meta" class="v-note" data-testid="viewer-none">
        <FileIcon class="h-10 w-10 text-text-3" />
        <p>{{ t('viewer.none') }}</p>
        <button type="button" class="btn-primary" @click="download"><Download class="h-4 w-4" />{{ t('drive.download') }}</button>
      </div>

      <div v-else-if="kind === 'pdf'" ref="pdfEl" class="v-pdf" data-testid="viewer-pdf">
        <canvas v-for="p in pages" :key="p.n" :data-page="p.n" :style="{ aspectRatio: `${p.w} / ${p.h}` }" />
      </div>

      <article v-else-if="kind === 'docx' || kind === 'markdown'" class="v-paper" data-testid="viewer-doc" v-html="html" />

      <pre v-else-if="kind === 'text'" class="v-text" data-testid="viewer-text">{{ text }}</pre>

      <img v-else-if="kind === 'image' && url" :src="url" alt="" class="v-media" data-testid="viewer-image" />
      <video v-else-if="kind === 'video' && url" :src="url" controls class="v-media" data-testid="viewer-video" />
      <audio v-else-if="kind === 'audio' && url" :src="url" controls class="mt-16" data-testid="viewer-audio" />

      <div v-else-if="(kind === 'sheet' || kind === 'csv') && sheet" class="v-sheet" data-testid="viewer-sheet">
        <div v-if="sheet.tabs.length > 1" class="v-tabs" role="tablist">
          <button v-for="(tb, i) in sheet.tabs" :key="tb.id" type="button" role="tab" :aria-selected="tab === i" :class="{ on: tab === i }" data-testid="viewer-tab" @click="tab = i">{{ tb.name }}</button>
        </div>
        <div v-if="table" class="v-grid">
          <table>
            <thead>
              <tr>
                <th />
                <th v-for="c in table.cols" :key="c">{{ c }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, r) in table.rows" :key="r">
                <th>{{ r + 1 }}</th>
                <td v-for="(cell, c) in row" :key="c" :style="{ textAlign: cell.align as 'left', fontWeight: cell.bold ? 600 : undefined, fontStyle: cell.italic ? 'italic' : undefined }">{{ cell.text }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div v-else-if="kind === 'pptx' && deck" class="v-slides" data-testid="viewer-slides">
        <div v-for="(s, i) in deck.slides" :key="i" class="v-slide-wrap">
          <div class="v-slide" :style="{ aspectRatio: `${deck.width} / ${deck.height}`, background: s.background || '#fff' }" data-testid="viewer-slide">
            <div v-for="(sh, j) in s.shapes" :key="j" class="v-shape" :style="{ left: `${sh.x * 100}%`, top: `${sh.y * 100}%`, width: `${sh.w * 100}%`, height: `${sh.h * 100}%`, background: sh.fill }">
              <img v-if="sh.picture" :src="sh.picture" alt="" />
              <table v-else-if="sh.table" class="v-ptable">
                <tr v-for="(row, r) in sh.table" :key="r"><td v-for="(c, k) in row" :key="k">{{ c }}</td></tr>
              </table>
              <template v-else>
                <p v-for="(p, k) in sh.paras" :key="k" :class="{ bullet: p.bullet }" :style="{ textAlign: p.align, marginLeft: `${p.level * 4}%` }">
                  <span
                    v-for="(r, m) in p.runs"
                    :key="m"
                    :style="{ fontSize: pt(r.size, !!sh.title), fontWeight: r.bold || sh.title ? 600 : undefined, fontStyle: r.italic ? 'italic' : undefined, textDecoration: r.underline ? 'underline' : undefined, color: r.color }"
                    >{{ r.text }}</span
                  >
                </p>
              </template>
            </div>
          </div>
          <div class="mt-1.5 text-center text-[11.5px] text-text-3">{{ i + 1 }} / {{ deck.slides.length }}</div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.viewer {
  display: flex;
  height: 100%;
  min-height: 0;
  flex-direction: column;
  background: var(--bg);
}
.v-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-bottom: 0.5px solid var(--border);
  background: var(--surface);
}
.v-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 24px clamp(12px, 3vw, 40px) 48px;
  background: var(--fill-2);
}
.v-note {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  margin-top: 80px;
  font-size: 14px;
  color: var(--text-2);
}
.v-pdf {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
}
.v-pdf canvas {
  width: min(100%, 900px);
  background: #fff;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12), 0 0 0 0.5px var(--border);
}
.v-paper {
  max-width: 820px;
  margin: 0 auto;
  padding: clamp(24px, 5vw, 64px);
  border-radius: 4px;
  background: #fff;
  color: #1c1c1e;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12), 0 0 0 0.5px var(--border);
}
.v-paper {
  font-size: 15px;
  line-height: 1.6;
}
.v-paper :deep(h1) {
  margin: 0 0 0.6em;
  font-size: 1.9em;
  font-weight: 700;
  line-height: 1.2;
}
.v-paper :deep(h2) {
  margin: 1.2em 0 0.4em;
  font-size: 1.45em;
  font-weight: 650;
  line-height: 1.25;
}
.v-paper :deep(h3) {
  margin: 1em 0 0.35em;
  font-size: 1.2em;
  font-weight: 600;
}
.v-paper :deep(h4),
.v-paper :deep(h5),
.v-paper :deep(h6) {
  margin: 0.9em 0 0.3em;
  font-weight: 600;
}
.v-paper :deep(p) {
  margin: 0 0 0.7em;
}
.v-paper :deep(ul),
.v-paper :deep(ol) {
  margin: 0 0 0.7em;
  padding-left: 1.6em;
}
.v-paper :deep(ul) {
  list-style: disc;
}
.v-paper :deep(ol) {
  list-style: decimal;
}
.v-paper :deep(blockquote) {
  margin: 0 0 0.7em;
  padding-left: 1em;
  border-left: 3px solid #d2d2d7;
  color: #48484a;
}
.v-paper :deep(a) {
  color: #007aff;
  text-decoration: underline;
}
.v-paper :deep(pre),
.v-paper :deep(code) {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.9em;
}
.v-paper :deep(td) :deep(p) {
  margin: 0;
}
.v-paper :deep(img) {
  max-width: 100%;
  height: auto;
}
.v-paper :deep(table) {
  border-collapse: collapse;
  margin: 12px 0;
}
.v-paper :deep(td) {
  padding: 4px 8px;
  border: 1px solid #d2d2d7;
  vertical-align: top;
}
.v-paper :deep(.subtitle) {
  color: #6e6e73;
  font-size: 1.15em;
}
.v-text {
  max-width: 1000px;
  margin: 0 auto;
  padding: 20px;
  border-radius: 10px;
  background: var(--surface);
  font: 12.5px/1.55 ui-monospace, SFMono-Regular, Menlo, monospace;
  white-space: pre-wrap;
  word-break: break-word;
}
.v-media {
  display: block;
  max-width: 100%;
  max-height: calc(100vh - 180px);
  margin: 0 auto;
  border-radius: 6px;
}
.v-body > audio {
  display: block;
  margin-left: auto;
  margin-right: auto;
}
.v-sheet {
  max-width: 100%;
}
.v-tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 10px;
}
.v-tabs button {
  padding: 4px 12px;
  border-radius: 8px;
  font-size: 13px;
  color: var(--text-2);
}
.v-tabs button.on {
  background: var(--surface);
  color: var(--text);
  box-shadow: 0 0 0 0.5px var(--border);
}
.v-grid {
  overflow: auto;
  border-radius: 8px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border);
}
.v-grid table {
  border-collapse: collapse;
  font-size: 12.5px;
}
.v-grid th,
.v-grid td {
  min-width: 64px;
  max-width: 320px;
  padding: 4px 8px;
  border: 0.5px solid var(--border);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.v-grid th {
  min-width: 36px;
  background: var(--fill-2);
  color: var(--text-3);
  font-weight: 500;
  text-align: center;
}
.v-slides {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 22px;
}
.v-slide-wrap {
  width: min(100%, 960px);
}
.v-slide {
  position: relative;
  overflow: hidden;
  container-type: inline-size;
  color: #1c1c1e;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.14), 0 0 0 0.5px var(--border);
}
.v-shape {
  position: absolute;
  overflow: hidden;
}
.v-shape img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.v-shape p {
  margin: 0;
  line-height: 1.2;
  white-space: pre-wrap;
}
.v-shape p.bullet::before {
  content: '• ';
}
.v-ptable {
  width: 100%;
  height: 100%;
  border-collapse: collapse;
  font-size: 1.4cqw;
}
.v-ptable td {
  padding: 0.4cqw;
  border: 1px solid #d2d2d7;
}
</style>

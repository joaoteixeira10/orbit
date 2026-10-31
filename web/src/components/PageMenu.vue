<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Star, StarOff, Copy, Link, FolderInput, Trash2, ChevronLeft, FileText, CornerDownRight, Download, FileCode, LayoutTemplate, Printer, FileType, Table2, Folder as FolderIcon, FolderX, Check, History, Share2, Lock, BookA } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { filesApi, tree, type Folder } from '@/lib/filesApi'
import Popover from './ui/Popover.vue'
import { pagesApi } from '@/api/planner'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { pageFile, download, fileName } from '@/lib/markdown'

// A page's actions: favourite, duplicate, copy link, download or copy it as
// Markdown, move to another parent, file it in a folder (with Files on),
// move to the trash.
const props = defineProps<{ pageId: string; ws: string; anchor: HTMLElement | null }>()
const emit = defineEmits<{ close: [] }>()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const toast = useToastStore()
const router = useRouter()
const { t } = useI18n()

const page = computed(() => pages.find(props.pageId))
const picking = ref(false)
// Downloading as a PDF or a Word file: the choice of how (drawn in, or links).
const exporting = ref<'pdf' | 'docx' | null>(null)

// Filing: the workspace's folders, as a tree.
const filing = ref(false)
const folders = ref<Folder[]>([])
const canFile = computed(() => !!wss.byId(props.ws)?.modules.includes('files'))
async function startFiling() {
  try {
    folders.value = await filesApi.folders(props.ws)
    filing.value = true
  } catch (e: any) {
    toast.error(e.message)
  }
}
const fileIn = (f: Folder | null) =>
  run(async () => {
    await filesApi.move(props.ws, f?.id ?? '', [{ kind: 'page', id: props.pageId }])
    pages.patch({ id: props.pageId, folder_id: f?.id ?? '' })
    toast.success(f ? t('project.filed', { name: f.name }) : t('project.unfiled'))
  })

// Pages it can move under: not itself or its descendants.
const targets = computed(() => {
  const all = pages.byWs[props.ws] ?? []
  const banned = new Set([props.pageId])
  let grew = true
  while (grew) {
    grew = false
    for (const p of all) {
      if (p.parent_id && banned.has(p.parent_id) && !banned.has(p.id)) {
        banned.add(p.id)
        grew = true
      }
    }
  }
  return all.filter((p) => !banned.has(p.id)).sort((a, b) => a.title.localeCompare(b.title))
})

async function run(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e: any) {
    toast.error(e.message)
  }
  emit('close')
}
const favorite = () =>
  run(async () => {
    const on = !page.value?.favorite
    await pagesApi.favorite(props.pageId, on)
    pages.patch({ id: props.pageId, favorite: on })
    await wss.loadFavorites()
  })
const duplicate = () =>
  run(async () => {
    const p = await pagesApi.duplicate(props.pageId)
    await pages.load(props.ws)
    router.push({ name: 'ws-page', params: { ws: props.ws, page: p.id } })
  })
const copyLink = () =>
  run(async () => {
    await navigator.clipboard.writeText(`${location.origin}/w/${props.ws}/p/${props.pageId}`)
    toast.success(t('pages.linkCopied'))
  })
// As a .md file (or on the clipboard): the title as a heading, then the page.
const asMarkdown = async () => {
  const p = await pagesApi.get(props.pageId)
  return { name: fileName(p.title || t('pages.untitled')) + '.md', text: pageFile(p.title || t('pages.untitled'), p.content) }
}
const downloadMd = () => run(async () => {
  const f = await asMarkdown()
  download(f.name, f.text)
})
const copyMd = () => run(async () => {
  await navigator.clipboard.writeText((await asMarkdown()).text)
  toast.success(t('editor.copied'))
})
// As a PDF or a Word file, downloaded: the page with what it shows in place
// drawn in (sheets' cells, diagrams' and whiteboards' pictures), or as their
// links (the choice remembered).
const PDF_KEY = 'orbit_pdf_links'
const pdfLinks = ref(readPdfLinks())
function readPdfLinks() {
  try {
    return localStorage.getItem(PDF_KEY) === '1'
  } catch {
    return false
  }
}
const asFile = (links: boolean) =>
  run(async () => {
    const as = exporting.value ?? 'pdf'
    pdfLinks.value = links
    try {
      localStorage.setItem(PDF_KEY, links ? '1' : '0')
    } catch {
      /* private window */
    }
    toast.info(t(as === 'docx' ? 'editor.docxMaking' : 'editor.pdfMaking'))
    const p = await pagesApi.get(props.pageId)
    const title = p.title || t('pages.untitled')
    const meta = [wss.byId(props.ws)?.name, new Intl.DateTimeFormat(undefined, { dateStyle: 'long' }).format(new Date(p.updated_at))].filter(Boolean).join(' · ')
    const { pageSources } = await import('@/lib/pdfSources')
    const subpages = () => (pages.byWs[props.ws] ?? []).filter((x) => x.parent_id === props.pageId).map((x) => x.title || t('pages.untitled'))
    const src = pageSources({ subpages, tabName: t('sheets.tabName', { n: 1 }), links })
    if (as === 'docx') {
      await (await import('@/lib/docxExport')).downloadDocx(fileName(title), title, meta, p.content, src)
      return
    }
    const { docOf, downloadPdf } = await import('@/lib/pdf')
    await downloadPdf(fileName(title), await docOf(title, meta, p.content, src))
  })
// A database of the pages inside it (or no longer: they stay as pages).
const toggleDatabase = () =>
  run(async () => {
    const { newDatabase } = await import('@/lib/database')
    await pages.saveDB(props.pageId, page.value?.db ? null : newDatabase({ status: t('db.status'), todo: t('db.todo'), doing: t('db.doing'), done: t('db.doneOpt'), table: t('db.table') }))
  })
// A glossary's terms are explained where the workspace's other pages use them.
const toggleGlossary = () =>
  run(async () => {
    const on = !page.value?.glossary
    await pagesApi.update(props.pageId, { glossary: on })
    pages.patch({ id: props.pageId, glossary: on })
    toast.success(on ? t('glossary.on') : t('glossary.off'))
  })
// A template is offered when a new page starts empty.
const toggleTemplate = () =>
  run(async () => {
    const on = !page.value?.template
    await pagesApi.update(props.pageId, { template: on })
    pages.patch({ id: props.pageId, template: on })
    toast.success(on ? t('pages.templateOn') : t('pages.templateOff'))
  })
const moveTo = (parent: string | null) =>
  run(async () => {
    await pagesApi.move(props.pageId, parent, '')
    if (parent) pages.toggle(parent, true)
    await pages.load(props.ws)
  })
const auth = useAuthStore()
// Who can see it, over the page (?access=1).
function access() {
  emit('close')
  void router.push({ name: 'ws-page', params: { ws: props.ws, page: props.pageId }, query: { access: '1' } })
}
// Guest links open over the page (?share=1).
function share() {
  emit('close')
  void router.push({ name: 'ws-page', params: { ws: props.ws, page: props.pageId }, query: { share: '1' } })
}
// History opens over the page (?history=1).
function history() {
  emit('close')
  void router.push({ name: 'ws-page', params: { ws: props.ws, page: props.pageId }, query: { history: '1' } })
}
const trash = () =>
  run(async () => {
    await pagesApi.trashPage(props.pageId)
    await pages.load(props.ws)
    await wss.loadFavorites()
    toast.success(t('pages.trashed'))
    if (router.currentRoute.value.params.page === props.pageId) router.push({ name: 'ws-pages', params: { ws: props.ws } })
  })
</script>

<template>
  <Popover :anchor="anchor" :width="240" placement="right-start" @close="emit('close')">
    <template v-if="filing">
      <button class="menu-item text-text-2" @click="filing = false"><ChevronLeft class="h-4 w-4" />{{ t('project.fileTitle') }}</button>
      <div class="menu-sep" />
      <div class="max-h-72 overflow-y-auto" data-testid="page-folders">
        <button class="menu-item" @click="fileIn(null)"><FolderX class="h-4 w-4 text-text-2" />{{ t('project.noFolder') }}<Check v-if="!page?.folder_id" class="ml-auto h-4 w-4 text-accent" /></button>
        <button v-for="x in tree(folders)" :key="x.folder.id" class="menu-item" data-testid="page-folder" @click="fileIn(x.folder)">
          <FolderIcon class="h-4 w-4 shrink-0" :style="{ color: x.folder.color || '#5ac8fa', marginLeft: `${x.depth * 12}px` }" />
          <span class="truncate">{{ x.folder.name }}</span>
          <Check v-if="page?.folder_id === x.folder.id" class="ml-auto h-4 w-4 shrink-0 text-accent" />
        </button>
      </div>
    </template>
    <template v-else-if="exporting">
      <button class="menu-item text-text-2" @click="exporting = null"><ChevronLeft class="h-4 w-4" />{{ exporting === 'docx' ? t('editor.docx') : t('editor.pdf') }}</button>
      <div class="menu-sep" />
      <button class="menu-item" data-testid="pdf-drawn" @click="asFile(false)">
        <Printer class="h-4 w-4 shrink-0 text-text-2" /><span class="flex-1">{{ t('editor.pdfDrawn') }}</span><Check v-if="!pdfLinks" class="h-4 w-4 shrink-0 text-accent" />
      </button>
      <button class="menu-item" data-testid="pdf-links" @click="asFile(true)">
        <Link class="h-4 w-4 shrink-0 text-text-2" /><span class="flex-1">{{ t('editor.pdfLinks') }}</span><Check v-if="pdfLinks" class="h-4 w-4 shrink-0 text-accent" />
      </button>
    </template>
    <template v-else-if="!picking">
      <button class="menu-item" @click="favorite">
        <component :is="page?.favorite ? StarOff : Star" class="h-4 w-4 text-text-2" />
        {{ page?.favorite ? t('pages.unfavorite') : t('pages.favorite') }}
      </button>
      <button class="menu-item" @click="duplicate"><Copy class="h-4 w-4 text-text-2" />{{ t('pages.duplicate') }}</button>
      <button class="menu-item" @click="copyLink"><Link class="h-4 w-4 text-text-2" />{{ t('pages.copyLink') }}</button>
      <button class="menu-item" @click="copyMd"><FileCode class="h-4 w-4 text-text-2" />{{ t('editor.copyMd') }}</button>
      <button class="menu-item" @click="downloadMd"><Download class="h-4 w-4 text-text-2" />{{ t('editor.download') }}</button>
      <button class="menu-item" data-testid="page-pdf" @click="exporting = 'pdf'"><Printer class="h-4 w-4 text-text-2" />{{ t('editor.pdf') }}</button>
      <button class="menu-item" data-testid="page-docx" @click="exporting = 'docx'"><FileType class="h-4 w-4 text-text-2" />{{ t('editor.docx') }}</button>
      <button class="menu-item" data-testid="page-access" @click="access"><Lock class="h-4 w-4 text-text-2" />{{ t('access.menu') }}</button>
      <button v-if="auth.features.guest_links !== false" class="menu-item" data-testid="page-share" @click="share"><Share2 class="h-4 w-4 text-text-2" />{{ t('share.menu') }}</button>
      <button class="menu-item" data-testid="page-history-open" @click="history"><History class="h-4 w-4 text-text-2" />{{ t('history.title') }}</button>
      <button class="menu-item" @click="picking = true"><FolderInput class="h-4 w-4 text-text-2" />{{ t('pages.moveTo') }}</button>
      <button v-if="canFile" class="menu-item" data-testid="page-file-in" @click="startFiling"><FolderIcon class="h-4 w-4 text-text-2" />{{ t('project.fileIn') }}</button>
      <button class="menu-item" data-testid="page-db-toggle" @click="toggleDatabase">
        <Table2 class="h-4 w-4 text-text-2" />{{ page?.db ? t('db.stop') : t('db.turnInto') }}
      </button>
      <button class="menu-item" data-testid="page-glossary-toggle" @click="toggleGlossary">
        <BookA class="h-4 w-4 text-text-2" />{{ page?.glossary ? t('glossary.stop') : t('glossary.use') }}
      </button>
      <button class="menu-item" data-testid="page-template-toggle" @click="toggleTemplate">
        <LayoutTemplate class="h-4 w-4 text-text-2" />{{ page?.template ? t('pages.templateStop') : t('pages.templateUse') }}
      </button>
      <div class="menu-sep" />
      <button class="menu-item danger" @click="trash"><Trash2 class="h-4 w-4" />{{ t('pages.toTrash') }}</button>
    </template>
    <template v-else>
      <button class="menu-item text-text-2" @click="picking = false"><ChevronLeft class="h-4 w-4" />{{ t('pages.moveTo') }}</button>
      <div class="menu-sep" />
      <div class="max-h-72 overflow-y-auto">
        <button class="menu-item" @click="moveTo(null)"><CornerDownRight class="h-4 w-4 text-text-2" />{{ t('pages.topLevel') }}</button>
        <button v-for="p in targets" :key="p.id" class="menu-item" @click="moveTo(p.id)">
          <span v-if="p.icon" class="w-4 text-center">{{ p.icon }}</span>
          <FileText v-else class="h-4 w-4 text-text-3" />
          <span class="truncate">{{ p.title || t('pages.untitled') }}</span>
        </button>
      </div>
    </template>
  </Popover>
</template>

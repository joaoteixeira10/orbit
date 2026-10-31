<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Brush, ChevronDown, FileInput, FileText, GitFork, Plus, Sheet, Upload, Waypoints, Workflow } from 'lucide-vue-next'
import { pagesApi } from '@/api/planner'
import Popover from '@/components/ui/Popover.vue'
import FileBrowser from '@/components/files/FileBrowser.vue'
import { FILE_KINDS, KIND_MODULE, type FileKind } from '@/lib/filesApi'
import { bringIn, kindOfFile, routeOf } from '@/lib/fileOps'
import { sheetsApi } from '@/lib/sheetsApi'
import { diagramsApi } from '@/lib/diagramsApi'
import { MERMAID_TEMPLATES } from '@/lib/mermaid'
import { drawingsApi } from '@/components/draw/drawing'
import { emptySheet } from '@/lib/sheet'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Files: a workspace's folders with everything in them — its files (from
// the computer, and the attachments of its pages, tasks and notes), and its
// sheets, diagrams and whiteboards (those whose modules are on). New ones,
// and files uploaded, go in the folder shown; Import turns files from the
// computer (.xlsx, .csv, .drawio, draw.io's .svg, .excalidraw) into the
// matching kind.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const kinds = computed<FileKind[]>(() => FILE_KINDS.filter((k) => workspace.value?.modules.includes(KIND_MODULE[k])))
const ICON: Record<FileKind, Component> = { sheet: Sheet, diagram: Workflow, board: Brush, page: FileText, file: FileText }

const browser = ref<InstanceType<typeof FileBrowser> | null>(null)
const folder = () => browser.value?.here ?? ''

const newOpen = ref(false)
const newEl = ref<HTMLElement | null>(null)
// What "New" makes: a diagram is draw.io's or Mermaid's.
const news = computed(() => kinds.value.flatMap((k): (FileKind | 'mermaid')[] => (k === 'diagram' ? ['diagram', 'mermaid'] : k === 'file' ? [] : [k])))
const importable = computed(() => kinds.value.some((k) => k === 'sheet' || k === 'diagram' || k === 'board'))
// Any file, as it is, into the folder shown.
const rawEl = ref<HTMLInputElement | null>(null)
function uploadRaw(e: Event) {
  const list = [...((e.target as HTMLInputElement).files ?? [])]
  ;(e.target as HTMLInputElement).value = ''
  void browser.value?.addFiles(list)
}
async function create(what: FileKind | 'mermaid') {
  newOpen.value = false
  const kind: FileKind = what === 'mermaid' ? 'diagram' : what
  try {
    const f = folder()
    const id =
      kind === 'page'
        ? (await pagesApi.create(ws.value, { title: '', folder_id: f })).id
        : kind === 'sheet'
        ? (await sheetsApi.create(ws.value, { title: '', data: emptySheet(t('sheets.tabName', { n: 1 })), folder_id: f })).id
        : kind === 'diagram'
          ? (await diagramsApi.create(ws.value, { title: '', folder_id: f, kind: what === 'mermaid' ? 'mermaid' : 'drawio', data: what === 'mermaid' ? MERMAID_TEMPLATES[0].code : undefined })).id
          : (await drawingsApi.make(ws.value, { title: '', folder_id: f })).id
    await router.push(routeOf(ws.value, { kind, id }))
  } catch (e: any) {
    toast.error(e.message)
  }
}

const fileEl = ref<HTMLInputElement | null>(null)
const accept = computed(() =>
  [kinds.value.includes('sheet') && '.xlsx,.csv,.tsv', kinds.value.includes('diagram') && '.drawio,.dio,.xml,.svg,.mmd,.mermaid', kinds.value.includes('board') && '.excalidraw'].filter(Boolean).join(','),
)
const busy = ref(false)
async function upload(e: Event) {
  const list = [...((e.target as HTMLInputElement).files ?? [])]
  ;(e.target as HTMLInputElement).value = ''
  if (!list.length) return
  busy.value = true
  let n = 0
  for (const f of list) {
    const kind = kindOfFile(f.name)
    if (!kind || !kinds.value.includes(kind)) {
      toast.error(t('drive.cantRead', { name: f.name }))
      continue
    }
    try {
      await bringIn(ws.value, f, folder(), { tab: t('sheets.tabName', { n: 1 }), notDrawio: t('diagrams.notDrawio'), notExcalidraw: t('drive.notExcalidraw'), notMermaid: t('mermaid.notMermaid') })
      n++
    } catch (err: any) {
      toast.error(t('drive.importFailed', { name: f.name, error: err?.message ?? String(err) }))
    }
  }
  busy.value = false
  if (n) {
    toast.success(t('drive.imported', { n }, n))
    void browser.value?.reload()
  }
}
</script>

<template>
  <div class="files-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.files') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }}</p>
      </div>
      <RouterLink :to="{ name: 'ws-graph', params: { ws } }" class="btn btn-ghost"><Waypoints class="h-4 w-4" />{{ t('graph.title') }}</RouterLink>
      <template v-if="editable && kinds.length">
        <template v-if="importable">
          <input ref="fileEl" type="file" class="hidden" multiple :accept="accept" data-testid="files-upload-input" @change="upload" />
          <button class="btn btn-ghost" :disabled="busy" :title="t('drive.uploadHint')" @click="fileEl?.click()"><FileInput class="h-4 w-4" />{{ t('drive.upload') }}</button>
        </template>
        <template v-if="kinds.includes('file')">
          <input ref="rawEl" type="file" class="hidden" multiple data-testid="files-add-input" @change="uploadRaw" />
          <button class="btn" :title="t('drive.addFilesHint')" data-testid="files-add" @click="rawEl?.click()"><Upload class="h-4 w-4" />{{ t('drive.addFiles') }}</button>
        </template>
        <button v-if="news.length" ref="newEl" class="btn-primary" data-testid="files-new" @click="newOpen = !newOpen"><Plus class="h-4 w-4" />{{ t('drive.new') }}<ChevronDown class="h-3.5 w-3.5" /></button>
      </template>
    </header>
    <p v-if="workspace && !kinds.length" class="mb-6 rounded-xl bg-fill-2 px-4 py-2.5 text-[13px] text-text-2">{{ t('drive.noKinds') }}</p>

    <FileBrowser ref="browser" :ws="ws" :kinds="kinds" :editable="editable" :root-name="t('modules.files')" />

    <Popover v-if="newOpen" :anchor="newEl" placement="bottom-end" :width="220" @close="newOpen = false">
      <div class="p-1.5">
        <button v-for="k in news" :key="k" type="button" class="menu-item w-full" :data-testid="`files-new-${k}`" @click="create(k)">
          <component :is="k === 'mermaid' ? GitFork : ICON[k]" class="h-4 w-4 text-text-2" />{{ k === 'mermaid' ? t('mermaid.newFile') : t(`drive.newKind.${k}`) }}
        </button>
      </div>
    </Popover>
  </div>
</template>

<style scoped>
.files-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
</style>

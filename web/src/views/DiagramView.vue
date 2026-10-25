<script setup lang="ts">
import { computed, onBeforeUnmount, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import { Check, ChevronLeft, Download, Link, LoaderCircle, MoreHorizontal, Trash2 } from 'lucide-vue-next'
import DrawioFrame from '@/components/diagrams/DrawioFrame.vue'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { diagramsApi, type DiagramDoc } from '@/lib/diagramsApi'
import { bytesOfDataUrl, labelsOf, lightPicture, svgUrl, textOfDataUrl } from '@/lib/drawio'
import { fileName } from '@/lib/markdown'
import { ApiError, CLIENT_ID } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// One diagram, as big as the window: its name, a menu (its link; download
// as .drawio, an editable .svg or .png; delete), the draw.io editor. It
// saves itself a moment after each change — the file, a picture of it (for
// lists and pages) and its labels (for search); a save over someone else's
// newer one is refused, and theirs is shown. Viewers see the picture.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const id = computed(() => String(route.params.id))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const doc = ref<DiagramDoc | null>(null)
const title = ref('')
const missing = ref(false)
const xml = ref('') // the file as the editor last handed it over
async function load() {
  try {
    const d = await diagramsApi.get(id.value)
    doc.value = d
    title.value = d.title
    xml.value = d.data ?? ''
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 404) missing.value = true
    else toast.error(e.message)
  }
}
watch(id, load, { immediate: true })

const editor = ref<InstanceType<typeof DrawioFrame> | null>(null)

// ---- Saving ----
const state = ref<'idle' | 'waiting' | 'saving' | 'saved' | 'error'>('idle')
let timer: ReturnType<typeof setTimeout> | undefined
let running: Promise<void> = Promise.resolve()
function changed(x: string) {
  if (x === xml.value) return
  xml.value = x
  state.value = 'waiting'
  clearTimeout(timer)
  timer = setTimeout(() => void save(), 1200)
}
// The picture: the editor makes it (none when it can't).
async function picture(): Promise<string | undefined> {
  try {
    return lightPicture(textOfDataUrl(await editor.value!.exportAs('svg')))
  } catch {
    return undefined
  }
}
function save(extra: { title?: string } = {}): Promise<void> {
  clearTimeout(timer)
  const run = running.then(async () => {
    const d = doc.value
    if (!d || !editable.value) return
    const withData = state.value === 'waiting'
    if (!withData && extra.title === undefined) return
    state.value = 'saving'
    try {
      const data = xml.value
      const body = withData ? { data, preview: await picture(), text: labelsOf(data) } : {}
      const saved = await diagramsApi.save(d.id, { ...body, ...extra, version: d.version })
      doc.value = { ...saved, data }
      if (state.value === 'saving') state.value = 'saved'
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 409 && e.body?.id) {
        // Saved elsewhere meanwhile: theirs wins; show it.
        const theirs = e.body as DiagramDoc
        doc.value = theirs
        title.value = theirs.title
        xml.value = theirs.data ?? ''
        editor.value?.load(xml.value)
        state.value = 'idle'
        toast.info(t('diagrams.changedElsewhere'))
        return
      }
      state.value = 'error'
      toast.error(e.message)
    }
  })
  running = run
  return run
}
async function flush() {
  if (state.value === 'waiting') await save()
  else await running
}
onBeforeRouteLeave(() => flush())
onBeforeUnmount(() => void flush())
function onUnload(e: BeforeUnloadEvent) {
  if (state.value !== 'waiting' && state.value !== 'saving') return
  void flush()
  e.preventDefault()
}
window.addEventListener('beforeunload', onUnload)
onUnmounted(() => window.removeEventListener('beforeunload', onUnload))

function saveTitle() {
  const v = title.value.trim()
  if (!doc.value || v === doc.value.title) return
  void save({ title: v })
}

// Someone else's save: take it when nothing of ours is waiting.
const off = onLiveEvent('ws', async (e) => {
  const d = doc.value
  if (!d || e.origin === CLIENT_ID || e.what !== 'diagrams' || String(e.ws) !== ws.value || state.value === 'waiting' || state.value === 'saving') return
  try {
    const fresh = await diagramsApi.get(d.id)
    if (fresh.version > d.version) {
      doc.value = fresh
      title.value = fresh.title
      xml.value = fresh.data ?? ''
      editor.value?.load(xml.value)
    }
  } catch {
    /* gone: the next load says so */
  }
})
onUnmounted(off)

// ---- Downloads ----
const menuEl = ref<HTMLElement | null>(null)
const menu = ref(false)
function saveFile(blob: Blob, ext: string) {
  const url = URL.createObjectURL(blob)
  const a = Object.assign(document.createElement('a'), { href: url, download: `${fileName(title.value || t('diagrams.untitled'))}.${ext}` })
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
async function download(kind: 'drawio' | 'svg' | 'png') {
  menu.value = false
  try {
    if (kind === 'drawio') return saveFile(new Blob([xml.value], { type: 'application/vnd.jgraph.mxfile' }), 'drawio')
    if (!editor.value) {
      // Viewers: the picture there is.
      return saveFile(new Blob([doc.value?.preview ?? ''], { type: 'image/svg+xml' }), 'svg')
    }
    const data = await editor.value.exportAs(kind === 'svg' ? 'xmlsvg' : 'xmlpng', kind === 'png' ? { scale: 2 } : {})
    const { bytes, type } = bytesOfDataUrl(data)
    saveFile(new Blob([bytes as BlobPart], { type }), kind)
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function copyLink() {
  menu.value = false
  await navigator.clipboard.writeText(`${location.origin}/w/${ws.value}/d/${id.value}`)
  toast.success(t('diagrams.linkCopied'))
}
const confirmDelete = ref(false)
async function remove() {
  if (!doc.value) return
  try {
    clearTimeout(timer)
    state.value = 'idle'
    await diagramsApi.remove(doc.value.id)
    void router.push({ name: 'ws-diagrams', params: { ws: ws.value } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

const fill = ref<HTMLElement | null>(null)
useFillHeight(fill, 480)
const status = computed(() =>
  state.value === 'waiting' || state.value === 'saving' ? t('common.saving') : state.value === 'saved' ? t('common.saved') : state.value === 'error' ? t('editor.saveFailed') : '',
)
</script>

<template>
  <div class="diagram-view">
    <header class="mb-3 flex items-center gap-2">
      <RouterLink :to="{ name: 'ws-diagrams', params: { ws } }" class="btn-plain -ml-2 shrink-0"><ChevronLeft class="h-4 w-4" />{{ t('modules.diagrams') }}</RouterLink>
      <input
        v-model="title"
        class="min-w-0 flex-1 rounded-lg bg-transparent px-2 py-1 text-[20px] font-bold outline-none hover:bg-fill-2 focus:bg-fill-2"
        :placeholder="t('diagrams.untitled')"
        :readonly="!editable"
        maxlength="300"
        data-testid="diagram-title"
        @keydown.enter="($event.target as HTMLInputElement).blur()"
        @blur="saveTitle"
      />
      <span v-if="status" class="flex shrink-0 items-center gap-1 text-[12px] text-text-3" data-testid="diagram-state">
        <LoaderCircle v-if="state === 'waiting' || state === 'saving'" class="h-3.5 w-3.5 animate-spin" />
        <Check v-else-if="state === 'saved'" class="h-3.5 w-3.5" />
        {{ status }}
      </span>
      <button ref="menuEl" type="button" class="icon-btn shrink-0" :aria-label="t('diagrams.more')" data-testid="diagram-menu" @click="menu = !menu"><MoreHorizontal class="h-[18px] w-[18px]" /></button>
    </header>
    <p v-if="missing" class="py-24 text-center text-[14px] text-text-3">{{ t('diagrams.notFound') }}</p>
    <div v-else ref="fill" class="diagram-frame">
      <template v-if="doc">
        <DrawioFrame v-if="editable" ref="editor" :xml="xml" :title="title" @change="changed" />
        <div v-else class="paper">
          <img v-if="doc.preview" :src="svgUrl(doc.preview)" :alt="title" class="max-h-full max-w-full" />
          <p v-else class="text-[13.5px] text-text-3">{{ t('diagrams.noPicture') }}</p>
        </div>
      </template>
    </div>

    <Popover v-if="menu" :anchor="menuEl" :width="250" placement="bottom-end" @close="menu = false">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full" @click="copyLink"><Link class="h-4 w-4 text-text-2" />{{ t('diagrams.copyLink') }}</button>
        <button type="button" class="menu-item w-full" :disabled="!xml" data-testid="diagram-drawio" @click="download('drawio')"><Download class="h-4 w-4 text-text-2" />{{ t('diagrams.downloadDrawio') }}</button>
        <button type="button" class="menu-item w-full" data-testid="diagram-svg" @click="download('svg')"><Download class="h-4 w-4 text-text-2" />{{ t('diagrams.downloadSvg') }}</button>
        <button v-if="editable" type="button" class="menu-item w-full" data-testid="diagram-png" @click="download('png')"><Download class="h-4 w-4 text-text-2" />{{ t('diagrams.downloadPng') }}</button>
        <template v-if="editable">
          <div class="menu-sep" />
          <button type="button" class="menu-item danger w-full" @click="(menu = false), (confirmDelete = true)"><Trash2 class="h-4 w-4" />{{ t('diagrams.delete') }}</button>
        </template>
      </div>
    </Popover>
    <Confirm :open="confirmDelete" :title="t('diagrams.deleteTitle', { name: title || t('diagrams.untitled') })" :message="t('diagrams.deleteBody')" :confirm-label="t('diagrams.delete')" danger @confirm="remove" @close="confirmDelete = false" />
  </div>
</template>

<style scoped>
.diagram-view {
  padding: 16px clamp(12px, 2vw, 24px) 0;
}
.diagram-frame {
  display: flex;
  flex-direction: column;
  min-height: 480px;
}
.paper {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  overflow: auto;
  padding: 24px;
  border: 0.5px solid var(--border);
  border-radius: 12px;
  background: #fff;
}
</style>

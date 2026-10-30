<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ChevronDown, GitFork, Plus, Upload, Workflow } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import FileBrowser from '@/components/files/FileBrowser.vue'
import { diagramsApi } from '@/lib/diagramsApi'
import { fileOf, labelsOf, lightPicture } from '@/lib/drawio'
import { MERMAID_TEMPLATES } from '@/lib/mermaid'
import { bringIn } from '@/lib/fileOps'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's diagrams (draw.io and Mermaid), in its folders, each with
// its picture; a new one (in the folder shown), or one from a file (.drawio,
// an .svg draw.io saved with its diagram inside, or Mermaid's .mmd).
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const browser = ref<InstanceType<typeof FileBrowser> | null>(null)
const folder = () => browser.value?.here ?? ''
const open = (id: string) => router.push({ name: 'ws-diagram', params: { ws: ws.value, id } })
const newEl = ref<HTMLElement | null>(null)
const newOpen = ref(false)
async function create(kind: 'drawio' | 'mermaid') {
  newOpen.value = false
  try {
    const d = await diagramsApi.create(ws.value, { title: '', folder_id: folder(), kind, data: kind === 'mermaid' ? MERMAID_TEMPLATES[0].code : undefined })
    await open(d.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Import: a draw.io file becomes a new diagram (its picture is made when
// it's first opened and changed; an .svg brings its own).
const fileEl = ref<HTMLInputElement | null>(null)
const importing = ref(false)
async function imported(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  ;(e.target as HTMLInputElement).value = ''
  if (!f) return
  importing.value = true
  try {
    if (/\.(mmd|mermaid)$/i.test(f.name)) {
      const d = await bringIn(ws.value, f, folder(), { tab: '', notDrawio: t('diagrams.notDrawio'), notExcalidraw: '', notMermaid: t('mermaid.notMermaid') })
      toast.success(t('diagrams.imported', { name: d.title }))
      await open(d.id)
      return
    }
    const text = await f.text()
    const data = fileOf(text)
    if (!data) throw new Error(t('diagrams.notDrawio'))
    const name = f.name.replace(/\.(drawio|xml|svg|dio)$/i, '').replace(/\.drawio$/i, '')
    const d = await diagramsApi.create(ws.value, { title: name, data, text: labelsOf(data), preview: /\.svg$/i.test(f.name) ? lightPicture(text) : undefined, folder_id: folder() })
    toast.success(t('diagrams.imported', { name }))
    await open(d.id)
  } catch (err: any) {
    toast.error(t('diagrams.importFailed', { error: err?.message ?? String(err) }))
  } finally {
    importing.value = false
  }
}

</script>

<template>
  <div class="diagrams-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.diagrams') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }}</p>
      </div>
      <template v-if="editable">
        <input ref="fileEl" type="file" class="hidden" accept=".drawio,.dio,.xml,.svg,.mmd,.mermaid,application/xml,image/svg+xml" data-testid="diagram-import-input" @change="imported" />
        <button class="btn btn-ghost" :disabled="importing" :title="t('diagrams.importHint')" @click="fileEl?.click()"><Upload class="h-4 w-4" />{{ t('diagrams.import') }}</button>
        <button ref="newEl" class="btn-primary" data-testid="new-diagram" @click="newOpen = !newOpen"><Plus class="h-4 w-4" />{{ t('diagrams.new') }}<ChevronDown class="h-3.5 w-3.5" /></button>
      </template>
    </header>

    <Popover v-if="newOpen" :anchor="newEl" placement="bottom-end" :width="260" @close="newOpen = false">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full !items-start" data-testid="new-drawio" @click="create('drawio')">
          <Workflow class="mt-0.5 h-4 w-4 text-text-2" />
          <span class="text-left"><span class="block">{{ t('mermaid.newDrawio') }}</span><span class="block text-[12px] text-text-3">{{ t('mermaid.newDrawioHint') }}</span></span>
        </button>
        <button type="button" class="menu-item w-full !items-start" data-testid="new-mermaid" @click="create('mermaid')">
          <GitFork class="mt-0.5 h-4 w-4 text-text-2" />
          <span class="text-left"><span class="block">{{ t('mermaid.newMermaid') }}</span><span class="block text-[12px] text-text-3">{{ t('mermaid.newMermaidHint') }}</span></span>
        </button>
      </div>
    </Popover>

    <FileBrowser ref="browser" :ws="ws" :kinds="['diagram']" :editable="editable" :root-name="t('modules.diagrams')">
      <template #empty>{{ t('diagrams.noneHint') }}</template>
    </FileBrowser>
  </div>
</template>

<style scoped>
.diagrams-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
</style>

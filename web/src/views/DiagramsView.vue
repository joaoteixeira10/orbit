<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Upload } from 'lucide-vue-next'
import FileBrowser from '@/components/files/FileBrowser.vue'
import { diagramsApi } from '@/lib/diagramsApi'
import { fileOf, labelsOf, lightPicture } from '@/lib/drawio'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's diagrams (draw.io), in its folders, each with its picture;
// a new one (in the folder shown), or one from a draw.io file (.drawio, or
// an .svg draw.io saved with its diagram inside).
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
async function create() {
  try {
    const d = await diagramsApi.create(ws.value, { title: '', folder_id: folder() })
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
        <input ref="fileEl" type="file" class="hidden" accept=".drawio,.dio,.xml,.svg,application/xml,image/svg+xml" data-testid="diagram-import-input" @change="imported" />
        <button class="btn btn-ghost" :disabled="importing" :title="t('diagrams.importHint')" @click="fileEl?.click()"><Upload class="h-4 w-4" />{{ t('diagrams.import') }}</button>
        <button class="btn-primary" data-testid="new-diagram" @click="create"><Plus class="h-4 w-4" />{{ t('diagrams.new') }}</button>
      </template>
    </header>

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

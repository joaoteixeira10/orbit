<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { MoreHorizontal, Plus, Trash2, Upload, Workflow } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { diagramsApi, type DiagramSummary } from '@/lib/diagramsApi'
import { fileOf, labelsOf, lightPicture, svgUrl } from '@/lib/drawio'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's diagrams (draw.io), newest change first, each with its
// picture; a new one, or one from a draw.io file (.drawio, or an .svg
// draw.io saved with its diagram inside).
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const list = ref<DiagramSummary[]>([])
const loaded = ref(false)
async function load() {
  try {
    list.value = await diagramsApi.list(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(
  ws,
  () => {
    loaded.value = false
    void load()
  },
  { immediate: true },
)
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && e.what === 'diagrams') void load()
})
onUnmounted(off)

const open = (id: string) => router.push({ name: 'ws-diagram', params: { ws: ws.value, id } })
async function create() {
  try {
    const d = await diagramsApi.create(ws.value, { title: '' })
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
    const d = await diagramsApi.create(ws.value, { title: name, data, text: labelsOf(data), preview: /\.svg$/i.test(f.name) ? lightPicture(text) : undefined })
    toast.success(t('diagrams.imported', { name }))
    await open(d.id)
  } catch (err: any) {
    toast.error(t('diagrams.importFailed', { error: err?.message ?? String(err) }))
  } finally {
    importing.value = false
  }
}

const menu = ref<{ d: DiagramSummary; el: HTMLElement } | null>(null)
const removing = ref<DiagramSummary | null>(null)
async function remove() {
  const d = removing.value
  removing.value = null
  if (!d) return
  try {
    await diagramsApi.remove(d.id)
    list.value = list.value.filter((x) => x.id !== d.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="diagrams-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.diagrams') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }} · {{ list.length }}</p>
      </div>
      <template v-if="editable">
        <input ref="fileEl" type="file" class="hidden" accept=".drawio,.dio,.xml,.svg,application/xml,image/svg+xml" data-testid="diagram-import-input" @change="imported" />
        <button class="btn btn-ghost" :disabled="importing" :title="t('diagrams.importHint')" @click="fileEl?.click()"><Upload class="h-4 w-4" />{{ t('diagrams.import') }}</button>
        <button class="btn-primary" data-testid="new-diagram" @click="create"><Plus class="h-4 w-4" />{{ t('diagrams.new') }}</button>
      </template>
    </header>

    <div v-if="loaded && !list.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #ff9500"><Workflow class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('diagrams.none') }}</p>
      <p class="mt-1 max-w-[380px] text-[13px] text-text-2">{{ t('diagrams.noneHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="create"><Plus class="h-4 w-4" />{{ t('diagrams.new') }}</button>
    </div>

    <div class="grid">
      <div v-for="d in list" :key="d.id" class="card-d" role="link" tabindex="0" data-testid="diagram-card" @click="open(d.id)" @keydown.enter="open(d.id)">
        <div class="pic">
          <img v-if="d.preview" :src="svgUrl(d.preview)" alt="" loading="lazy" />
          <Workflow v-else class="h-8 w-8 text-[#c7c7cc]" />
        </div>
        <div class="flex items-center gap-2 px-3 py-2.5">
          <div class="min-w-0 flex-1">
            <div class="truncate text-[14px] font-medium">{{ d.title || t('diagrams.untitled') }}</div>
            <div class="text-[12px] text-text-3">{{ t('diagrams.edited', { when: ago(d.updated_at, locale()) }) }}</div>
          </div>
          <button v-if="editable" class="icon-btn h-7 w-7 shrink-0" :aria-label="t('diagrams.more')" @click.stop="menu = { d, el: $event.currentTarget as HTMLElement }"><MoreHorizontal class="h-4 w-4" /></button>
        </div>
      </div>
    </div>

    <Popover v-if="menu" :anchor="menu.el" placement="bottom-end" :width="200" @close="menu = null">
      <button class="menu-item danger" @click="(removing = menu.d), (menu = null)"><Trash2 class="h-4 w-4" />{{ t('diagrams.delete') }}</button>
    </Popover>
    <Confirm :open="!!removing" :title="t('diagrams.deleteTitle', { name: removing?.title || t('diagrams.untitled') })" :message="t('diagrams.deleteBody')" :confirm-label="t('diagrams.delete')" danger @confirm="remove" @close="removing = null" />
  </div>
</template>

<style scoped>
.diagrams-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 16px;
}
.card-d {
  overflow: hidden;
  cursor: pointer;
  border: 0.5px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
  transition: box-shadow var(--speed);
}
.card-d:hover,
.card-d:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--ring);
}
/* Diagrams are drawn on white (as draw.io shows them). */
.pic {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 150px;
  padding: 10px;
  background: #fff;
  border-bottom: 0.5px solid var(--border);
}
.pic img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}
</style>

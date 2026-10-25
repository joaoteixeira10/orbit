<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Plus, MoreHorizontal, Pencil, Trash2, Brush, FolderInput } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import FileBrowser from '@/components/files/FileBrowser.vue'
import NotePlacePicker from '@/components/notes/NotePlacePicker.vue'
import { drawingsApi, previewSrc, type Drawing } from '@/components/draw/drawing'
import { emptySketch } from '@/lib/sketch'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's whiteboards, in its folders (the file browser) — or, with no
// workspace in the address, Quick Draw: one's own sketches, a card each with
// its picture, newest change first. A click opens it, full size.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const personal = computed(() => !route.params.ws)
const ws = computed(() => (personal.value ? 'me' : String(route.params.ws)))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() =>
  personal.value ? !auth.privileges.read_only : !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only,
)

const list = ref<Drawing[]>([])
const loaded = ref(false)
async function load() {
  if (!personal.value) return // the file browser lists them
  try {
    list.value = await drawingsApi.list(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, () => {
  loaded.value = false
  void load()
}, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && e.what === 'drawings') void load()
})
onUnmounted(off)

const browser = ref<InstanceType<typeof FileBrowser> | null>(null)
const open = (d: Drawing) =>
  personal.value ? router.push({ name: 'my-drawing', params: { id: d.id } }) : router.push({ name: 'ws-whiteboard', params: { ws: ws.value, id: d.id } })
async function create() {
  try {
    // Quick Draw's sketches have its own canvas; whiteboards are Excalidraw's
    // (made in the folder shown).
    const d = personal.value
      ? await drawingsApi.create(ws.value, '', JSON.stringify(emptySketch(t('sketch.layerN', { n: 1 }))))
      : await drawingsApi.make(ws.value, { title: '', folder_id: browser.value?.here ?? '' })
    await open(d)
  } catch (e: any) {
    toast.error(e.message)
  }
}

const menu = ref<{ d: Drawing; el: HTMLElement } | null>(null)
const renaming = ref('')
const name = ref('')
function startRename(d: Drawing) {
  menu.value = null
  renaming.value = d.id
  name.value = d.title
}
async function rename(d: Drawing) {
  renaming.value = ''
  if (name.value.trim() === d.title) return
  try {
    const saved = await drawingsApi.save(d.id, { title: name.value.trim(), version: d.version })
    Object.assign(d, { title: saved.title, version: saved.version })
  } catch (e: any) {
    toast.error(e.message)
    void load()
  }
}
const removing = ref<Drawing | null>(null)
async function remove() {
  const d = removing.value
  removing.value = null
  if (!d) return
  try {
    await drawingsApi.remove(d.id)
    list.value = list.value.filter((x) => x.id !== d.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// A sketch into a workspace (as one of its whiteboards).
const moving = ref<Drawing | null>(null)
async function moveTo(target: string) {
  const d = moving.value
  moving.value = null
  if (!d) return
  try {
    await drawingsApi.move(d.id, target)
    list.value = list.value.filter((x) => x.id !== d.id)
    toast.success(t('draw.moved', { where: wss.byId(target)?.name ?? '' }))
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="boards-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ personal ? t('draw.quick') : t('modules.whiteboards') }}</h1>
        <p class="subhead mt-0.5">{{ personal ? t('draw.quickSub') : workspace?.name }}</p>
      </div>
      <button v-if="editable" class="btn-primary" data-testid="new-whiteboard" @click="create"><Plus class="h-4 w-4" />{{ personal ? t('draw.newSketch') : t('draw.new') }}</button>
    </header>

    <FileBrowser v-if="!personal" ref="browser" :ws="ws" :kinds="['board']" :editable="editable" :root-name="t('modules.whiteboards')">
      <template #empty>{{ t('draw.noneHint') }}</template>
    </FileBrowser>

    <div v-else-if="loaded && !list.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #ff9500"><Brush class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ personal ? t('draw.noneMine') : t('draw.none') }}</p>
      <p class="mt-1 max-w-[340px] text-[13px] text-text-2">{{ t('draw.noneHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="create"><Plus class="h-4 w-4" />{{ personal ? t('draw.newSketch') : t('draw.new') }}</button>
    </div>

    <div v-if="personal" class="grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-4">
      <div v-for="d in list" :key="d.id" class="board card" role="link" tabindex="0" data-testid="whiteboard-card" @click="renaming !== d.id && open(d)" @keydown.enter="open(d)">
        <div class="thumb">
          <img v-if="d.preview" :src="previewSrc(d.preview)" alt="" draggable="false" />
          <Brush v-else class="h-8 w-8 text-text-faint" :stroke-width="1.5" />
        </div>
        <div class="flex items-start gap-2 px-3.5 py-2.5">
          <div class="min-w-0 flex-1">
            <input v-if="renaming === d.id" v-model="name" class="input !py-0.5 text-[14px]" maxlength="300" autofocus @click.stop @keydown.enter="rename(d)" @keydown.esc="renaming = ''" @blur="rename(d)" />
            <div v-else class="truncate text-[14px] font-medium">{{ d.title || t('draw.untitled') }}</div>
            <div class="text-[12px] text-text-3">{{ t('draw.edited', { when: ago(d.updated_at, locale()) }) }}</div>
          </div>
          <button v-if="editable" class="icon-btn -mr-1.5 h-7 w-7 shrink-0" :aria-label="t('common.more')" @click.stop="menu = { d, el: $event.currentTarget as HTMLElement }"><MoreHorizontal class="h-4 w-4" /></button>
        </div>
      </div>
    </div>

    <Popover v-if="menu" :anchor="menu.el" placement="bottom-end" :width="200" @close="menu = null">
      <button class="menu-item" @click="startRename(menu.d)"><Pencil class="h-4 w-4 text-text-2" />{{ t('draw.rename') }}</button>
      <button class="menu-item" data-testid="drawing-move" @click="(moving = menu.d), (menu = null)"><FolderInput class="h-4 w-4 text-text-2" />{{ t('draw.moveTo') }}</button>
      <button class="menu-item danger" @click="removing = menu.d; menu = null"><Trash2 class="h-4 w-4" />{{ t('draw.delete') }}</button>
    </Popover>
    <NotePlacePicker :open="!!moving" mode="move" drawing current="me" @close="moving = null" @pick="moveTo" />
    <Confirm
      :open="!!removing"
      :title="t('draw.deleteTitle', { name: removing?.title || t('draw.untitled') })"
      :message="t('draw.deleteBody')"
      :confirm-label="t('common.delete')"
      danger
      @confirm="remove"
      @close="removing = null"
    />
  </div>
</template>

<style scoped>
.boards-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.board {
  overflow: hidden;
  cursor: pointer;
  transition: transform 0.15s var(--spring), box-shadow 0.15s;
}
.board:hover {
  transform: translateY(-2px);
}
.thumb {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 150px;
  overflow: hidden;
  padding: 10px;
  background: #fff;
  border-bottom: 0.5px solid var(--border);
}
/* The picture fills the box and is scaled to fit it (a sketch's picture is
   drawn at its full size). */
.thumb img {
  width: 100%;
  height: 100%;
  min-height: 0;
  object-fit: contain;
}
</style>

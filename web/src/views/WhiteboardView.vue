<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft, FolderInput } from 'lucide-vue-next'
import NotePlacePicker from '@/components/notes/NotePlacePicker.vue'
import { drawingsApi } from '@/components/draw/drawing'
import { useToastStore } from '@/stores/toast'
import DrawingEditor from '@/components/draw/DrawingEditor.vue'
import type { Drawing } from '@/components/draw/drawing'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'

// One whiteboard, as big as the window: its name on top, Excalidraw below.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const { t } = useI18n()

// No workspace in the address: a Quick Draw sketch (one's own).
const personal = computed(() => !route.params.ws)
const ws = computed(() => String(route.params.ws ?? ''))
const id = computed(() => String(route.params.id))
const workspace = computed(() => wss.byId(ws.value))
const readonly = computed(() =>
  personal.value ? auth.privileges.read_only : !workspace.value || workspace.value.role === 'viewer' || !!workspace.value.archived_at || auth.privileges.read_only,
)

const editor = ref<InstanceType<typeof DrawingEditor> | null>(null)
const title = ref('')
const loadedTitle = ref('')
function loaded(d: Drawing) {
  title.value = loadedTitle.value = d.title
}
async function saveTitle() {
  if (title.value.trim() === loadedTitle.value) return
  loadedTitle.value = title.value.trim()
  await editor.value?.save({ title: loadedTitle.value })
}
async function back() {
  await editor.value?.flush()
  void router.push(personal.value ? { name: 'my-drawings' } : { name: 'ws-whiteboards', params: { ws: ws.value } })
}

// Move: a sketch into a workspace, a whiteboard to another one or to Quick
// Draw; then it opens where it went.
const toast = useToastStore()
const moving = ref(false)
async function moveTo(target: string) {
  moving.value = false
  try {
    await editor.value?.flush()
    await drawingsApi.move(id.value, target)
    toast.success(t('draw.moved', { where: target === 'me' ? t('draw.quick') : (wss.byId(target)?.name ?? '') }))
    void router.replace(target === 'me' ? { name: 'my-drawing', params: { id: id.value } } : { name: 'ws-whiteboard', params: { ws: target, id: id.value } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
const fill = ref<HTMLElement | null>(null)
useFillHeight(fill, 420)
</script>

<template>
  <div class="board-view">
    <header class="mb-3 flex items-center gap-2">
      <button class="btn-plain -ml-2 shrink-0" data-testid="whiteboard-back" @click="back"><ChevronLeft class="h-4 w-4" />{{ personal ? t('draw.quick') : t('draw.back') }}</button>
      <input
        v-model="title"
        class="min-w-0 flex-1 rounded-lg bg-transparent px-2 py-1 text-[20px] font-bold outline-none hover:bg-fill-2 focus:bg-fill-2"
        :placeholder="t('draw.titlePh')"
        :readonly="readonly"
        maxlength="300"
        data-testid="whiteboard-title"
        @keydown.enter="($event.target as HTMLInputElement).blur()"
        @blur="saveTitle"
      />
      <button v-if="!readonly" class="btn btn-sm shrink-0" :title="t('draw.moveTo')" data-testid="whiteboard-move" @click="moving = true"><FolderInput class="h-3.5 w-3.5" /><span class="hidden sm:inline">{{ t('draw.moveTo') }}</span></button>
    </header>
    <NotePlacePicker :open="moving" mode="move" drawing :current="personal ? 'me' : ws" @close="moving = false" @pick="moveTo" />
    <div ref="fill" class="board-frame">
      <DrawingEditor ref="editor" :id="id" :readonly="readonly" @loaded="loaded" />
    </div>
  </div>
</template>

<style scoped>
.board-view {
  padding: 16px clamp(12px, 2vw, 24px) 0;
}
.board-frame {
  overflow: hidden;
  border-radius: 14px;
  box-shadow: 0 0 0 0.5px var(--border);
}
</style>

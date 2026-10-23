<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ChevronLeft } from 'lucide-vue-next'
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

const ws = computed(() => String(route.params.ws))
const id = computed(() => String(route.params.id))
const workspace = computed(() => wss.byId(ws.value))
const readonly = computed(() => !workspace.value || workspace.value.role === 'viewer' || !!workspace.value.archived_at || auth.privileges.read_only)

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
  void router.push({ name: 'ws-whiteboards', params: { ws: ws.value } })
}

const fill = ref<HTMLElement | null>(null)
useFillHeight(fill, 420)
</script>

<template>
  <div class="board-view">
    <header class="mb-3 flex items-center gap-2">
      <button class="btn-plain -ml-2 shrink-0" data-testid="whiteboard-back" @click="back"><ChevronLeft class="h-4 w-4" />{{ t('draw.back') }}</button>
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
    </header>
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

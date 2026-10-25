<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Brush, Check, NotebookPen } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'

// Where a note goes: another workspace (or back to Quick Notes) for "Move
// to…", or the workspace that gets its tasks or its page. Only workspaces
// the user can edit (and, for tasks or a page, that have that module).
// drawing: where a Quick Draw sketch or a whiteboard goes (workspaces with
// whiteboards, or back to Quick Draw).
const props = defineProps<{ open: boolean; mode: 'move' | 'tasks' | 'page'; current: string; drawing?: boolean }>()
const emit = defineEmits<{ close: []; pick: [ws: string] }>()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const { t } = useI18n()

const choices = computed(() =>
  wss.active.filter(
    (w) =>
      w.role !== 'viewer' &&
      (props.drawing ? w.modules.includes('whiteboards') : props.mode === 'move' || w.modules.includes(props.mode === 'tasks' ? 'tasks' : 'pages')),
  ),
)
const title = computed(() =>
  t(props.drawing ? 'draw.pickMove' : props.mode === 'move' ? 'notes.mine.pickMove' : props.mode === 'tasks' ? 'notes.mine.pickTasks' : 'notes.mine.pickPage'),
)
</script>

<template>
  <Overlay :open="open" size="sm" :title="title" @close="emit('close')">
    <div class="group">
      <button v-if="mode === 'move' && current !== 'me' && !auth.privileges.read_only" class="group-row w-full text-left" @click="emit('pick', 'me')">
        <span v-if="drawing" class="tile h-7 w-7" style="--tile: #ff9500"><Brush class="h-4 w-4" /></span>
        <span v-else class="tile h-7 w-7" style="--tile: #ffcc00"><NotebookPen class="h-4 w-4" /></span>
        <span class="min-w-0 flex-1">
          <span class="block text-[14px]">{{ drawing ? t('draw.quick') : t('notes.mine.title') }}</span>
          <span class="block text-[12px] text-text-3">{{ t('notes.mine.onlyYou') }}</span>
        </span>
      </button>
      <button v-for="w in choices" :key="w.id" class="group-row w-full text-left" :disabled="w.id === current" @click="emit('pick', w.id)">
        <WsIcon :icon="w.icon" :color="w.color" :size="28" :name="w.name" />
        <span class="min-w-0 flex-1 truncate text-[14px]">{{ w.name }}</span>
        <Check v-if="w.id === current" class="h-4 w-4 text-accent" />
      </button>
      <div v-if="!choices.length" class="group-row text-[13px] text-text-3">{{ drawing ? t('draw.noTarget') : t('notes.mine.noTarget') }}</div>
    </div>
  </Overlay>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, LoaderCircle, TriangleAlert } from 'lucide-vue-next'
import ExcalidrawCanvas from './ExcalidrawCanvas.vue'
import { drawingsApi, type Drawing } from './drawing'
import { ApiError, CLIENT_ID } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'
import { useToastStore } from '@/stores/toast'

// A drawing, open: it saves itself a moment after each change (and when it
// closes). Saves carry the version they change, so two people drawing at
// once don't overwrite each other blindly: the later save is refused, and
// the newer drawing replaces this one (with a word saying so). A change
// saved elsewhere shows here when nothing's waiting to be saved.
const props = defineProps<{ id: string; readonly?: boolean }>()
const emit = defineEmits<{ loaded: [d: Drawing]; saved: [d: Drawing] }>()
const { t } = useI18n()
const toast = useToastStore()

const drawing = ref<Drawing | null>(null)
const canvas = ref<InstanceType<typeof ExcalidrawCanvas> | null>(null)
const state = ref<'idle' | 'waiting' | 'saving' | 'saved' | 'error'>('idle')
let timer: ReturnType<typeof setTimeout> | undefined
let running: Promise<void> = Promise.resolve()

async function load() {
  try {
    drawing.value = await drawingsApi.get(props.id)
    emit('loaded', drawing.value)
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch(() => props.id, load, { immediate: true })

function changed() {
  if (props.readonly) return
  state.value = 'waiting'
  clearTimeout(timer)
  timer = setTimeout(() => void save(), 1200)
}

// save sends the drawing as it is now (one save at a time).
function save(extra: { title?: string } = {}): Promise<void> {
  clearTimeout(timer)
  timer = undefined
  const run = running.then(async () => {
    const d = drawing.value
    if (!d || props.readonly) return
    const snap = state.value === 'waiting' ? await canvas.value?.snapshot() : null
    if (!snap && extra.title === undefined) return
    state.value = 'saving'
    try {
      const saved = await drawingsApi.save(d.id, { ...(snap ?? {}), ...extra, version: d.version })
      drawing.value = { ...d, ...saved, scene: saved.scene ?? d.scene }
      state.value = 'saved'
      emit('saved', drawing.value)
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 409 && e.body?.id) {
        // Saved elsewhere meanwhile: theirs wins; show it.
        drawing.value = e.body as Drawing
        canvas.value?.replace(e.body.scene ?? '')
        state.value = 'idle'
        toast.info(t('draw.changedElsewhere'))
        return
      }
      state.value = 'error'
      toast.error(e.message)
    }
  })
  running = run
  return run
}

// flush saves now what's waiting (before closing).
async function flush() {
  if (state.value === 'waiting') await save()
  else await running
}
defineExpose({ flush, save, drawing })
onBeforeUnmount(() => void flush())

// Someone else's save: take it if nothing of ours is waiting.
const off = onLiveEvent('ws', async (e) => {
  const d = drawing.value
  if (!d || e.origin === CLIENT_ID || (e.what !== 'drawings' && e.what !== 'notes') || state.value === 'waiting' || state.value === 'saving') return
  try {
    const fresh = await drawingsApi.get(d.id)
    if (fresh.version > d.version) {
      drawing.value = fresh
      canvas.value?.replace(fresh.scene ?? '')
    }
  } catch {
    /* gone: the page says so when it reloads */
  }
})
onUnmounted(off)

const status = computed(() =>
  state.value === 'waiting' || state.value === 'saving' ? t('draw.saving') : state.value === 'saved' ? t('draw.saved') : state.value === 'error' ? t('draw.notSaved') : '',
)
</script>

<template>
  <div class="relative h-full w-full">
    <ExcalidrawCanvas v-if="drawing" ref="canvas" :scene="drawing.scene ?? ''" :readonly="readonly" :name="drawing.title" @change="changed" />
    <span v-if="status" class="save-state" :class="state" data-testid="draw-state">
      <LoaderCircle v-if="state === 'waiting' || state === 'saving'" class="h-3.5 w-3.5 animate-spin" />
      <Check v-else-if="state === 'saved'" class="h-3.5 w-3.5" />
      <TriangleAlert v-else class="h-3.5 w-3.5" />
      {{ status }}
    </span>
  </div>
</template>

<style scoped>
.save-state {
  position: absolute;
  left: 50%;
  bottom: 14px;
  transform: translateX(-50%);
  z-index: 5;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--bg) 85%, transparent);
  box-shadow: 0 0 0 0.5px var(--border);
  font-size: 12px;
  color: var(--text-2);
  pointer-events: none;
}
.save-state.error {
  color: var(--crit-text);
}
</style>

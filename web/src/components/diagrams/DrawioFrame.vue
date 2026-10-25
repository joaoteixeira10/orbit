<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle, TriangleAlert } from 'lucide-vue-next'
import { editorUrl, originOf } from '@/lib/drawio'
import { currentLocale } from '@/i18n'
import { useBrandingStore } from '@/stores/branding'

// The draw.io editor in a frame, from the address the instance sets. It
// says when it's ready and hands over the file at each change (it saves
// itself as you go); asked, it makes a picture of the diagram (export).
// Only messages from that frame, at that address, are listened to.
const props = defineProps<{ xml: string; title?: string }>()
const emit = defineEmits<{ change: [xml: string]; ready: [] }>()
const { t } = useI18n()
const branding = useBrandingStore()

const base = computed(() => branding.drawioUrl)
const origin = computed(() => originOf(base.value))
const src = editorUrl(base.value, currentLocale(), document.documentElement.classList.contains('dark'))
const frame = ref<HTMLIFrameElement | null>(null)
const ready = ref(false)
const silent = ref(false) // no answer: the address is wrong, or out of reach

function post(m: Record<string, unknown>) {
  frame.value?.contentWindow?.postMessage(JSON.stringify(m), origin.value)
}

// Exports go one at a time (the answer doesn't say which ask it's for).
let queue: Promise<unknown> = Promise.resolve()
let waiting: ((data: string) => void) | null = null
function exportAs(format: 'svg' | 'xmlsvg' | 'png' | 'xmlpng', extra: Record<string, unknown> = {}): Promise<string> {
  const run = queue.then(
    () =>
      new Promise<string>((resolve, reject) => {
        if (!ready.value) return reject(new Error(t('diagrams.notReady')))
        const timer = setTimeout(() => {
          waiting = null
          reject(new Error(t('diagrams.notReady')))
        }, 15000)
        waiting = (data) => {
          clearTimeout(timer)
          waiting = null
          resolve(data)
        }
        post({ action: 'export', format, border: 8, ...extra })
      }),
  )
  queue = run.catch(() => undefined)
  return run
}
// load shows another file (someone else's newer save).
function load(xml: string) {
  if (ready.value) post({ action: 'load', xml, autosave: 1, title: props.title })
}
defineExpose({ exportAs, load, ready })

function onMessage(e: MessageEvent) {
  if (!frame.value || e.source !== frame.value.contentWindow || e.origin !== origin.value) return
  let m: { event?: string; xml?: string; data?: string }
  try {
    m = typeof e.data === 'string' ? JSON.parse(e.data) : e.data
  } catch {
    return
  }
  switch (m?.event) {
    case 'init':
      clearTimeout(noAnswer)
      silent.value = false
      post({ action: 'load', xml: props.xml, autosave: 1, title: props.title })
      break
    case 'load':
      ready.value = true
      emit('ready')
      break
    case 'autosave':
    case 'save':
      if (typeof m.xml === 'string') emit('change', m.xml)
      break
    case 'export':
      if (typeof m.data === 'string') waiting?.(m.data)
      break
  }
}
let noAnswer: ReturnType<typeof setTimeout> | undefined
onMounted(() => {
  window.addEventListener('message', onMessage)
  noAnswer = setTimeout(() => (silent.value = true), 20000)
})
onUnmounted(() => {
  window.removeEventListener('message', onMessage)
  clearTimeout(noAnswer)
})
</script>

<template>
  <div class="drawio">
    <iframe
      ref="frame"
      :src="src"
      :title="t('diagrams.editor')"
      class="drawio-frame"
      sandbox="allow-scripts allow-same-origin allow-popups allow-downloads allow-forms allow-modals"
      allow="clipboard-read; clipboard-write"
      referrerpolicy="no-referrer"
      data-testid="drawio-frame"
    />
    <div v-if="!ready" class="drawio-wait">
      <template v-if="silent">
        <TriangleAlert class="h-6 w-6 text-warn" />
        <p class="max-w-[420px] text-[13.5px]">{{ t('diagrams.silent', { where: origin || base }) }}</p>
      </template>
      <LoaderCircle v-else class="h-6 w-6 animate-spin text-text-3" />
    </div>
  </div>
</template>

<style scoped>
.drawio {
  position: relative;
  display: flex;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  border: 0.5px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
}
.drawio-frame {
  flex: 1;
  width: 100%;
  border: 0;
}
.drawio-wait {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 24px;
  text-align: center;
  color: var(--text-2);
  background: var(--surface);
}
</style>

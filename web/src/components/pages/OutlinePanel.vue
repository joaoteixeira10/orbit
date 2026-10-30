<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ListTree, X } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import { outlineOf } from '@/lib/outline'
import { ALERTS } from '@/components/editor/alerts'

// Beside a page: its headings, to jump to; and its callouts and checklist
// items gathered in one list — a click goes to one, a box ticks (when the
// page can be changed and isn't being edited). Wide windows show it at the
// side; narrow ones over the page.
const props = defineProps<{ open: boolean; content: string; root: HTMLElement | null; canTick: boolean }>()
const emit = defineEmits<{ close: []; toggle: [index: number] }>()
const { t } = useI18n()

const outline = computed(() => outlineOf(props.content))
const empty = computed(() => !outline.value.headings.length && !outline.value.callouts.length && !outline.value.tasks.length)

const wide = ref(window.matchMedia('(min-width: 1280px)').matches)
const mq = window.matchMedia('(min-width: 1280px)')
const onMq = () => (wide.value = mq.matches)
onMounted(() => mq.addEventListener('change', onMq))
onUnmounted(() => mq.removeEventListener('change', onMq))

// go scrolls to the n-th of what the selector finds in the page, and lights it.
function go(selector: string, n: number) {
  const els = [...(props.root?.querySelectorAll<HTMLElement>(selector) ?? [])].filter((el) => !el.closest('.page-embed'))
  const el = els[n]
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el.classList.add('outline-flash')
  setTimeout(() => el.classList.remove('outline-flash'), 1200)
  if (!wide.value) emit('close')
}
const HEADINGS = '.md-view h1, .md-view h2, .md-view h3, .md-view h4, .md-view h5, .md-view h6, .ProseMirror h1, .ProseMirror h2, .ProseMirror h3'
const CALLOUTS = '.md-alert, .ProseMirror [data-type="callout"]'
const TASKS = '.md-view li:has(> input[data-task]), .md-view li:has(> p > input[data-task]), .ProseMirror li[data-checked]'
const emoji = (kind: string) => ALERTS[kind as keyof typeof ALERTS] ?? '💡'
</script>

<template>
  <component :is="wide ? 'aside' : Overlay" v-if="open" v-bind="wide ? { class: 'outline-aside' } : { open: true, mode: 'side', title: t('outline.title') }" data-testid="outline" @close="emit('close')">
    <div class="ol-body">
      <div v-if="wide" class="mb-2 flex items-center gap-2">
        <ListTree class="h-4 w-4 text-text-3" />
        <h2 class="flex-1 text-[13px] font-semibold">{{ t('outline.title') }}</h2>
        <button type="button" class="icon-btn h-7 w-7" :aria-label="t('common.close')" @click="emit('close')"><X class="h-4 w-4" /></button>
      </div>
      <p v-if="empty" class="text-[12.5px] text-text-3">{{ t('outline.empty') }}</p>
      <nav v-if="outline.headings.length" :aria-label="t('outline.headings')">
        <button
          v-for="h in outline.headings"
          :key="'h' + h.index"
          type="button"
          class="ol-h"
          :style="{ paddingLeft: `${8 + (h.level - 1) * 12}px` }"
          :class="`l${h.level}`"
          data-testid="outline-heading"
          @click="go(HEADINGS, h.index)"
        >
          {{ h.text || t('pages.untitled') }}
        </button>
      </nav>
      <template v-if="outline.callouts.length || outline.tasks.length">
        <h3 class="cap mb-1 mt-4">{{ t('outline.notes') }}</h3>
        <button v-for="c in outline.callouts" :key="'c' + c.index" type="button" class="ol-note" data-testid="outline-callout" @click="go(CALLOUTS, c.index)">
          <span class="w-4 shrink-0 text-center">{{ emoji(c.kind) }}</span><span class="line-clamp-2">{{ c.text || t(`editor.alert.${c.kind}`) }}</span>
        </button>
        <div v-for="x in outline.tasks" :key="'t' + x.index" class="ol-note">
          <input type="checkbox" class="mt-0.5 shrink-0" :checked="x.done" :disabled="!canTick" :aria-label="x.text" data-testid="outline-task" @change="emit('toggle', x.index)" />
          <button type="button" class="line-clamp-2 text-left" :class="{ 'text-text-3 line-through': x.done }" @click="go(TASKS, x.index)">{{ x.text }}</button>
        </div>
      </template>
    </div>
  </component>
</template>

<style scoped>
.outline-aside {
  position: fixed;
  top: 64px;
  right: 16px;
  z-index: 5;
  width: 250px;
  max-height: calc(100dvh - 88px);
  overflow-y: auto;
  padding: 12px;
  border-radius: 14px;
  background: color-mix(in srgb, var(--surface) 94%, transparent);
  box-shadow: 0 0 0 0.5px var(--border), 0 6px 24px rgba(0, 0, 0, 0.06);
  backdrop-filter: blur(10px);
}
.ol-h {
  display: block;
  width: 100%;
  padding: 3px 8px;
  border-radius: 7px;
  text-align: left;
  font-size: 13px;
  color: var(--text-2);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ol-h.l1 {
  font-weight: 600;
  color: var(--text);
}
.ol-h:hover,
.ol-note:hover {
  background: var(--fill-2);
}
.ol-note {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  width: 100%;
  padding: 4px 8px;
  border-radius: 7px;
  text-align: left;
  font-size: 12.5px;
}
</style>

<style>
.outline-flash {
  border-radius: 6px;
  animation: outline-flash 1.2s ease;
}
@keyframes outline-flash {
  0%,
  40% {
    background: color-mix(in srgb, var(--accent) 18%, transparent);
  }
  100% {
    background: transparent;
  }
}
</style>

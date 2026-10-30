<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ChevronDown, ListTree, X } from 'lucide-vue-next'
import LocalGraph from '@/components/graph/LocalGraph.vue'
import { outlineOf } from '@/lib/outline'
import { ALERTS } from '@/components/editor/alerts'
import { rail, railFocus, setRailOpen } from '@/lib/rail'

// The side panel (as Folio's), where something is open — a page, a sheet,
// a diagram, a whiteboard, a task list, a folder: its graph, 1–3 links
// deep; on a page also its outline (headings to jump to) and its notes (its
// callouts and to-dos; a box ticks from here). Each part folds (remembered).
const route = useRoute()
const { t } = useI18n()

const ws = computed(() => (typeof route.params.ws === 'string' ? route.params.ws : ''))
// What's open: its id, for its own graph.
const focus = computed(() => railFocus(route))

// ---- The page's outline and notes ----
const outline = computed(() => (rail.page ? outlineOf(rail.page.content) : null))
function go(selector: string, n: number) {
  const els = [...(rail.page?.root?.querySelectorAll<HTMLElement>(selector) ?? [])].filter((el) => !el.closest('.page-embed'))
  const el = els[n]
  if (!el) return
  el.scrollIntoView({ behavior: 'smooth', block: 'center' })
  el.classList.add('outline-flash')
  setTimeout(() => el.classList.remove('outline-flash'), 1200)
}
const HEADINGS = '.md-view h1, .md-view h2, .md-view h3, .md-view h4, .md-view h5, .md-view h6, .ProseMirror h1, .ProseMirror h2, .ProseMirror h3'
const CALLOUTS = '.md-alert, .ProseMirror [data-type="callout"]'
const TASKS = '.md-view li:has(> input[data-task]), .md-view li:has(> p > input[data-task]), .ProseMirror li[data-checked]'
const emoji = (kind: string) => ALERTS[kind as keyof typeof ALERTS] ?? '💡'

// ---- Folding ----
type Part = 'graph' | 'outline' | 'notes'
const folded = ref<Set<Part>>(
  new Set(
    (() => {
      try {
        return JSON.parse(localStorage.getItem('orbit_rail_folded') ?? '[]') as Part[]
      } catch {
        return []
      }
    })(),
  ),
)
function fold(p: Part) {
  const s = new Set(folded.value)
  if (s.has(p)) s.delete(p)
  else s.add(p)
  folded.value = s
  try {
    localStorage.setItem('orbit_rail_folded', JSON.stringify([...s]))
  } catch {
    /* private mode */
  }
}
</script>

<template>
  <div class="rail-body" data-testid="outline">
    <div class="mb-1 flex items-center">
      <span class="flex-1 text-[12px] font-semibold text-text-2">{{ t('outline.title') }}</span>
      <button type="button" class="icon-btn h-7 w-7" :aria-label="t('common.close')" data-testid="rail-close" @click="setRailOpen(false)"><X class="h-4 w-4" /></button>
    </div>

    <button type="button" class="ol-part" :aria-expanded="!folded.has('graph')" @click="fold('graph')"><ChevronDown class="chev" :class="{ shut: folded.has('graph') }" />{{ t('graph.local') }}</button>
    <LocalGraph v-if="!folded.has('graph') && ws && focus" :key="focus" rail :ws="ws" :id="focus" />

    <template v-if="outline">
      <button type="button" class="ol-part" :aria-expanded="!folded.has('outline')" @click="fold('outline')"><ChevronDown class="chev" :class="{ shut: folded.has('outline') }" /><ListTree class="h-3.5 w-3.5" />{{ t('outline.headings') }}</button>
      <template v-if="!folded.has('outline')">
        <p v-if="!outline.headings.length" class="px-2 text-[12px] text-text-3">{{ t('outline.noHeadings') }}</p>
        <nav v-else :aria-label="t('outline.headings')">
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
      </template>

      <button type="button" class="ol-part" :aria-expanded="!folded.has('notes')" @click="fold('notes')"><ChevronDown class="chev" :class="{ shut: folded.has('notes') }" />{{ t('outline.notes') }}</button>
      <template v-if="!folded.has('notes')">
        <p v-if="!outline.callouts.length && !outline.tasks.length" class="px-2 text-[12px] text-text-3">{{ t('outline.noNotes') }}</p>
        <button v-for="c in outline.callouts" :key="'c' + c.index" type="button" class="ol-note" data-testid="outline-callout" @click="go(CALLOUTS, c.index)">
          <span class="w-4 shrink-0 text-center">{{ emoji(c.kind) }}</span><span class="line-clamp-2">{{ c.text || t(`editor.alert.${c.kind}`) }}</span>
        </button>
        <div v-for="x in outline.tasks" :key="'t' + x.index" class="ol-note">
          <input type="checkbox" class="mt-0.5 shrink-0" :checked="x.done" :disabled="!rail.page?.canTick" :aria-label="x.text" data-testid="outline-task" @change="rail.page?.toggle(x.index)" />
          <button type="button" class="line-clamp-2 text-left" :class="{ 'text-text-3 line-through': x.done }" @click="go(TASKS, x.index)">{{ x.text }}</button>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.rail-body {
  padding: 12px;
}
.ol-part {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  margin: 12px 0 4px;
  padding: 2px 4px;
  font-size: 11.5px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-3);
}
.ol-part .chev {
  width: 13px;
  height: 13px;
  transition: transform var(--speed);
}
.ol-part .chev.shut {
  transform: rotate(-90deg);
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

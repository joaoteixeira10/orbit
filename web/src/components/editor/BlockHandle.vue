<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Editor } from '@tiptap/core'
import { NodeSelection, TextSelection } from '@tiptap/pm/state'
import type { Node as PMNode } from '@tiptap/pm/model'
import {
  ArrowDown, ArrowUp, Copy, GripVertical, Heading1, Heading2, Heading3, List, ListOrdered, ListTodo, MessageSquareQuote, Plus, Quote, SquareCode, Trash2, Type,
} from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { ALERTS, alertOf, type AlertType } from './alerts'

// The handle beside the block under the pointer, as in Notion: + adds a
// block after it (with the "/" menu open); ⋮⋮ drags it somewhere else in
// the page, or, clicked, opens its menu — turn it into another kind,
// colour it (a coloured callout: GitHub's alerts, so the Markdown stays
// plain), duplicate, move up or down, delete. Top-level blocks only.
const props = defineProps<{ editor: Editor; root: HTMLElement | null }>()
const { t } = useI18n()

const at = ref<{ pos: number; top: number; left: number } | null>(null)
const menuOpen = ref(false)
const grip = ref<HTMLElement | null>(null)
let hideTimer: ReturnType<typeof setTimeout> | undefined

// blockAt finds the top-level block at a height on the page.
function blockAt(y: number): { pos: number; node: PMNode; dom: HTMLElement } | null {
  const view = props.editor.view
  const box = view.dom.getBoundingClientRect()
  const hit = view.posAtCoords({ left: box.left + Math.min(60, box.width / 2), top: y })
  if (!hit) return null
  const { doc } = view.state
  const $pos = doc.resolve(hit.pos)
  // Inside a block: its top-level ancestor; between blocks: the one there.
  let pos = -1
  if ($pos.depth >= 1) pos = $pos.before(1)
  else if (hit.inside >= 0) {
    const $in = doc.resolve(hit.inside)
    pos = $in.depth >= 1 ? $in.before(1) : hit.inside
  } else if ($pos.nodeAfter) pos = hit.pos
  if (pos < 0) return null
  const node = view.state.doc.nodeAt(pos)
  const dom = view.nodeDOM(pos) as HTMLElement | null
  if (!node || !dom || !(dom instanceof HTMLElement)) return null
  return { pos, node, dom }
}

// From a press on the grip until the drag ends, the handle stays where it
// is (it is the drag's source; moving it would keep the drag from starting).
let dragging = false
function press() {
  dragging = true
  window.addEventListener('mouseup', release, { once: true })
}
function release() {
  dragging = false
}
function onMove(e: MouseEvent) {
  if (dragging || menuOpen.value || !props.editor.isEditable || !props.root) return
  const b = blockAt(e.clientY)
  if (!b) return
  clearTimeout(hideTimer)
  const r = props.root.getBoundingClientRect()
  const d = b.dom.getBoundingClientRect()
  // Level with the block's first line.
  const line = parseFloat(getComputedStyle(b.dom).lineHeight) || 26
  const top = d.top - r.top + Math.max(0, Math.min(line, d.height) / 2 - 13) + (parseFloat(getComputedStyle(b.dom).paddingTop) || 0)
  at.value = { pos: b.pos, top, left: d.left - r.left - 52 }
}
function stay() {
  clearTimeout(hideTimer)
}
function onLeave() {
  clearTimeout(hideTimer)
  hideTimer = setTimeout(() => {
    if (!menuOpen.value && !dragging) at.value = null
  }, 250)
}
onMounted(() => {
  props.root?.addEventListener('mousemove', onMove)
  props.root?.addEventListener('mouseleave', onLeave)
})
onUnmounted(() => {
  props.root?.removeEventListener('mousemove', onMove)
  props.root?.removeEventListener('mouseleave', onLeave)
  clearTimeout(hideTimer)
})

const node = computed(() => (at.value ? props.editor.state.doc.nodeAt(at.value.pos) : null))

// ---- Dragging: the editor moves it (its own drop, its own undo) ----
function dragStart(e: DragEvent) {
  if (!at.value) return
  dragging = true
  const view = props.editor.view
  const sel = NodeSelection.create(view.state.doc, at.value.pos)
  view.dispatch(view.state.tr.setSelection(sel))
  const dom = view.nodeDOM(at.value.pos) as HTMLElement | null
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', sel.node.textContent)
    if (dom) e.dataTransfer.setDragImage(dom, 0, 0)
  }
  ;(view as unknown as { dragging: unknown }).dragging = { slice: sel.content(), move: true, node: sel }
}
function dragEnd() {
  dragging = false
  ;(props.editor.view as unknown as { dragging: unknown }).dragging = null
  at.value = null
}

// ---- +: a block after this one, the "/" menu open ----
function addBelow() {
  const n = node.value
  if (!at.value || !n) return
  const end = at.value.pos + n.nodeSize
  const chain = props.editor.chain()
  if (n.type.name === 'paragraph' && !n.content.size) chain.focus(at.value.pos + 1)
  else chain.insertContentAt(end, { type: 'paragraph' }).focus(end + 1)
  chain.insertContent('/').run()
  at.value = null
}

// ---- The menu ----
function openMenu() {
  if (!at.value) return
  menuOpen.value = true
}
function close() {
  menuOpen.value = false
  at.value = null
}
// select puts the selection over the block's text, so commands act on it.
function select(): boolean {
  const n = node.value
  if (!at.value || !n) return false
  const { state } = props.editor
  const from = at.value.pos + 1
  const to = at.value.pos + n.nodeSize - 1
  try {
    props.editor.view.dispatch(state.tr.setSelection(TextSelection.between(state.doc.resolve(from), state.doc.resolve(Math.max(from, to)))))
    return true
  } catch {
    return false
  }
}
type Turn = { key: string; icon: Component; run: () => void; on: (n: PMNode) => boolean }
const e = () => props.editor.chain().focus()
const TURNS: Turn[] = [
  { key: 'text', icon: Type, run: () => e().clearNodes().setParagraph().run(), on: (n) => n.type.name === 'paragraph' },
  { key: 'h1', icon: Heading1, run: () => e().clearNodes().setHeading({ level: 1 }).run(), on: (n) => n.type.name === 'heading' && n.attrs.level === 1 },
  { key: 'h2', icon: Heading2, run: () => e().clearNodes().setHeading({ level: 2 }).run(), on: (n) => n.type.name === 'heading' && n.attrs.level === 2 },
  { key: 'h3', icon: Heading3, run: () => e().clearNodes().setHeading({ level: 3 }).run(), on: (n) => n.type.name === 'heading' && n.attrs.level === 3 },
  { key: 'bullet', icon: List, run: () => e().clearNodes().toggleBulletList().run(), on: (n) => n.type.name === 'bulletList' },
  { key: 'numbered', icon: ListOrdered, run: () => e().clearNodes().toggleOrderedList().run(), on: (n) => n.type.name === 'orderedList' },
  { key: 'todo', icon: ListTodo, run: () => e().clearNodes().toggleTaskList().run(), on: (n) => n.type.name === 'taskList' },
  { key: 'quote', icon: Quote, run: () => e().clearNodes().toggleBlockquote().run(), on: (n) => n.type.name === 'blockquote' },
  { key: 'code', icon: SquareCode, run: () => e().clearNodes().toggleCodeBlock().run(), on: (n) => n.type.name === 'codeBlock' },
  { key: 'callout', icon: MessageSquareQuote, run: () => e().clearNodes().setCallout().run(), on: (n) => n.type.name === 'callout' },
]
// What a block can be turned into: text blocks, lists, quotes, code (not
// tables, pictures, lines).
const turnable = computed(() => !!node.value && ['paragraph', 'heading', 'bulletList', 'orderedList', 'taskList', 'blockquote', 'codeBlock', 'callout'].includes(node.value.type.name))
function turn(x: Turn) {
  if (select()) x.run()
  close()
}

// Colours: the callout kinds, in GitHub's colours.
const COLOURS: { kind: AlertType; color: string }[] = [
  { kind: 'NOTE', color: '#0969da' },
  { kind: 'TIP', color: '#1a7f37' },
  { kind: 'IMPORTANT', color: '#8250df' },
  { kind: 'WARNING', color: '#bf8700' },
  { kind: 'CAUTION', color: '#cf222e' },
]
const colourNow = computed<AlertType | ''>(() => (node.value?.type.name === 'callout' ? alertOf(node.value.attrs.emoji) : ''))
function colour(kind: AlertType | '') {
  const n = node.value
  const pos = at.value?.pos
  if (!n || pos == null) return
  const view = props.editor.view
  if (n.type.name === 'callout') {
    if (kind) view.dispatch(view.state.tr.setNodeMarkup(pos, undefined, { ...n.attrs, emoji: ALERTS[kind] }))
    else {
      // No colour: the blocks come out of the callout.
      const tr = view.state.tr.replaceWith(pos, pos + n.nodeSize, n.content)
      view.dispatch(tr)
    }
  } else if (kind) {
    const callout = view.state.schema.nodes.callout.create({ emoji: ALERTS[kind] }, n)
    view.dispatch(view.state.tr.replaceWith(pos, pos + n.nodeSize, callout))
  }
  props.editor.commands.focus()
  close()
}

function duplicate() {
  const n = node.value
  const pos = at.value?.pos
  if (!n || pos == null) return
  props.editor.view.dispatch(props.editor.state.tr.insert(pos + n.nodeSize, n.copy(n.content)))
  close()
}
function shift(by: -1 | 1) {
  const n = node.value
  const pos = at.value?.pos
  if (!n || pos == null) return
  const { state } = props.editor
  const $pos = state.doc.resolve(pos)
  const i = $pos.index(0)
  const other = state.doc.maybeChild(i + by)
  if (!other) return close()
  const tr = state.tr.delete(pos, pos + n.nodeSize)
  const to = by < 0 ? pos - other.nodeSize : pos + other.nodeSize
  tr.insert(to, n)
  props.editor.view.dispatch(tr)
  close()
}
function remove() {
  const n = node.value
  const pos = at.value?.pos
  if (!n || pos == null) return
  props.editor.view.dispatch(props.editor.state.tr.delete(pos, pos + n.nodeSize))
  close()
}
const canUp = computed(() => !!at.value && props.editor.state.doc.resolve(at.value.pos).index(0) > 0)
const canDown = computed(() => !!at.value && props.editor.state.doc.resolve(at.value.pos).index(0) < props.editor.state.doc.childCount - 1)
</script>

<template>
  <div v-if="at" class="bh" :style="{ top: `${at.top}px`, left: `${at.left}px` }" contenteditable="false" @mouseenter="stay" @mouseleave="onLeave">
    <button type="button" class="bh-btn" :title="t('editor.handle.add')" :aria-label="t('editor.handle.add')" data-testid="block-add" @mousedown.prevent @click="addBelow"><Plus class="h-4 w-4" /></button>
    <!-- A div, not a button: browsers don't all start a drag from a button. -->
    <div
      ref="grip"
      role="button"
      tabindex="0"
      class="bh-btn grip"
      draggable="true"
      :title="t('editor.handle.grip')"
      :aria-label="t('editor.handle.grip')"
      data-testid="block-grip"
      @click="openMenu"
      @keydown.enter.prevent="openMenu"
      @keydown.space.prevent="openMenu"
      @mousedown="press"
      @dragstart="dragStart"
      @dragend="dragEnd"
    >
      <GripVertical class="h-4 w-4" />
    </div>
  </div>

  <Popover v-if="menuOpen && grip" :anchor="grip" :width="250" placement="bottom-start" @close="close">
    <div class="p-1.5" data-testid="block-menu">
      <template v-if="turnable">
        <div class="cap px-2.5 pb-1 pt-1.5">{{ t('editor.handle.turnInto') }}</div>
        <div class="turns">
          <button
            v-for="x in TURNS"
            :key="x.key"
            type="button"
            class="turn"
            :class="{ on: node && x.on(node) }"
            :title="t(`editor.block.${x.key}.title`)"
            :aria-label="t(`editor.block.${x.key}.title`)"
            :data-testid="`turn-${x.key}`"
            @click="turn(x)"
          >
            <component :is="x.icon" class="h-4 w-4" />
          </button>
        </div>
        <div class="cap px-2.5 pb-1 pt-2">{{ t('editor.handle.colour') }}</div>
        <div class="flex items-center gap-1.5 px-2.5 pb-1.5">
          <button type="button" class="dot none" :class="{ on: !colourNow }" :title="t('editor.handle.noColour')" :aria-label="t('editor.handle.noColour')" data-testid="colour-none" @click="colour('')" />
          <button
            v-for="c in COLOURS"
            :key="c.kind"
            type="button"
            class="dot"
            :class="{ on: colourNow === c.kind }"
            :style="{ background: c.color }"
            :title="t(`editor.handle.colours.${c.kind}`)"
            :aria-label="t(`editor.handle.colours.${c.kind}`)"
            :data-testid="`colour-${c.kind}`"
            @click="colour(c.kind)"
          />
        </div>
        <div class="menu-sep" />
      </template>
      <button type="button" class="menu-item w-full" data-testid="block-duplicate" @click="duplicate"><Copy class="h-4 w-4 text-text-2" />{{ t('editor.handle.duplicate') }}</button>
      <button type="button" class="menu-item w-full" :disabled="!canUp" data-testid="block-up" @click="shift(-1)"><ArrowUp class="h-4 w-4 text-text-2" />{{ t('editor.handle.up') }}</button>
      <button type="button" class="menu-item w-full" :disabled="!canDown" data-testid="block-down" @click="shift(1)"><ArrowDown class="h-4 w-4 text-text-2" />{{ t('editor.handle.down') }}</button>
      <div class="menu-sep" />
      <button type="button" class="menu-item danger w-full" data-testid="block-delete" @click="remove"><Trash2 class="h-4 w-4" />{{ t('editor.handle.delete') }}</button>
    </div>
  </Popover>
</template>

<style scoped>
.bh {
  position: absolute;
  z-index: 4;
  display: flex;
  gap: 0;
  user-select: none;
}
.bh-btn {
  display: inline-flex;
  width: 24px;
  height: 26px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  color: var(--text-3);
  transition: background var(--speed), color var(--speed);
}
.bh-btn:hover {
  background: var(--fill-2);
  color: var(--text-2);
}
.grip {
  cursor: grab;
}
.grip:active {
  cursor: grabbing;
}
.turns {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 2px;
  padding: 0 6px 2px;
}
.turn {
  display: inline-flex;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  color: var(--text-2);
}
.turn:hover {
  background: var(--fill-2);
  color: var(--text);
}
.turn.on {
  background: var(--accent-bg, var(--fill));
  color: var(--accent-text, var(--accent));
}
.dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
}
.dot.none {
  border: 1.5px solid var(--border-strong, var(--text-3));
  background: transparent;
}
.dot.on {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--accent);
}
@media (pointer: coarse) {
  .bh {
    display: none;
  }
}
</style>

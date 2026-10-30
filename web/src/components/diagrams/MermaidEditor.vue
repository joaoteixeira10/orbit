<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Cable, LayoutTemplate, Plus, TriangleAlert } from 'lucide-vue-next'
import CodeArea from '@/components/editor/CodeArea.vue'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { MERMAID_TEMPLATES, addNode, connect, kindOf, renderMermaid, type MermaidKind } from '@/lib/mermaid'

// A Mermaid diagram, edited: the text and its drawing side by side (one
// above the other on a phone), redrawn a moment after each change. A new one
// starts from a template. On a flowchart the drawing edits the text too:
// "+ Node" adds a box (joined to the one picked, if any), "Connect" joins two
// boxes clicked one after the other. Readers see the drawing alone.
const props = defineProps<{ code: string; readonly?: boolean }>()
const emit = defineEmits<{ change: [code: string] }>()
const { t } = useI18n()

const text = ref(props.code)
watch(
  () => props.code,
  (c) => {
    if (c !== text.value) text.value = c
  },
)
function edited(v: string) {
  text.value = v
  emit('change', v)
}

// ---- Drawing ----
const svg = ref('')
const error = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
async function draw() {
  const mine = ++seq
  const r = await renderMermaid(text.value)
  if (mine !== seq) return
  error.value = r.error ?? ''
  if (r.svg) svg.value = r.svg
  else if (!text.value.trim()) svg.value = ''
  await nextTick()
  mark()
}
watch(
  text,
  () => {
    clearTimeout(timer)
    timer = setTimeout(() => void draw(), svg.value ? 350 : 0)
  },
  { immediate: true },
)
onUnmounted(() => clearTimeout(timer))

// picture is the drawing as it is now (for the diagram's picture): drawn
// afresh, so it matches the text even right after a change.
async function picture(): Promise<string | undefined> {
  const r = await renderMermaid(text.value)
  return r.svg
}
defineExpose({ picture })

// ---- Templates ----
const tplEl = ref<HTMLElement | null>(null)
const tplOpen = ref(false)
const pending = ref<string | null>(null)
function pickTemplate(key: MermaidKind) {
  tplOpen.value = false
  const code = MERMAID_TEMPLATES.find((x) => x.key === key)!.code
  if (text.value.trim()) pending.value = code
  else edited(code)
}

// ---- Editing on the drawing (flowcharts) ----
const flow = computed(() => kindOf(text.value) === 'flowchart')
const picked = ref('') // the box picked (joined from by "+ Node", the first end of "Connect")
const connecting = ref(false)
const naming = ref(false)
const label = ref('')
const labelEl = ref<HTMLInputElement | null>(null)
const pane = ref<HTMLElement | null>(null)

// A box's id in the text: Mermaid draws it as a .node whose id is
// "…flowchart-{id}-{n}".
function idOf(el: Element): string {
  return el.getAttribute('data-id') || (/flowchart-(.+)-\d+$/.exec(el.id)?.[1] ?? '')
}
function mark() {
  for (const el of pane.value?.querySelectorAll('.node.mmd-picked') ?? []) el.classList.remove('mmd-picked')
  if (!picked.value) return
  const el = [...(pane.value?.querySelectorAll('.node') ?? [])].find((n) => idOf(n) === picked.value)
  if (el) el.classList.add('mmd-picked')
  else picked.value = ''
}
watch(picked, mark)

function onPaneClick(e: MouseEvent) {
  if (props.readonly || !flow.value) return
  const node = (e.target as Element).closest('.node')
  const id = node ? idOf(node) : ''
  if (!id) {
    picked.value = ''
    return
  }
  if (connecting.value && picked.value && picked.value !== id) {
    edited(connect(text.value, picked.value, id))
    picked.value = id
    connecting.value = false
    return
  }
  picked.value = picked.value === id && !connecting.value ? '' : id
}
function startConnect() {
  connecting.value = !connecting.value
}
async function startNode() {
  naming.value = true
  label.value = ''
  await nextTick()
  labelEl.value?.focus()
}
function addOne() {
  const r = addNode(text.value, label.value.trim(), picked.value || undefined)
  edited(r.code)
  picked.value = r.id
  naming.value = false
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && (connecting.value || picked.value)) {
    e.preventDefault()
    connecting.value = false
    picked.value = ''
  }
}
const hint = computed(() => (connecting.value ? (picked.value ? t('mermaid.connectTo') : t('mermaid.connectFrom')) : picked.value ? t('mermaid.picked', { id: picked.value }) : ''))
</script>

<template>
  <div class="mermaid-editor" :class="{ 'is-readonly': readonly }" @keydown="onKey">
    <div v-if="!readonly" class="me-tools">
      <button ref="tplEl" type="button" class="btn btn-sm" data-testid="mermaid-templates" @click="tplOpen = !tplOpen"><LayoutTemplate class="h-3.5 w-3.5" />{{ t('mermaid.templates') }}</button>
      <template v-if="flow">
        <span class="me-sep" />
        <button type="button" class="btn btn-sm" data-testid="mermaid-add-node" @click="startNode"><Plus class="h-3.5 w-3.5" />{{ t('mermaid.addNode') }}</button>
        <button type="button" class="btn btn-sm" :class="{ on: connecting }" :aria-pressed="connecting" data-testid="mermaid-connect" @click="startConnect"><Cable class="h-3.5 w-3.5" />{{ t('mermaid.connect') }}</button>
        <form v-if="naming" class="flex items-center gap-1" @submit.prevent="addOne">
          <input ref="labelEl" v-model="label" class="field !h-7 w-44 !py-0 text-[13px]" :placeholder="t('mermaid.nodeLabel')" maxlength="120" data-testid="mermaid-node-label" @keydown.esc.stop.prevent="naming = false" />
          <button type="submit" class="btn btn-sm btn-primary">{{ t('mermaid.add') }}</button>
        </form>
        <span v-if="hint" class="text-[12px] text-text-3">{{ hint }}</span>
      </template>
    </div>
    <div class="me-split">
      <div v-if="!readonly" class="me-code">
        <CodeArea :model-value="text" :markdown="false" mono :label="t('mermaid.code')" :placeholder="t('mermaid.placeholder')" testid="mermaid-code" @update:model-value="edited" />
        <p v-if="error" class="me-error" data-testid="mermaid-error"><TriangleAlert class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span>{{ error }}</span></p>
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -- Mermaid's own SVG, drawn with securityLevel strict (sanitised) -->
      <div ref="pane" class="me-pane" :class="{ flow, connecting }" data-testid="mermaid-drawing" @click="onPaneClick" v-html="svg" />
    </div>
    <p v-if="!svg && !error && readonly" class="py-10 text-center text-[13px] text-text-3">{{ t('mermaid.empty') }}</p>

    <Popover v-if="tplOpen" :anchor="tplEl" :width="230" placement="bottom-start" @close="tplOpen = false">
      <div class="p-1.5">
        <button v-for="tpl in MERMAID_TEMPLATES" :key="tpl.key" type="button" class="menu-item w-full" :data-testid="`mermaid-tpl-${tpl.key}`" @click="pickTemplate(tpl.key)">{{ t(`mermaid.kind.${tpl.key}`) }}</button>
      </div>
    </Popover>
    <Confirm :open="pending !== null" :title="t('mermaid.replaceTitle')" :message="t('mermaid.replaceBody')" :confirm-label="t('mermaid.replace')" @confirm="edited(pending!), (pending = null)" @close="pending = null" />
  </div>
</template>

<style scoped>
.mermaid-editor {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  gap: 8px;
}
.me-tools {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.me-tools .btn.on {
  background: var(--accent);
  color: var(--accent-fg);
}
.me-sep {
  width: 0.5px;
  height: 18px;
  background: var(--border);
}
.me-split {
  display: grid;
  flex: 1;
  min-height: 0;
  grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
  gap: 10px;
}
.is-readonly .me-split {
  grid-template-columns: minmax(0, 1fr);
}
@media (max-width: 760px) {
  .me-split {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: minmax(200px, 1fr) minmax(260px, 1.4fr);
  }
}
.me-code {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 6px;
  overflow: auto;
  padding: 10px 12px;
  border: 0.5px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
}
.me-code :deep(.cm-editor) {
  min-height: 100%;
}
.me-error {
  display: flex;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--danger) 12%, transparent);
  color: var(--crit-text);
  font-size: 12px;
  white-space: pre-wrap;
}
.me-pane {
  display: flex;
  min-height: 0;
  align-items: center;
  justify-content: center;
  overflow: auto;
  padding: 16px;
  border: 0.5px solid var(--border);
  border-radius: 12px;
  background: #fff;
  color: #000;
}
.me-pane :deep(svg) {
  max-width: 100%;
  height: auto;
}
.me-pane.flow :deep(.node) {
  cursor: pointer;
}
.me-pane.connecting {
  cursor: crosshair;
}
.me-pane :deep(.node.mmd-picked) > :is(rect, polygon, circle, path, ellipse),
.me-pane :deep(.node.mmd-picked .label-container) {
  stroke: #007aff !important;
  stroke-width: 3px !important;
}
</style>

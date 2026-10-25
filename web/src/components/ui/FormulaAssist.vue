<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { at, helpFor, suggest, type FnHelp } from '@/lib/formulaHelp'
import { currentLocale } from '@/i18n'

// Help while writing a formula, as Excel gives it: the functions whose name
// starts with what's typed (↑ ↓ choose, Tab or Enter put it in, with its
// "("; Esc hides them), and inside a function's brackets its arguments, the
// one being written in bold. The field keeps the keyboard: it hands its
// keys to key() first.
const props = defineProps<{ el: HTMLInputElement | HTMLTextAreaElement | null; text: string; caret: number; on: boolean }>()
const emit = defineEmits<{ pick: [edit: { from: number; to: number; insert: string }] }>()
const { t } = useI18n()
const pt = computed(() => currentLocale() === 'pt')

const state = computed(() => (props.on ? at(props.text, props.caret) : { word: null, call: null }))
const dismissed = ref('')
const items = computed<FnHelp[]>(() => {
  const w = state.value.word
  if (!w || dismissed.value === `${w.from}:${w.text}`) return []
  const list = suggest(w.text, pt.value)
  // The name already typed out in full: nothing to suggest.
  return list.length === 1 && (pt.value ? list[0].pt : list[0].en).toLowerCase() === w.text.toLowerCase() ? [] : list
})
const index = ref(0)
watch(items, () => (index.value = 0))
const nameOf = (f: FnHelp) => (pt.value ? f.pt : f.en)
const sep = computed(() => (pt.value ? '; ' : ', '))
const argsOf = (f: FnHelp) => (pt.value ? f.args[1] : f.args[0]).split(', ')

// The call the caret is in, its arguments, the one being written.
const hint = computed(() => {
  const c = state.value.call
  const f = c ? helpFor(c.name) : undefined
  if (!f || items.value.length) return null
  const args = argsOf(f)
  // Past the last argument of a list "…": the last named one stays bold.
  const repeat = args[args.length - 1] === '…'
  const i = repeat && c!.arg >= args.length - 1 ? args.length - 2 : c!.arg
  return { f, args, i }
})

function pick(f: FnHelp) {
  const w = state.value.word
  if (!w) return
  emit('pick', { from: w.from, to: props.caret, insert: `${nameOf(f)}(` })
}
// key handles a key of the field's (true: it was ours).
function key(e: KeyboardEvent): boolean {
  if (!items.value.length) return false
  const n = items.value.length
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    index.value = (index.value + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
  } else if (e.key === 'Tab' || (e.key === 'Enter' && !e.shiftKey && !e.altKey)) {
    pick(items.value[index.value])
  } else if (e.key === 'Escape') {
    const w = state.value.word!
    dismissed.value = `${w.from}:${w.text}`
  } else return false
  e.preventDefault()
  e.stopPropagation()
  return true
}
defineExpose({ key })

const pos = computed(() => {
  void props.text
  void props.caret
  const r = props.el?.getBoundingClientRect()
  if (!r) return null
  const below = r.bottom + 4 + 260 < window.innerHeight
  return below ? { left: `${Math.max(8, r.left)}px`, top: `${r.bottom + 4}px` } : { left: `${Math.max(8, r.left)}px`, bottom: `${window.innerHeight - r.top + 4}px` }
})
</script>

<template>
  <Teleport to="body">
    <div v-if="on && pos && (items.length || hint)" class="fx-assist popover" :style="pos" data-testid="formula-assist" @mousedown.prevent>
      <template v-if="items.length">
        <button
          v-for="(f, i) in items"
          :key="f.en"
          type="button"
          class="fx-item"
          :class="{ on: i === index }"
          data-testid="formula-suggestion"
          @mouseenter="index = i"
          @click="pick(f)"
        >
          <span class="fx-name">{{ nameOf(f) }}</span>
          <span class="fx-desc">{{ pt ? f.desc[1] : f.desc[0] }}</span>
        </button>
        <p class="fx-foot">{{ nameOf(items[index]) }}({{ argsOf(items[index]).join(sep) }}) · {{ t('formulaHelp.keys') }}</p>
      </template>
      <p v-else-if="hint" class="fx-hint" data-testid="formula-hint">
        <span class="fx-name">{{ nameOf(hint.f) }}</span>(<template v-for="(a, i) in hint.args" :key="i"><span :class="{ cur: i === hint.i }">{{ a }}</span>{{ i < hint.args.length - 1 ? sep : '' }}</template>)
        <span class="fx-desc block">{{ pt ? hint.f.desc[1] : hint.f.desc[0] }}</span>
      </p>
    </div>
  </Teleport>
</template>

<style scoped>
.fx-assist {
  position: fixed;
  z-index: 1000;
  width: min(420px, calc(100vw - 16px));
  max-height: 260px;
  overflow-y: auto;
  padding: 4px;
  font-size: 12.5px;
}
.fx-item {
  display: flex;
  width: 100%;
  align-items: baseline;
  gap: 10px;
  padding: 4px 8px;
  border-radius: 6px;
  text-align: left;
}
.fx-item.on {
  background: var(--accent-bg);
}
.fx-name {
  flex: none;
  font: 600 12.5px var(--font-mono);
  color: var(--text);
}
.fx-desc {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-3);
}
.fx-foot {
  margin-top: 2px;
  padding: 4px 8px 2px;
  border-top: 0.5px solid var(--border);
  font: 11.5px var(--font-mono);
  color: var(--text-2);
}
.fx-hint {
  padding: 4px 8px;
  font: 12px var(--font-mono);
  color: var(--text-2);
}
.fx-hint .cur {
  font-weight: 700;
  color: var(--accent-text);
}
.fx-hint .fx-desc {
  margin-top: 2px;
  font-family: inherit;
  white-space: normal;
}
</style>

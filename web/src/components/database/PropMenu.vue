<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { EyeOff, Plus, Trash2, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import FormulaAssist from '@/components/ui/FormulaAssist.vue'
import { ChevronRight } from 'lucide-vue-next'
import { OPTION_COLORS, PROP_TYPES, formulaProblem, newId, type DBProp, type PropType } from '@/lib/database'

// A property's menu (its column's header): its name and type, a select's
// options (renamed, recoloured, taken away), hide it here, delete it.
// others: the database's other properties (to put in a formula).
const props = defineProps<{ anchor: HTMLElement | null; prop: DBProp; isNew?: boolean; others?: DBProp[] }>()
const emit = defineEmits<{ close: []; save: [prop: DBProp]; hide: []; remove: [] }>()
const { t } = useI18n()

const draft = ref<DBProp>(JSON.parse(JSON.stringify(props.prop)))
const optionName = ref('')
const formulaEl = ref<HTMLTextAreaElement | null>(null)
// Function suggestions and argument hints (FormulaAssist).
const assist = ref<InstanceType<typeof FormulaAssist> | null>(null)
const caret = ref(0)
const trackCaret = () => (caret.value = formulaEl.value?.selectionStart ?? 0)
function formulaKey(e: KeyboardEvent) {
  if (assist.value?.key(e)) return
  if (e.key === 'Enter' && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
    e.preventDefault()
    done()
  }
}
function applyPick(edit: { from: number; to: number; insert: string }) {
  const f = draft.value.formula ?? ''
  draft.value.formula = f.slice(0, edit.from) + edit.insert + f.slice(edit.to)
  const at = edit.from + edit.insert.length
  caret.value = at
  void nextTick(() => {
    formulaEl.value?.focus()
    formulaEl.value?.setSelectionRange(at, at)
  })
}
// insert puts text where the caret is in the formula; back: how far from
// its end the caret goes (inside a function's brackets).
function insert(text: string, back = 0) {
  const el = formulaEl.value
  const f = draft.value.formula ?? ''
  const at = el ? el.selectionStart : f.length
  draft.value.formula = f.slice(0, at) + text + f.slice(el ? el.selectionEnd : f.length)
  const caretAt = at + text.length - back
  caret.value = caretAt
  void nextTick(() => {
    el?.focus()
    el?.setSelectionRange(caretAt, caretAt)
  })
}

// A property clicked: its value on this row, or a total of its column (the
// same sums as the totals row, as formula functions).
const pick = ref<{ anchor: HTMLElement; prop: DBProp } | null>(null)
function choicesFor(p: DBProp): { label: string; text: string }[] {
  const col = `[${p.name}]`
  const out = [{ label: t('db.thisRow'), text: `[@${p.name}]` }]
  if (p.type === 'number' || p.type === 'formula') {
    out.push(
      { label: t('db.colAgg', { agg: t('db.aggs.sum') }), text: `SUM(${col})` },
      { label: t('db.colAgg', { agg: t('db.aggs.avg') }), text: `AVERAGE(${col})` },
      { label: t('db.colAgg', { agg: t('db.aggs.min') }), text: `MIN(${col})` },
      { label: t('db.colAgg', { agg: t('db.aggs.max') }), text: `MAX(${col})` },
    )
  }
  if (p.type === 'checkbox') out.push({ label: t('db.colAgg', { agg: t('db.aggs.checked') }), text: `COUNTIF(${col}, TRUE)` })
  out.push({ label: t('db.colAgg', { agg: t('db.aggs.count') }), text: `COUNTA(${col})` })
  return out
}
function choose(text: string) {
  pick.value = null
  insert(text)
}
// The functions most formulas need, a click away (the caret lands inside).
const FUNCTIONS = ['IF()', 'ROUND(, 2)', 'SUM()', 'AVERAGE()', 'IFERROR()', 'CONCAT()', 'TODAY()', 'DAYS(, TODAY())']
function insertFn(f: string) {
  const open = f.indexOf('(') + 1
  insert(f, f.length - open)
}
const choices = () => draft.value.type === 'select' || draft.value.type === 'multi_select'
function setType(type: PropType) {
  draft.value.type = type
  if (choices()) draft.value.options ??= []
}
function addOption() {
  const name = optionName.value.trim()
  if (!name) return
  draft.value.options = [...(draft.value.options ?? []), { id: newId(), name, color: OPTION_COLORS[(draft.value.options?.length ?? 0) % OPTION_COLORS.length] }]
  optionName.value = ''
}
function cycle(i: number) {
  const o = draft.value.options![i]
  o.color = OPTION_COLORS[(OPTION_COLORS.indexOf(o.color ?? '') + 1) % OPTION_COLORS.length]
}
function done() {
  const p = draft.value
  p.name = p.name.trim() || t(`db.types.${p.type}`)
  if (!choices()) delete p.options
  else p.options = (p.options ?? []).filter((o) => o.name.trim())
  emit('save', p)
  emit('close')
}
</script>

<template>
  <Popover :anchor="anchor" :width="draft.type === 'formula' ? 320 : 264" @close="done">
    <div class="space-y-2 p-2.5" data-testid="prop-menu">
      <input v-model="draft.name" class="field !py-1.5 text-[13.5px]" :placeholder="t('db.propName')" :aria-label="t('db.propName')" maxlength="100" autofocus @keydown.enter.prevent="done" />
      <label class="flex items-center gap-2 text-[13px] text-text-2">
        {{ t('db.type') }}
        <select class="input ml-auto !w-auto !py-1 text-[13px]" :value="draft.type" data-testid="prop-type" @change="setType(($event.target as HTMLSelectElement).value as PropType)">
          <option v-for="k in PROP_TYPES" :key="k" :value="k">{{ t(`db.types.${k}`) }}</option>
        </select>
      </label>
      <template v-if="draft.type === 'formula'">
        <div class="cap px-0.5 pt-1">{{ t('db.formula') }}</div>
        <textarea
          ref="formulaEl"
          v-model="draft.formula"
          class="field min-h-[64px] font-mono !text-[12.5px]"
          rows="3"
          spellcheck="false"
          placeholder="=[@Price]*[@Qty]"
          :aria-label="t('db.formula')"
          data-testid="prop-formula"
          @keydown="formulaKey"
          @input="trackCaret"
          @keyup="trackCaret"
          @click="trackCaret"
        />
        <FormulaAssist ref="assist" :el="formulaEl" :text="draft.formula ?? ''" :caret="caret" :on="true" @pick="applyPick" />
        <p v-if="draft.formula && formulaProblem(draft.formula)" class="text-[12px] text-crit-text">{{ formulaProblem(draft.formula) }}</p>
        <p v-else class="text-[11.5px] leading-snug text-text-3">{{ t('db.formulaHint') }}</p>
        <div v-if="others?.length">
          <div class="cap px-0.5 pb-1">{{ t('db.properties') }}</div>
          <div class="flex flex-wrap gap-1">
            <button v-for="o in others" :key="o.id" type="button" class="chip !text-[11.5px]" :data-testid="`formula-prop-${o.name}`" @click="pick = { anchor: $event.currentTarget as HTMLElement, prop: o }">
              {{ o.name }}<ChevronRight class="h-3 w-3 opacity-50" />
            </button>
          </div>
        </div>
        <div>
          <div class="cap px-0.5 pb-1">{{ t('db.functions') }}</div>
          <div class="flex flex-wrap gap-1">
            <button v-for="f in FUNCTIONS" :key="f" type="button" class="chip font-mono !text-[11px]" @click="insertFn(f)">{{ f.slice(0, f.indexOf('(')) }}</button>
          </div>
        </div>
        <Popover v-if="pick" :anchor="pick.anchor" :width="230" placement="bottom-start" @close="pick = null">
          <div class="p-1.5" data-testid="formula-pick">
            <button v-for="c in choicesFor(pick.prop)" :key="c.text" type="button" class="menu-item w-full" @click="choose(c.text)">
              <span class="min-w-0 flex-1 truncate">{{ c.label }}</span>
              <code class="shrink-0 text-[11px] text-text-3">{{ c.text.length > 18 ? c.text.slice(0, 17) + '…' : c.text }}</code>
            </button>
          </div>
        </Popover>
      </template>
      <template v-if="choices()">
        <div class="cap px-0.5 pt-1">{{ t('db.options') }}</div>
        <div v-for="(o, i) in draft.options" :key="o.id" class="flex items-center gap-1.5">
          <button type="button" class="h-4 w-4 shrink-0 rounded-full" :style="{ background: o.color || '#8e8e93' }" :aria-label="t('db.colour')" @click="cycle(i)" />
          <input v-model="o.name" class="field !py-1 text-[13px]" maxlength="100" :aria-label="t('db.optionName')" />
          <button type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('db.removeOption', { name: o.name })" @click="draft.options!.splice(i, 1)"><X class="h-3.5 w-3.5" /></button>
        </div>
        <div class="flex items-center gap-1.5">
          <Plus class="h-4 w-4 shrink-0 text-text-3" />
          <input v-model="optionName" class="field !py-1 text-[13px]" :placeholder="t('db.addOption')" maxlength="100" data-testid="prop-option" @keydown.enter.prevent="addOption" />
        </div>
      </template>
      <div class="border-t border-line-soft pt-1.5">
        <button v-if="!isNew" type="button" class="menu-item w-full" @click="emit('hide'), emit('close')"><EyeOff class="h-4 w-4 text-text-2" />{{ t('db.hide') }}</button>
        <button v-if="!isNew" type="button" class="menu-item danger w-full" @click="emit('remove'), emit('close')"><Trash2 class="h-4 w-4" />{{ t('db.deleteProp') }}</button>
        <button type="button" class="btn-primary btn-sm mt-1 w-full" data-testid="prop-done" @click="done">{{ t('db.done') }}</button>
      </div>
    </div>
  </Popover>
</template>

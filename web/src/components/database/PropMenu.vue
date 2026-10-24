<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { EyeOff, Plus, Trash2, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
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
// insert puts [@Name] where the caret is in the formula.
function insert(name: string) {
  const el = formulaEl.value
  const f = draft.value.formula ?? ''
  const ref = `[@${name}]`
  const at = el ? el.selectionStart : f.length
  draft.value.formula = f.slice(0, at) + ref + f.slice(el ? el.selectionEnd : f.length)
  requestAnimationFrame(() => {
    el?.focus()
    el?.setSelectionRange(at + ref.length, at + ref.length)
  })
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
  <Popover :anchor="anchor" :width="264" @close="done">
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
          @keydown.enter.exact.prevent="done"
        />
        <p v-if="draft.formula && formulaProblem(draft.formula)" class="text-[12px] text-crit-text">{{ formulaProblem(draft.formula) }}</p>
        <p v-else class="text-[11.5px] leading-snug text-text-3">{{ t('db.formulaHint') }}</p>
        <div v-if="others?.length" class="flex flex-wrap gap-1">
          <button v-for="o in others" :key="o.id" type="button" class="chip !text-[11.5px]" @click="insert(o.name)">{{ o.name }}</button>
        </div>
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

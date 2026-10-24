<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { EyeOff, Plus, Trash2, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { OPTION_COLORS, PROP_TYPES, newId, type DBProp, type PropType } from '@/lib/database'

// A property's menu (its column's header): its name and type, a select's
// options (renamed, recoloured, taken away), hide it here, delete it.
const props = defineProps<{ anchor: HTMLElement | null; prop: DBProp; isNew?: boolean }>()
const emit = defineEmits<{ close: []; save: [prop: DBProp]; hide: []; remove: [] }>()
const { t } = useI18n()

const draft = ref<DBProp>(JSON.parse(JSON.stringify(props.prop)))
const optionName = ref('')
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

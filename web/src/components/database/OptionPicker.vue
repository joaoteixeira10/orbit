<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Plus } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { OPTION_COLORS, newId, type DBOption, type DBProp } from '@/lib/database'

// The choices of a select (one) or multi-select (several) property, with a
// box to find one or make a new one (it joins the property's options).
const props = defineProps<{ anchor: HTMLElement | null; prop: DBProp; value: string[]; multi: boolean; canAdd: boolean }>()
const emit = defineEmits<{ close: []; pick: [ids: string[]]; add: [option: DBOption] }>()
const { t } = useI18n()
const q = ref('')
const shown = computed(() => (props.prop.options ?? []).filter((o) => o.name.toLowerCase().includes(q.value.trim().toLowerCase())))
const exact = computed(() => (props.prop.options ?? []).some((o) => o.name.toLowerCase() === q.value.trim().toLowerCase()))

function toggle(id: string) {
  if (!props.multi) {
    emit('pick', props.value[0] === id ? [] : [id])
    emit('close')
    return
  }
  emit('pick', props.value.includes(id) ? props.value.filter((x) => x !== id) : [...props.value, id])
}
function create() {
  const name = q.value.trim()
  if (!name || exact.value || !props.canAdd) return
  const o: DBOption = { id: newId(), name: name.slice(0, 100), color: OPTION_COLORS[(props.prop.options?.length ?? 0) % OPTION_COLORS.length] }
  emit('add', o)
  q.value = ''
  toggle(o.id)
}
function onEnter() {
  if (shown.value.length === 1 && !props.canAdd) toggle(shown.value[0].id)
  else if (!exact.value) create()
  else {
    const o = shown.value.find((x) => x.name.toLowerCase() === q.value.trim().toLowerCase())
    if (o) toggle(o.id)
  }
}
</script>

<template>
  <Popover :anchor="anchor" :width="240" @close="emit('close')">
    <div class="p-1.5" data-testid="option-picker">
      <input v-model="q" class="field mb-1.5 !py-1.5 text-[13.5px]" :placeholder="canAdd ? t('db.findOrAdd') : t('db.find')" autofocus @keydown.enter.prevent="onEnter" />
      <div class="max-h-64 overflow-y-auto">
        <button v-for="o in shown" :key="o.id" type="button" class="menu-item w-full" @click="toggle(o.id)">
          <Check class="h-4 w-4" :class="value.includes(o.id) ? 'text-accent' : 'opacity-0'" />
          <span class="chip-opt" :style="{ '--c': o.color || '#8e8e93' }">{{ o.name }}</span>
        </button>
        <button v-if="canAdd && q.trim() && !exact" type="button" class="menu-item w-full" @click="create">
          <Plus class="h-4 w-4 text-text-2" />{{ t('db.newOption', { name: q.trim() }) }}
        </button>
        <p v-if="!shown.length && !(canAdd && q.trim())" class="px-2.5 py-2 text-[13px] text-text-3">{{ t('db.noOptions') }}</p>
      </div>
    </div>
  </Popover>
</template>

<style scoped>
.chip-opt {
  padding: 1px 7px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--c) 18%, transparent);
  color: var(--text);
  font-size: 13px;
}
</style>

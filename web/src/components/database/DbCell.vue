<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink } from 'lucide-vue-next'
import OptionPicker from './OptionPicker.vue'
import { shownValue, type DBOption, type DBProp, type PropValue } from '@/lib/database'
import type { Member } from '@/api/planner'

// One value of a row: shown as it reads, and edited in place — text, a
// number, a link, a date and a box right in the cell; choices from a list.
const props = defineProps<{ prop: DBProp; value: PropValue | undefined; editable: boolean; members: Member[] }>()
const emit = defineEmits<{ set: [value: PropValue | null]; addOption: [option: DBOption] }>()
const { t } = useI18n()

const names = computed(() => new Map(props.members.map((m) => [m.user_id, m.display_name || m.username])))
const draft = ref('')
watch(
  () => props.value,
  (v) => (draft.value = v === undefined || v === null ? '' : String(v)),
  { immediate: true },
)
function commitText() {
  const v = draft.value.trim()
  if (props.prop.type === 'number') {
    const n = v === '' ? null : Number(v.replace(',', '.'))
    if (n !== null && Number.isNaN(n)) return void (draft.value = props.value === undefined ? '' : String(props.value))
    if (n !== (props.value ?? null)) emit('set', n)
    return
  }
  if (v !== (props.value ?? '')) emit('set', v || null)
}

const ids = computed<string[]>(() => (Array.isArray(props.value) ? props.value : typeof props.value === 'string' && props.value ? [props.value] : []))
const chosen = computed(() => ids.value.map((id) => props.prop.options?.find((o) => o.id === id)).filter((o): o is DBOption => !!o))
const anchor = ref<HTMLElement | null>(null)
const picking = ref(false)
function pick(next: string[]) {
  if (props.prop.type === 'multi_select') emit('set', next.length ? next : null)
  else emit('set', next[0] ?? null)
}
const href = computed(() => (props.prop.type === 'url' && typeof props.value === 'string' && /^https?:\/\//i.test(props.value) ? props.value : ''))
</script>

<template>
  <!-- A formula: worked out, read only (an Excel error in red) -->
  <span v-if="prop.type === 'formula'" class="cell-line">
    <span class="cell-text" :class="{ num: typeof value === 'number', 'text-crit-text': typeof value === 'string' && value.startsWith('#') }" data-testid="db-formula">{{ shownValue(prop, value, names) }}</span>
  </span>

  <!-- Text, numbers, links: typed in place -->
  <span v-else-if="prop.type === 'text' || prop.type === 'number' || prop.type === 'url'" class="cell-line">
    <input
      v-if="editable"
      v-model="draft"
      class="cell-input"
      :class="{ num: prop.type === 'number' }"
      :inputmode="prop.type === 'number' ? 'decimal' : undefined"
      :aria-label="prop.name"
      maxlength="2000"
      @blur="commitText"
      @keydown.enter="($event.target as HTMLInputElement).blur()"
    />
    <span v-else class="cell-text" :class="{ num: prop.type === 'number' }">{{ shownValue(prop, value, names) }}</span>
    <a v-if="href" :href="href" target="_blank" rel="noopener noreferrer nofollow" class="cell-out" :title="href"><ExternalLink class="h-3.5 w-3.5" /></a>
  </span>

  <input v-else-if="prop.type === 'checkbox'" type="checkbox" class="cell-check" :checked="!!value" :disabled="!editable" :aria-label="prop.name" @change="emit('set', ($event.target as HTMLInputElement).checked || null)" />

  <input
    v-else-if="prop.type === 'date' && editable"
    type="date"
    class="cell-input"
    :value="typeof value === 'string' ? value : ''"
    :aria-label="prop.name"
    @change="emit('set', ($event.target as HTMLInputElement).value || null)"
  />
  <span v-else-if="prop.type === 'date'" class="cell-text">{{ shownValue(prop, value, names) }}</span>

  <!-- A person: one of the members -->
  <span v-else-if="prop.type === 'person'" class="cell-line relative">
    <span class="cell-text" :class="{ 'text-text-3': !value }">{{ shownValue(prop, value, names) || (editable ? '—' : '') }}</span>
    <select v-if="editable" class="cell-over" :value="typeof value === 'string' ? value : ''" :aria-label="prop.name" @change="emit('set', ($event.target as HTMLSelectElement).value || null)">
      <option value="">{{ t('db.nobody') }}</option>
      <option v-for="m in members" :key="m.user_id" :value="m.user_id">{{ m.display_name || m.username }}</option>
    </select>
  </span>

  <!-- Choices -->
  <span v-else ref="anchor" class="cell-line chips" :class="{ click: editable }" :role="editable ? 'button' : undefined" :tabindex="editable ? 0 : undefined" :aria-label="prop.name" @click="editable && (picking = true)" @keydown.enter="editable && (picking = true)">
    <span v-for="o in chosen" :key="o.id" class="opt" :style="{ '--c': o.color || '#8e8e93' }">{{ o.name }}</span>
    <span v-if="!chosen.length && editable" class="text-text-faint">—</span>
  </span>
  <OptionPicker
    v-if="picking"
    :anchor="anchor"
    :prop="prop"
    :value="ids"
    :multi="prop.type === 'multi_select'"
    :can-add="editable"
    @pick="pick"
    @add="emit('addOption', $event)"
    @close="picking = false"
  />
</template>

<style scoped>
.cell-line {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 4px;
  width: 100%;
  min-height: 28px;
}
.cell-input {
  width: 100%;
  min-width: 0;
  padding: 4px 6px;
  border-radius: 6px;
  background: transparent;
  font-size: 13.5px;
  color: var(--text);
  outline: none;
}
.cell-input:hover {
  background: var(--fill-2);
}
.cell-input:focus {
  background: var(--surface);
  box-shadow: 0 0 0 2px var(--accent);
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
/* A number shown (a formula's, or read only) sits at the right, as typed ones do. */
.cell-text.num {
  flex: 1;
}
.cell-text {
  min-width: 0;
  padding: 4px 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13.5px;
}
.cell-out {
  flex: none;
  color: var(--text-3);
}
.cell-check {
  width: 16px;
  height: 16px;
  margin: 6px;
  accent-color: var(--accent);
}
.cell-over {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.chips {
  flex-wrap: wrap;
  padding: 2px 4px;
  border-radius: 6px;
}
.chips.click {
  cursor: pointer;
}
.chips.click:hover {
  background: var(--fill-2);
}
.opt {
  padding: 1px 7px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--c) 18%, transparent);
  font-size: 12.5px;
  white-space: nowrap;
}
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUpDown, Eye, EyeOff, ListFilter, Plus, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { NO_VALUE, OPS, typeOf, type Database, type DBFilter, type DBView } from '@/lib/database'
import type { Member } from '@/api/planner'

// A view's filter, sort and properties: each a button with a popover. Every
// change is saved with the view (it's the database's, so everyone sees it).
const props = defineProps<{ db: Database; view: DBView; editable: boolean; members: Member[] }>()
const emit = defineEmits<{ update: [view: DBView] }>()
const { t } = useI18n()

const open = ref<'' | 'filter' | 'sort' | 'props'>('')
const anchors = { filter: ref<HTMLElement | null>(null), sort: ref<HTMLElement | null>(null), props: ref<HTMLElement | null>(null) }
const fields = computed(() => [{ id: 'title', name: t('db.name') }, ...props.db.props.map((p) => ({ id: p.id, name: p.name }))])
const save = (patch: Partial<DBView>) => emit('update', { ...props.view, ...patch })

// ---- Filters ----
const filters = computed(() => props.view.filters ?? [])
function addFilter() {
  const prop = props.db.props[0]?.id ?? 'title'
  save({ filters: [...filters.value, { prop, op: OPS[typeOf(props.db, prop)][0] }] })
}
function setFilter(i: number, patch: Partial<DBFilter>) {
  const next = filters.value.map((f, j) => (j === i ? { ...f, ...patch } : f))
  // A new property: its first comparison, no value yet.
  if (patch.prop) next[i] = { prop: patch.prop, op: OPS[typeOf(props.db, patch.prop)][0] }
  save({ filters: next })
}
const optionsOf = (prop: string) => props.db.props.find((p) => p.id === prop)?.options ?? []

// ---- Sorts ----
const sorts = computed(() => props.view.sorts ?? [])
function addSort() {
  const used = new Set(sorts.value.map((s) => s.prop))
  const prop = fields.value.find((f) => !used.has(f.id))?.id
  if (prop) save({ sorts: [...sorts.value, { prop }] })
}

// ---- Properties shown ----
const hidden = computed(() => new Set(props.view.hidden ?? []))
function toggleHidden(id: string) {
  const h = new Set(hidden.value)
  if (!h.delete(id)) h.add(id)
  save({ hidden: [...h] })
}
</script>

<template>
  <button :ref="(el) => (anchors.filter.value = el as HTMLElement)" type="button" class="btn btn-sm" :class="{ 'tool-on': filters.length }" data-testid="db-filter" @click="open = open === 'filter' ? '' : 'filter'">
    <ListFilter class="h-3.5 w-3.5" />{{ t('db.filter') }}<span v-if="filters.length" class="badge">{{ filters.length }}</span>
  </button>
  <button :ref="(el) => (anchors.sort.value = el as HTMLElement)" type="button" class="btn btn-sm" :class="{ 'tool-on': sorts.length }" @click="open = open === 'sort' ? '' : 'sort'">
    <ArrowUpDown class="h-3.5 w-3.5" />{{ t('db.sort') }}
  </button>
  <button :ref="(el) => (anchors.props.value = el as HTMLElement)" type="button" class="btn btn-sm" @click="open = open === 'props' ? '' : 'props'">
    <Eye class="h-3.5 w-3.5" />{{ t('db.properties') }}
  </button>

  <Popover v-if="open === 'filter'" :anchor="anchors.filter.value" :width="380" placement="bottom-end" @close="open = ''">
    <div class="space-y-1.5 p-2.5" data-testid="db-filters">
      <p v-if="!filters.length" class="px-0.5 text-[13px] text-text-3">{{ t('db.noFilters') }}</p>
      <div v-for="(f, i) in filters" :key="i" class="flex items-center gap-1.5">
        <select class="input !w-auto min-w-0 flex-1 !py-1 text-[13px]" :value="f.prop" :disabled="!editable" :aria-label="t('db.property')" @change="setFilter(i, { prop: ($event.target as HTMLSelectElement).value })">
          <option v-for="x in fields" :key="x.id" :value="x.id">{{ x.name }}</option>
        </select>
        <select class="input !w-auto !py-1 text-[13px]" :value="f.op" :disabled="!editable" :aria-label="t('db.comparison')" @change="setFilter(i, { op: ($event.target as HTMLSelectElement).value })">
          <option v-for="op in OPS[typeOf(db, f.prop)]" :key="op" :value="op">{{ t(`db.ops.${op}`) }}</option>
        </select>
        <template v-if="!NO_VALUE.has(f.op)">
          <select v-if="typeOf(db, f.prop) === 'select' || typeOf(db, f.prop) === 'multi_select'" class="input !w-auto min-w-0 flex-1 !py-1 text-[13px]" :value="f.value ?? ''" :disabled="!editable" :aria-label="t('db.value')" @change="setFilter(i, { value: ($event.target as HTMLSelectElement).value })">
            <option value="" disabled>—</option>
            <option v-for="o in optionsOf(f.prop)" :key="o.id" :value="o.id">{{ o.name }}</option>
          </select>
          <select v-else-if="typeOf(db, f.prop) === 'person'" class="input !w-auto min-w-0 flex-1 !py-1 text-[13px]" :value="f.value ?? ''" :disabled="!editable" :aria-label="t('db.value')" @change="setFilter(i, { value: ($event.target as HTMLSelectElement).value })">
            <option value="me">{{ t('filter.me') }}</option>
            <option v-for="m in members" :key="m.user_id" :value="m.user_id">{{ m.display_name || m.username }}</option>
          </select>
          <input
            v-else
            class="field min-w-0 flex-1 !py-1 text-[13px]"
            :type="typeOf(db, f.prop) === 'date' ? 'date' : typeOf(db, f.prop) === 'number' ? 'number' : 'text'"
            :value="f.value ?? ''"
            :disabled="!editable"
            :aria-label="t('db.value')"
            data-testid="db-filter-value"
            @change="setFilter(i, { value: typeOf(db, f.prop) === 'number' ? Number(($event.target as HTMLInputElement).value) : ($event.target as HTMLInputElement).value })"
          />
        </template>
        <button v-if="editable" type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('db.removeFilter')" @click="save({ filters: filters.filter((_, j) => j !== i) })"><X class="h-3.5 w-3.5" /></button>
      </div>
      <button v-if="editable" type="button" class="btn-plain !text-[13px]" data-testid="db-add-filter" @click="addFilter"><Plus class="h-4 w-4" />{{ t('db.addFilter') }}</button>
    </div>
  </Popover>

  <Popover v-if="open === 'sort'" :anchor="anchors.sort.value" :width="320" placement="bottom-end" @close="open = ''">
    <div class="space-y-1.5 p-2.5">
      <p v-if="!sorts.length" class="px-0.5 text-[13px] text-text-3">{{ t('db.noSorts') }}</p>
      <div v-for="(s, i) in sorts" :key="i" class="flex items-center gap-1.5">
        <select class="input min-w-0 flex-1 !py-1 text-[13px]" :value="s.prop" :disabled="!editable" @change="save({ sorts: sorts.map((x, j) => (j === i ? { ...x, prop: ($event.target as HTMLSelectElement).value } : x)) })">
          <option v-for="x in fields" :key="x.id" :value="x.id">{{ x.name }}</option>
        </select>
        <select class="input !w-auto !py-1 text-[13px]" :value="s.desc ? 'desc' : 'asc'" :disabled="!editable" @change="save({ sorts: sorts.map((x, j) => (j === i ? { ...x, desc: ($event.target as HTMLSelectElement).value === 'desc' } : x)) })">
          <option value="asc">{{ t('db.asc') }}</option>
          <option value="desc">{{ t('db.desc') }}</option>
        </select>
        <button v-if="editable" type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('db.removeSort')" @click="save({ sorts: sorts.filter((_, j) => j !== i) })"><X class="h-3.5 w-3.5" /></button>
      </div>
      <button v-if="editable && sorts.length < fields.length" type="button" class="btn-plain !text-[13px]" @click="addSort"><Plus class="h-4 w-4" />{{ t('db.addSort') }}</button>
    </div>
  </Popover>

  <Popover v-if="open === 'props'" :anchor="anchors.props.value" :width="240" placement="bottom-end" @close="open = ''">
    <div class="p-1.5">
      <button v-for="p in db.props" :key="p.id" type="button" class="menu-item w-full" :disabled="!editable" @click="toggleHidden(p.id)">
        <component :is="hidden.has(p.id) ? EyeOff : Eye" class="h-4 w-4" :class="hidden.has(p.id) ? 'text-text-3' : 'text-accent'" />
        <span class="truncate" :class="{ 'text-text-3': hidden.has(p.id) }">{{ p.name }}</span>
      </button>
      <p v-if="!db.props.length" class="px-2.5 py-2 text-[13px] text-text-3">{{ t('db.noProps') }}</p>
    </div>
  </Popover>
</template>

<style scoped>
.tool-on {
  background: var(--accent-bg);
  color: var(--accent-text);
}
.badge {
  display: inline-grid;
  place-items: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--accent);
  color: #fff;
  font-size: 10.5px;
  font-weight: 600;
}
</style>

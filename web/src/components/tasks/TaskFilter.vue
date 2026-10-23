<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ListFilter, Search, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { useTaskFilterStore, activeCount, DUE_BUCKETS, PROGRESS, type TaskFilter } from '@/lib/taskFilter'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'

// The Filter button and its popover: keyword, due, priority, progress, who
// it's for, tags and (where several lists show at once) lists. Every change
// applies at once; the workspace's filter is shared by the task views. fkey
// is the filter it edits (a smart list's own; the workspace's by default);
// canSave offers saving it as a smart list.
const props = withDefaults(defineProps<{ ws: string; fkey?: string; showLists?: boolean; canSave?: boolean }>(), { fkey: '', showLists: false, canSave: false })
const emit = defineEmits<{ save: [] }>()
const store = useTaskFilterStore()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const { t } = useI18n()

const anchor = ref<HTMLElement | null>(null)
const open = ref(false)
const key = computed(() => props.fkey || props.ws)
const f = computed(() => store.of(key.value))
const count = computed(() => activeCount(f.value))

const members = computed(() => wss.members[props.ws] ?? [])
const lists = computed(() => tasks.listsOf(props.ws))
const tags = computed(() => [...new Set(tasks.tasksOf(props.ws).flatMap((x) => x.tags))].sort((a, b) => a.localeCompare(b)))

type ArrayKey = Exclude<keyof TaskFilter, 'q'>
function toggle<K extends ArrayKey>(key: K, v: TaskFilter[K][number]) {
  const arr = f.value[key] as (typeof v)[]
  const i = arr.indexOf(v)
  if (i >= 0) arr.splice(i, 1)
  else arr.push(v)
  store.save()
}
const on = <K extends ArrayKey>(key: K, v: TaskFilter[K][number]) => (f.value[key] as (typeof v)[]).includes(v)
function setQ(v: string) {
  f.value.q = v
  store.save()
}
const prios = [3, 2, 1, 0]
</script>

<template>
  <button ref="anchor" class="btn btn-sm shrink-0" :class="{ 'filter-on': count }" :aria-expanded="open" @click="open = !open">
    <ListFilter class="h-3.5 w-3.5" />
    <span>{{ t('filter.filter') }}</span>
    <span v-if="count" class="badge">{{ count }}</span>
  </button>
  <button v-if="count" class="icon-btn h-7 w-7 shrink-0" :title="t('filter.clear')" :aria-label="t('filter.clear')" @click="store.clear(key)"><X class="h-3.5 w-3.5" /></button>

  <Popover v-if="open" :anchor="anchor" placement="bottom-end" :width="320" @close="open = false">
    <div class="max-h-[70vh] overflow-y-auto p-3">
      <label class="search mb-3 h-9 w-full">
        <Search class="h-4 w-4 shrink-0" />
        <input :value="f.q" :placeholder="t('filter.keyword')" @input="setQ(($event.target as HTMLInputElement).value)" />
      </label>

      <h4 class="cap mb-1.5 px-0.5">{{ t('filter.due') }}</h4>
      <div class="mb-3 flex flex-wrap gap-1.5">
        <button v-for="d in DUE_BUCKETS" :key="d" class="chip" :class="{ on: on('due', d) }" @click="toggle('due', d)">{{ t(`filter.dueB.${d}`) }}</button>
      </div>

      <h4 class="cap mb-1.5 px-0.5">{{ t('filter.progress') }}</h4>
      <div class="mb-3 flex flex-wrap gap-1.5">
        <button v-for="p in PROGRESS" :key="p" class="chip" :class="{ on: on('progress', p) }" @click="toggle('progress', p)">{{ t(`filter.prog.${p}`) }}</button>
      </div>

      <h4 class="cap mb-1.5 px-0.5">{{ t('filter.priority') }}</h4>
      <div class="mb-3 flex flex-wrap gap-1.5">
        <button v-for="p in prios" :key="p" class="chip" :class="{ on: on('priority', p) }" @click="toggle('priority', p)">{{ t(`filter.prio.${p}`) }}</button>
      </div>

      <template v-if="members.length > 1">
        <h4 class="cap mb-1.5 px-0.5">{{ t('filter.assignee') }}</h4>
        <div class="mb-3 flex flex-wrap gap-1.5">
          <button class="chip" :class="{ on: on('assignee', 'me') }" @click="toggle('assignee', 'me')">{{ t('filter.me') }}</button>
          <button class="chip" :class="{ on: on('assignee', 'none') }" @click="toggle('assignee', 'none')">{{ t('filter.unassigned') }}</button>
          <button v-for="m in members" :key="m.user_id" class="chip" :class="{ on: on('assignee', m.user_id) }" @click="toggle('assignee', m.user_id)">{{ m.display_name || m.username }}</button>
        </div>
      </template>

      <template v-if="showLists && lists.length > 1">
        <h4 class="cap mb-1.5 px-0.5">{{ t('filter.lists') }}</h4>
        <div class="mb-3 flex flex-wrap gap-1.5">
          <button v-for="l in lists" :key="l.id" class="chip" :class="{ on: on('lists', l.id) }" @click="toggle('lists', l.id)">
            <span class="h-2 w-2 rounded-full" :style="{ background: l.color }" />{{ l.name }}
          </button>
        </div>
      </template>

      <template v-if="tags.length">
        <h4 class="cap mb-1.5 px-0.5">{{ t('filter.tags') }}</h4>
        <div class="mb-3 flex flex-wrap gap-1.5">
          <button v-for="tag in tags" :key="tag" class="chip" :class="{ on: on('tags', tag) }" @click="toggle('tags', tag)">#{{ tag }}</button>
        </div>
      </template>

      <div class="flex justify-end gap-3 border-t border-line-soft pt-2.5">
        <button v-if="canSave" class="btn-plain mr-auto !text-[13px]" :disabled="!count" data-testid="filter-save" @click="(open = false), emit('save')">{{ t('smartList.save') }}</button>
        <button class="btn-plain !text-[13px]" :disabled="!count" @click="store.clear(key)">{{ t('filter.clear') }}</button>
      </div>
    </div>
  </Popover>
</template>

<style scoped>
.filter-on {
  background: var(--accent-bg);
  color: var(--accent-text);
}
.badge {
  display: inline-grid;
  place-items: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--accent);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
}
</style>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Trash2, Trophy, X, Plus } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { COLORS } from '@/lib/icons'
import { goalsApi, pct, type Goal, type GoalInput } from '@/lib/goals'
import { useTasksStore } from '@/stores/tasks'
import { useToastStore } from '@/stores/toast'

// A goal: its name, emoji, colour, date and why; its tasks (tick them, add
// one, take one away), or — with none — how far along it is by hand; mark
// it achieved.
const props = defineProps<{ open: boolean; ws: string; goal: Goal | null; editable: boolean }>()
const emit = defineEmits<{ close: []; saved: [g: Goal]; removed: [id: string] }>()
const { t } = useI18n()
const tasks = useTasksStore()
const toast = useToastStore()

const title = ref('')
const icon = ref('')
const color = ref(COLORS[0])
const due = ref('')
const notes = ref('')
const progress = ref(0)
const busy = ref(false)
const current = ref<Goal | null>(null)

watch(
  () => props.open,
  (o) => {
    if (!o) return
    const g = props.goal
    current.value = g
    title.value = g?.title ?? ''
    icon.value = g?.icon ?? ''
    color.value = g?.color || COLORS[0]
    due.value = g?.due ?? ''
    notes.value = g?.notes ?? ''
    progress.value = g?.progress ?? 0
    void tasks.load(props.ws).catch(() => {})
  },
)

const mine = computed(() => (current.value ? tasks.tasksOf(props.ws).filter((x) => x.goal_id === current.value!.id && !x.parent_id) : []))
const others = computed(() => tasks.tasksOf(props.ws).filter((x) => !x.goal_id && !x.parent_id && !x.done))

async function save() {
  if (!title.value.trim() || busy.value) return
  const body: GoalInput = { title: title.value.trim(), icon: icon.value.trim(), color: color.value, due: due.value, notes: notes.value }
  if (!mine.value.length) body.progress = progress.value
  busy.value = true
  try {
    const g = current.value ? await goalsApi.update(current.value.id, body) : await goalsApi.create(props.ws, body)
    emit('saved', g)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
async function setAchieved(on: boolean) {
  if (!current.value) return
  try {
    const g = await goalsApi.update(current.value.id, { achieved: on })
    emit('saved', g)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}
// Tasks: tick, link, unlink, add — the goal's counts follow.
async function refresh() {
  if (!current.value) return
  const all = await goalsApi.list(props.ws)
  const g = all.find((x) => x.id === current.value!.id)
  if (g) {
    current.value = g
    emit('saved', g)
  }
}
async function setTask(id: string, patch: { goal_id?: string; completed?: boolean }) {
  try {
    await tasks.update(props.ws, id, patch)
    await refresh()
  } catch (e: any) {
    toast.error(e.message)
  }
}
const newTask = ref('')
async function addTask() {
  const tt = newTask.value.trim()
  if (!tt || !current.value) return
  newTask.value = ''
  try {
    await tasks.create(props.ws, { title: tt, goal_id: current.value.id })
    await refresh()
  } catch (e: any) {
    toast.error(e.message)
  }
}

const confirmDelete = ref(false)
async function remove() {
  confirmDelete.value = false
  if (!current.value) return
  try {
    await goalsApi.remove(current.value.id)
    emit('removed', current.value.id)
    emit('close')
    void tasks.load(props.ws).catch(() => {})
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open" :title="goal ? t('goals.edit') : t('goals.new')" size="md" @close="emit('close')">
    <form class="space-y-4" data-testid="goal-sheet" @submit.prevent="save">
      <div class="group">
        <div class="group-row">
          <input v-model="icon" class="w-10 shrink-0 bg-transparent text-center text-[20px] outline-none" maxlength="8" placeholder="🎯" :disabled="!editable" aria-label="Emoji" />
          <input v-model="title" class="min-w-0 flex-1 bg-transparent text-[16px] font-semibold outline-none placeholder:text-text-faint" :placeholder="t('goals.namePh')" :aria-label="t('goals.name')" maxlength="200" :disabled="!editable" autofocus />
        </div>
        <label class="group-row">
          <span class="flex-1 text-[14px]">{{ t('goals.due') }}</span>
          <input v-model="due" type="date" class="input !w-auto !py-1 text-[13.5px]" :disabled="!editable" />
          <button v-if="due && editable" type="button" class="icon-btn" :aria-label="t('goals.noDue')" @click="due = ''"><X class="h-4 w-4" /></button>
        </label>
        <div class="group-row flex-wrap">
          <div class="flex flex-wrap items-center gap-1.5" role="radiogroup">
            <button v-for="c in COLORS" :key="c" type="button" class="swatch" role="radio" :aria-checked="color === c" :aria-label="c" :style="{ '--sw': c }" :disabled="!editable" @click="color = c">
              <Check v-if="color === c" class="h-3 w-3" :stroke-width="3.4" />
            </button>
          </div>
        </div>
      </div>
      <textarea v-model="notes" rows="2" class="w-full resize-y rounded-xl bg-fill-2 px-3 py-2.5 text-[14px] outline-none placeholder:text-text-faint" :placeholder="t('goals.notes')" :disabled="!editable" maxlength="20000" />

      <section v-if="current">
        <h3 class="cap mb-1.5 px-1">{{ t('goals.tasks') }} · {{ pct(current) }}%</h3>
        <div class="group">
          <div v-for="x in mine" :key="x.id" class="group-row">
            <button type="button" class="check" :class="{ on: x.done }" role="checkbox" :aria-checked="x.done" :aria-label="x.title" :disabled="!editable" @click="setTask(x.id, { completed: !x.done })">
              <Check class="h-2.5 w-2.5" :stroke-width="4" />
            </button>
            <span class="min-w-0 flex-1 truncate text-[14px]" :class="{ 'text-text-3 line-through': x.done }">{{ x.title }}</span>
            <button v-if="editable" type="button" class="icon-btn h-7 w-7" :title="t('goals.remove')" :aria-label="t('goals.remove')" @click="setTask(x.id, { goal_id: '' })"><X class="h-3.5 w-3.5" /></button>
          </div>
          <div v-if="editable" class="group-row">
            <Plus class="h-4 w-4 shrink-0 text-text-3" />
            <input v-model="newTask" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none placeholder:text-text-faint" :placeholder="t('goals.newTask')" data-testid="goal-new-task" @keydown.enter.prevent="addTask" />
          </div>
          <label v-if="editable && others.length" class="group-row">
            <select class="input min-w-0 flex-1 !py-1 text-[13.5px]" :value="''" @change="setTask(($event.target as HTMLSelectElement).value, { goal_id: current.id }); ($event.target as HTMLSelectElement).value = ''">
              <option value="" disabled>{{ t('goals.addTask') }}</option>
              <option v-for="x in others" :key="x.id" :value="x.id">{{ x.title }}</option>
            </select>
          </label>
        </div>
      </section>
      <div v-if="!mine.length" class="group">
        <label class="group-row flex-wrap">
          <span class="flex-1">
            <span class="block text-[14px]">{{ t('goals.byHand') }} · {{ progress }}%</span>
            <span class="block text-[12px] text-text-3">{{ t('goals.byHandHint') }}</span>
          </span>
          <input v-model.number="progress" type="range" min="0" max="100" step="5" class="w-40" :disabled="!editable" />
        </label>
      </div>
      <button type="submit" class="hidden" />
    </form>
    <template #footer>
      <template v-if="current && editable">
        <button class="btn-ghost !text-danger" :title="t('goals.delete')" :aria-label="t('goals.delete')" @click="confirmDelete = true"><Trash2 class="h-4 w-4" /></button>
        <button class="btn-ghost mr-auto" @click="setAchieved(!current.achieved_at)"><Trophy class="h-4 w-4" />{{ current.achieved_at ? t('goals.reopen') : t('goals.markAchieved') }}</button>
      </template>
      <button class="btn" @click="emit('close')">{{ editable ? t('common.cancel') : t('common.close') }}</button>
      <button v-if="editable" class="btn-primary" :disabled="!title.trim() || busy" @click="save">{{ goal ? t('common.save') : t('common.add') }}</button>
    </template>
  </Overlay>
  <Confirm :open="confirmDelete" :title="t('goals.deleteTitle', { name: current?.title ?? '' })" :message="t('goals.deleteBody')" :confirm-label="t('common.delete')" danger @confirm="remove" @close="confirmDelete = false" />
</template>

<style scoped>
.swatch {
  display: grid;
  place-items: center;
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: var(--sw);
  color: #fff;
}
.swatch[aria-checked='true'] {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--sw);
}
</style>

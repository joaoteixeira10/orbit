<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Minus, Plus, Archive, ArchiveRestore, Trash2 } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Toggle from '@/components/ui/Toggle.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { COLORS } from '@/lib/icons'
import { weekdayNames } from '@/lib/dates'
import { habitsApi, type Habit, type HabitInput } from '@/lib/habits'
import { useToastStore } from '@/stores/toast'

// A habit's details: its name and emoji, colour, how often (every day, some
// weekdays, or a number of times a week) and a reminder time.
const props = defineProps<{ open: boolean; ws: string; habit: Habit | null }>()
const emit = defineEmits<{ close: []; saved: [h: Habit]; removed: [id: string] }>()
const { t } = useI18n()
const toast = useToastStore()

const title = ref('')
const icon = ref('')
const color = ref(COLORS[0])
const mode = ref<'daily' | 'some' | 'weekly'>('daily')
const days = ref('1111111')
const target = ref(3)
const remindOn = ref(false)
const remindAt = ref('20:00')
const busy = ref(false)

watch(
  () => props.open,
  (o) => {
    if (!o) return
    const h = props.habit
    title.value = h?.title ?? ''
    icon.value = h?.icon ?? ''
    color.value = h?.color || COLORS[0]
    mode.value = h?.schedule === 'weekly' ? 'weekly' : h && h.days !== '1111111' ? 'some' : 'daily'
    days.value = h?.days ?? '1111111'
    target.value = h?.schedule === 'weekly' ? h.target : 3
    remindOn.value = !!h?.remind_at
    remindAt.value = h?.remind_at || '20:00'
  },
)

// Monday first, in the UI's language.
const dayNames = computed(() => weekdayNames('narrow', 1))
function toggleDay(i: number) {
  const a = days.value.split('')
  a[i] = a[i] === '1' ? '0' : '1'
  if (a.includes('1')) days.value = a.join('')
}

const canSave = computed(() => !!title.value.trim() && !busy.value)
async function save() {
  if (!canSave.value) return
  const body: HabitInput = {
    title: title.value.trim(),
    icon: icon.value.trim(),
    color: color.value,
    schedule: mode.value === 'weekly' ? 'weekly' : 'daily',
    days: mode.value === 'daily' ? '1111111' : days.value,
    target: mode.value === 'weekly' ? target.value : 1,
    remind_at: remindOn.value ? remindAt.value : '',
  }
  busy.value = true
  try {
    const h = props.habit ? await habitsApi.update(props.habit.id, body) : await habitsApi.create(props.ws, body)
    emit('saved', h)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
async function archive(on: boolean) {
  if (!props.habit) return
  try {
    emit('saved', await habitsApi.update(props.habit.id, { archived: on }))
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}
const confirmDelete = ref(false)
async function remove() {
  confirmDelete.value = false
  if (!props.habit) return
  try {
    await habitsApi.remove(props.habit.id)
    emit('removed', props.habit.id)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open" :title="habit ? t('habits.edit') : t('habits.new')" size="md" @close="emit('close')">
    <form class="space-y-4" data-testid="habit-sheet" @submit.prevent="save">
      <div class="group">
        <div class="group-row">
          <input v-model="icon" class="w-10 shrink-0 bg-transparent text-center text-[20px] outline-none" maxlength="8" :placeholder="'🙂'" :aria-label="t('habits.icon')" />
          <input v-model="title" class="min-w-0 flex-1 bg-transparent text-[16px] font-semibold outline-none placeholder:text-text-faint" :placeholder="t('habits.namePh')" :aria-label="t('habits.name')" maxlength="200" autofocus />
        </div>
        <div class="group-row flex-wrap">
          <div class="flex flex-wrap items-center gap-1.5" role="radiogroup">
            <button v-for="c in COLORS" :key="c" type="button" class="swatch" role="radio" :aria-checked="color === c" :aria-label="c" :style="{ '--sw': c }" @click="color = c">
              <Check v-if="color === c" class="h-3 w-3" :stroke-width="3.4" />
            </button>
          </div>
        </div>
      </div>

      <div class="group">
        <div class="group-row">
          <span class="flex-1 text-[14px]">{{ t('habits.schedule') }}</span>
          <div class="seg" role="radiogroup">
            <button type="button" :class="{ on: mode === 'daily' }" @click="mode = 'daily'">{{ t('habits.daily') }}</button>
            <button type="button" :class="{ on: mode === 'some' }" @click="mode = 'some'">{{ t('habits.someDays') }}</button>
            <button type="button" :class="{ on: mode === 'weekly' }" @click="mode = 'weekly'">{{ t('habits.weekly') }}</button>
          </div>
        </div>
        <div v-if="mode === 'some'" class="group-row justify-center gap-1.5">
          <button v-for="(n, i) in dayNames" :key="i" type="button" class="day" :class="{ on: days[i] === '1' }" :aria-pressed="days[i] === '1'" @click="toggleDay(i)">{{ n }}</button>
        </div>
        <div v-if="mode === 'weekly'" class="group-row">
          <span class="flex-1 text-[14px]">{{ t('habits.timesWeek', { n: target }, target) }}</span>
          <button type="button" class="icon-btn" :disabled="target <= 1" @click="target--"><Minus class="h-4 w-4" /></button>
          <button type="button" class="icon-btn" :disabled="target >= 7" @click="target++"><Plus class="h-4 w-4" /></button>
        </div>
        <div class="group-row">
          <span class="flex-1">
            <span class="block text-[14px]">{{ t('habits.remind') }}</span>
            <span class="block text-[12px] text-text-3">{{ t('habits.remindHint') }}</span>
          </span>
          <input v-if="remindOn" v-model="remindAt" type="time" class="input !w-auto !py-1 text-[13px]" />
          <Toggle v-model="remindOn" :aria-label="t('habits.remind')" />
        </div>
      </div>
      <button type="submit" class="hidden" />
    </form>
    <template #footer>
      <template v-if="habit">
        <button class="btn-ghost !text-danger" @click="confirmDelete = true"><Trash2 class="h-4 w-4" /></button>
        <button class="btn-ghost mr-auto" @click="archive(!habit.archived_at)">
          <component :is="habit.archived_at ? ArchiveRestore : Archive" class="h-4 w-4" />{{ habit.archived_at ? t('habits.unarchive') : t('habits.archive') }}
        </button>
      </template>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="btn-primary" :disabled="!canSave" @click="save">{{ habit ? t('common.save') : t('common.add') }}</button>
    </template>
  </Overlay>
  <Confirm
    :open="confirmDelete"
    :title="t('habits.deleteTitle', { name: habit?.title ?? '' })"
    :message="t('habits.deleteBody')"
    :confirm-label="t('common.delete')"
    danger
    @confirm="remove"
    @close="confirmDelete = false"
  />
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
.day {
  width: 34px;
  height: 34px;
  border-radius: 999px;
  background: var(--fill);
  font-size: 13px;
  font-weight: 600;
  color: var(--text-2);
}
.day.on {
  background: var(--accent);
  color: var(--accent-fg, #fff);
}
</style>

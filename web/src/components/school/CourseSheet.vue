<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2, Check, Plus, X } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { coursesApi, type Course, type SlotInput } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { COLORS } from '@/lib/icons'
import { deviceTZ, weekdayNames, weekStart } from '@/lib/dates'

// A course: its name and short name, teacher, room, colour, the tag of its
// tasks, its term, its weight in the overall average, and its weekly classes
// (each one a repeating event on the calendar). Read-only for viewers.
const props = defineProps<{ open: boolean; ws: string; course: Course | null; editable: boolean }>()
const emit = defineEmits<{ close: []; saved: [course: Course]; deleted: [] }>()
const { t } = useI18n()
const toast = useToastStore()

const name = ref('')
const code = ref('')
const teacher = ref('')
const room = ref('')
const color = ref(COLORS[0])
const tag = ref('')
const termStart = ref('')
const termEnd = ref('')
const weight = ref(1)
const notes = ref('')
const slots = ref<SlotInput[]>([])
const saving = ref(false)
const askDelete = ref(false)
const nameEl = ref<HTMLInputElement | null>(null)

// Weekdays in the person's week order: [1 Monday … 7 Sunday].
const days = computed(() => {
  const ws = weekStart()
  const names = weekdayNames('long', ws)
  return names.map((label, i) => ({ value: ((i + ws + 6) % 7) + 1, label }))
})

watch(
  () => props.open,
  async (o) => {
    if (!o) return
    const c = props.course
    name.value = c?.name ?? ''
    code.value = c?.code ?? ''
    teacher.value = c?.teacher ?? ''
    room.value = c?.room ?? ''
    color.value = c?.color || COLORS[Math.floor(Math.random() * COLORS.length)]
    tag.value = c?.tag ?? ''
    termStart.value = c?.term_start ?? ''
    termEnd.value = c?.term_end ?? ''
    weight.value = c?.weight ?? 1
    notes.value = c?.notes ?? ''
    slots.value = (c?.slots ?? []).map((s) => ({ id: s.id, weekday: s.weekday, start: s.start, end: s.end, room: s.room, kind: s.kind }))
    if (!c) {
      await nextTick()
      nameEl.value?.focus()
    }
  },
)

function addSlot() {
  const last = slots.value[slots.value.length - 1]
  slots.value.push(last ? { weekday: (last.weekday % 5) + 1, start: last.start, end: last.end, room: '', kind: '' } : { weekday: 1, start: '09:00', end: '10:00', room: '', kind: '' })
}
const badSlot = (s: SlotInput) => !s.start || !s.end || s.end <= s.start
const termBad = computed(() => !!termStart.value && !!termEnd.value && termEnd.value < termStart.value)
const canSave = computed(() => props.editable && !!name.value.trim() && !saving.value && !termBad.value && !slots.value.some(badSlot))

async function save() {
  if (!canSave.value) return
  saving.value = true
  const body = {
    name: name.value.trim(),
    code: code.value.trim(),
    teacher: teacher.value.trim(),
    room: room.value.trim(),
    color: color.value,
    tag: tag.value.replace(/^#/, '').trim(),
    term_start: termStart.value,
    term_end: termEnd.value,
    tz: props.course?.tz && props.course.tz !== 'UTC' ? props.course.tz : deviceTZ(),
    weight: Number(weight.value) || 0,
    notes: notes.value,
    slots: slots.value,
  }
  try {
    const c = props.course ? await coursesApi.update(props.course.id, body) : await coursesApi.create(props.ws, body)
    toast.success(t('school.saved'))
    emit('saved', c)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
async function remove() {
  if (!props.course) return
  try {
    await coursesApi.remove(props.course.id)
    askDelete.value = false
    emit('deleted')
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open" :title="course ? t('school.editCourse') : t('school.newCourse')" size="md" @close="emit('close')">
    <form class="space-y-4" @submit.prevent="save" @keydown.meta.enter="save" @keydown.ctrl.enter="save">
      <fieldset :disabled="!editable" class="space-y-4">
        <div class="group">
          <div class="group-row">
            <span class="h-3 w-3 shrink-0 rounded-full" :style="{ background: color }" />
            <input ref="nameEl" v-model="name" class="min-w-0 flex-1 bg-transparent text-[16px] font-semibold outline-none placeholder:text-text-faint focus-visible:!shadow-none" :placeholder="t('school.namePh')" maxlength="120" :aria-label="t('school.name')" />
            <input v-model="code" class="w-20 bg-transparent text-right text-[14px] uppercase outline-none placeholder:text-text-faint focus-visible:!shadow-none" :placeholder="t('school.codePh')" maxlength="16" :aria-label="t('school.code')" />
          </div>
          <label class="group-row">
            <span class="w-24 shrink-0 text-[14px]">{{ t('school.teacher') }}</span>
            <input v-model="teacher" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none focus-visible:!shadow-none" maxlength="120" />
          </label>
          <label class="group-row">
            <span class="w-24 shrink-0 text-[14px]">{{ t('school.room') }}</span>
            <input v-model="room" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none focus-visible:!shadow-none" maxlength="60" />
          </label>
          <div class="group-row flex-wrap">
            <span class="flex-1 text-[14px]">{{ t('school.color') }}</span>
            <div class="flex flex-wrap items-center gap-1.5" role="radiogroup" :aria-label="t('school.color')">
              <button
                v-for="c in COLORS"
                :key="c"
                type="button"
                class="swatch"
                role="radio"
                :aria-checked="color.toLowerCase() === c.toLowerCase()"
                :aria-label="c"
                :style="{ '--sw': c }"
                @click="color = c"
              >
                <Check v-if="color.toLowerCase() === c.toLowerCase()" class="h-3 w-3" :stroke-width="3.4" />
              </button>
            </div>
          </div>
        </div>

        <!-- Weekly classes -->
        <div>
          <h3 class="subhead mb-1.5 px-1">{{ t('school.classes') }}</h3>
          <div class="group">
            <div v-for="(s, i) in slots" :key="s.id ?? `new-${i}`" class="group-row flex-wrap !gap-y-1.5" data-testid="class-row">
              <select v-model.number="s.weekday" class="input !w-auto !py-1 text-[13.5px]" :aria-label="t('cal.starts')">
                <option v-for="d in days" :key="d.value" :value="d.value">{{ d.label }}</option>
              </select>
              <input v-model="s.start" type="time" class="input !w-auto !py-1 text-[13.5px]" required :aria-label="t('cal.starts')" />
              <span class="text-text-3">–</span>
              <input v-model="s.end" type="time" class="input !w-auto !py-1 text-[13.5px]" :class="{ '!text-crit-text line-through': badSlot(s) }" required :aria-label="t('cal.ends')" />
              <input v-model="s.room" class="input !w-24 !py-1 text-[13.5px]" :placeholder="room || t('school.room')" maxlength="60" :aria-label="t('school.room')" />
              <input v-model="s.kind" class="input min-w-0 flex-1 !py-1 text-[13.5px]" :placeholder="t('school.kindPh')" maxlength="40" :aria-label="t('school.kind')" />
              <button v-if="editable" type="button" class="icon-btn h-7 w-7" :title="t('school.removeClass')" :aria-label="t('school.removeClass')" @click="slots.splice(i, 1)"><X class="h-4 w-4" /></button>
            </div>
            <div v-if="!slots.length && !editable" class="group-row text-[13.5px] text-text-3">{{ t('school.noClasses') }}</div>
            <button v-if="editable" type="button" class="group-row w-full text-left text-[14px] text-accent-text" @click="addSlot"><Plus class="h-4 w-4" />{{ t('school.addClass') }}</button>
          </div>
        </div>

        <div class="group">
          <div class="group-row flex-wrap !gap-y-1.5">
            <span class="flex-1 text-[14px]">{{ t('school.term') }}</span>
            <label class="inline-flex items-center gap-1.5 text-[13px] text-text-2">{{ t('school.termStart') }} <input v-model="termStart" type="date" class="input !w-auto !py-1 text-[13.5px]" /></label>
            <label class="inline-flex items-center gap-1.5 text-[13px] text-text-2">{{ t('school.termEnd') }} <input v-model="termEnd" type="date" class="input !w-auto !py-1 text-[13.5px]" :class="{ '!text-crit-text line-through': termBad }" :min="termStart || undefined" /></label>
          </div>
          <label class="group-row">
            <span class="flex-1 text-[14px]">{{ t('school.tag') }}</span>
            <span class="text-text-3">#</span>
            <input v-model="tag" class="w-36 bg-transparent text-[14px] outline-none placeholder:text-text-faint focus-visible:!shadow-none" :placeholder="(code || name).toLowerCase().replace(/[^\p{L}\p{N}_-]+/gu, '-')" maxlength="40" />
          </label>
          <label class="group-row">
            <span class="flex-1 text-[14px]">{{ t('school.weight') }}</span>
            <input v-model.number="weight" type="number" min="0" max="100" step="0.5" class="input !w-20 !py-1 text-right text-[13.5px]" />
          </label>
        </div>
        <p class="footnote -mt-2 px-1">{{ t('school.termHint') }} {{ t('school.weightHint') }}</p>

        <textarea
          v-model="notes"
          rows="3"
          class="w-full resize-y rounded-xl bg-fill-2 px-3 py-2.5 text-[14px] outline-none placeholder:text-text-faint focus:shadow-[0_0_0_3px_var(--ring)]"
          :placeholder="t('school.notes')"
          maxlength="20000"
        />
      </fieldset>
      <button type="submit" class="hidden" />
    </form>

    <template #footer>
      <button v-if="course && editable" class="btn-ghost mr-auto !text-danger" @click="askDelete = true"><Trash2 class="h-4 w-4" />{{ t('school.delete') }}</button>
      <button class="btn" @click="emit('close')">{{ editable ? t('common.cancel') : t('common.done') }}</button>
      <button v-if="editable" class="btn-primary" :disabled="!canSave" data-testid="course-save" @click="save">{{ course ? t('common.save') : t('school.create') }}</button>
    </template>
  </Overlay>
  <Confirm
    :open="askDelete"
    :title="t('school.deleteTitle', { name: course?.name })"
    :message="t('school.deleteBody')"
    :confirm-label="t('common.delete')"
    danger
    @confirm="remove"
    @close="askDelete = false"
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
  box-shadow: inset 0 0 0 0.5px rgba(0, 0, 0, 0.12);
  transition: transform 0.15s var(--spring);
}
.swatch:hover {
  transform: scale(1.12);
}
.swatch[aria-checked='true'] {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--sw);
}
</style>

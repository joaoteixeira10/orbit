<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2, Check } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Toggle from '@/components/ui/Toggle.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { calendarApi, type CalEvent, type EventInput, type Occurrence, type Workspace } from '@/api/planner'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { COLORS } from '@/lib/icons'
import {
  REPEAT_RULES, repeatLabel, repeatPreset, type RepeatPreset, toLocalInput, utcMidnight, allDayDate, dateStr, parseDate,
  addDays, deviceTZ, toZoneInput, fromZoneInput, zoneName, allZones, sameZoneTime, fmtTime, fmtDate,
} from '@/lib/dates'

// Calendar's event sheet: title and location, all-day, when it starts and
// ends (typed in the event's time zone — the device's unless changed; the
// calendar shows it in the device's), repeat, alert, colour and notes. Editing a repeating event edits its
// series, at the occurrence opened; saving asks whether for only it, it and
// the ones after, or all of them — and deleting the same.
// choices (in the all-workspaces calendar) adds a workspace picker on create.
const props = defineProps<{
  open: boolean
  occurrence: Occurrence | null // edit this event (its series)
  draft: { start: Date; end: Date; allDay: boolean } | null // or create one
  ws: string // the workspace to create in (the picker's default)
  choices: Workspace[]
}>()
const emit = defineEmits<{ close: []; saved: [] }>()
const wss = useWorkspacesStore()
const toast = useToastStore()
const { t } = useI18n()

const editing = computed(() => !!props.occurrence)
const loading = ref(false)
const saving = ref(false)
const ev = ref<CalEvent | null>(null)

const title = ref('')
const location = ref('')
const notes = ref('')
const allDay = ref(false)
const tz = ref(deviceTZ()) // the zone the times are typed in
const startIn = ref('') // YYYY-MM-DD when all-day, else YYYY-MM-DDTHH:MM (in tz)
const endIn = ref('') // all-day: the last day (inclusive)
const repeat = ref<RepeatPreset>('')
const customRule = ref('')
const color = ref('')
const remind = ref('') // minutes before, "" = none
const wsSel = ref('')
let lastTimed: { start: string; end: string } | null = null
let origTiming = ''

const titleEl = ref<HTMLInputElement | null>(null)

function reset() {
  ev.value = null
  title.value = ''
  location.value = ''
  notes.value = ''
  repeat.value = ''
  customRule.value = ''
  color.value = ''
  remind.value = ''
  tz.value = deviceTZ()
  lastTimed = null
}

watch(
  () => props.open,
  async (o) => {
    if (!o) return
    reset()
    if (props.occurrence) {
      loading.value = true
      try {
        const e = await calendarApi.getEvent(props.occurrence.id)
        ev.value = e
        title.value = e.title
        location.value = e.location
        notes.value = e.notes
        allDay.value = e.all_day
        // A series shows the occurrence opened (its own day and time).
        const occ = props.occurrence.recurring ? { starts_at: props.occurrence.start, ends_at: props.occurrence.end } : e
        if (e.all_day) {
          startIn.value = allDayDate(occ.starts_at)
          const last = dateStr(addDays(parseDate(allDayDate(occ.ends_at)), -1))
          endIn.value = last < startIn.value ? startIn.value : last
        } else {
          tz.value = e.tz && e.tz !== 'Local' && e.tz !== 'UTC' ? e.tz : deviceTZ()
          if (!zones.value.includes(tz.value)) tz.value = deviceTZ()
          startIn.value = toZoneInput(new Date(occ.starts_at), tz.value)
          endIn.value = toZoneInput(new Date(occ.ends_at), tz.value)
        }
        repeat.value = repeatPreset(e.rrule)
        customRule.value = e.rrule
        color.value = e.color
        remind.value = e.remind_minutes == null ? '' : String(e.remind_minutes)
        origTiming = JSON.stringify(timing())
      } catch (err: any) {
        toast.error(err.message)
        emit('close')
      } finally {
        loading.value = false
      }
    } else if (props.draft) {
      const d = props.draft
      allDay.value = d.allDay
      startIn.value = toLocalInput(d.start, !d.allDay)
      endIn.value = toLocalInput(d.end, !d.allDay)
      wsSel.value = props.choices.some((w) => w.id === props.ws) ? props.ws : props.choices[0]?.id ?? props.ws
      await nextTick()
      titleEl.value?.focus()
    }
  },
)

const wsId = computed(() => props.occurrence?.workspace_id || wsSel.value || props.ws)
const wsColor = computed(() => wss.byId(wsId.value)?.color ?? 'var(--accent)')
const showPicker = computed(() => !editing.value && props.choices.length > 0)

// ---- All-day and times ----
function setAllDay(v: boolean) {
  if (v === allDay.value) return
  if (v) {
    lastTimed = { start: startIn.value, end: endIn.value }
    startIn.value = startIn.value.slice(0, 10)
    const e = endIn.value.slice(0, 10)
    endIn.value = e < startIn.value ? startIn.value : e
  } else if (lastTimed && lastTimed.start.slice(0, 10) === startIn.value && lastTimed.end.slice(0, 10) === endIn.value) {
    startIn.value = lastTimed.start
    endIn.value = lastTimed.end
  } else {
    startIn.value = `${startIn.value}T09:00`
    endIn.value = `${endIn.value}T10:00`
  }
  allDay.value = v
}
// Moving the start keeps the length, as in Calendar.
function setStart(v: string) {
  const prevS = startIn.value
  startIn.value = v
  if (!v || !prevS || !endIn.value) return
  if (allDay.value) {
    const days = Math.round((parseDate(endIn.value).getTime() - parseDate(prevS).getTime()) / 86400000)
    endIn.value = dateStr(addDays(parseDate(v), Math.max(0, days)))
  } else {
    const dur = Math.max(0, fromZoneInput(endIn.value, tz.value).getTime() - fromZoneInput(prevS, tz.value).getTime())
    endIn.value = toZoneInput(new Date(fromZoneInput(v, tz.value).getTime() + dur), tz.value)
  }
}
const endBeforeStart = computed(() => {
  if (!startIn.value || !endIn.value) return false
  return allDay.value ? endIn.value < startIn.value : endIn.value < startIn.value
})
function timing(): EventInput {
  if (allDay.value) {
    return { all_day: true, starts_at: utcMidnight(startIn.value), ends_at: utcMidnight(dateStr(addDays(parseDate(endIn.value), 1))) }
  }
  return { all_day: false, starts_at: fromZoneInput(startIn.value, tz.value).toISOString(), ends_at: fromZoneInput(endIn.value, tz.value).toISOString(), tz: tz.value }
}

// ---- Time zone ----
// Changing it keeps the typed times: "10:00, New York".
const zones = computed(() => {
  const all = allZones()
  const here = deviceTZ()
  return all.includes(here) ? all : [here, ...all]
})
// When the zone isn't the device's: the same times in the device's.
const localTimes = computed(() => {
  if (allDay.value || !startIn.value || !endIn.value || endBeforeStart.value) return ''
  const a = fromZoneInput(startIn.value, tz.value)
  if (sameZoneTime(tz.value, deviceTZ(), a)) return ''
  const b = fromZoneInput(endIn.value, tz.value)
  return `${fmtDate(a, { weekday: 'short', day: 'numeric', month: 'short' })}, ${fmtTime(a)} – ${fmtTime(b)}`
})

// ---- Repeat, alert ----
const rule = computed(() => (repeat.value === 'custom' ? customRule.value : repeat.value ? REPEAT_RULES[repeat.value] : ''))
const remindOptions = computed(() => {
  const opts = [
    { v: '', label: t('cal.remind.none') },
    { v: '0', label: t('cal.remind.atStart') },
    { v: '5', label: t('cal.remind.min', { n: 5 }) },
    { v: '10', label: t('cal.remind.min', { n: 10 }) },
    { v: '15', label: t('cal.remind.min', { n: 15 }) },
    { v: '30', label: t('cal.remind.min', { n: 30 }) },
    { v: '60', label: t('cal.remind.hour') },
    { v: '1440', label: t('cal.remind.day') },
  ]
  if (remind.value && !opts.some((o) => o.v === remind.value)) opts.push({ v: remind.value, label: t('cal.remind.min', { n: Number(remind.value) }) })
  return opts
})

const canSave = computed(() => !!title.value.trim() && !!startIn.value && !!endIn.value && !endBeforeStart.value && !loading.value && !saving.value)

async function save() {
  if (!canSave.value) return
  saving.value = true
  try {
    const tm = timing()
    if (editing.value && ev.value) {
      const e = ev.value
      const body: EventInput = { title: title.value.trim(), location: location.value.trim(), notes: notes.value }
      // Timing only when it changed: the server forgets the deleted
      // occurrences of a series whose timing changes.
      if (JSON.stringify(tm) !== origTiming) Object.assign(body, tm)
      if (rule.value !== e.rrule) body.rrule = rule.value
      if (color.value !== e.color) body.color = color.value
      const before = e.remind_minutes == null ? '' : String(e.remind_minutes)
      if (remind.value !== before) body.remind_minutes = remind.value === '' ? -1 : Number(remind.value)
      if (props.occurrence?.recurring) {
        // A series: which ones? (asked; saved from there)
        pending.value = body
        askScope.value = true
        return
      }
      await calendarApi.updateEvent(e.id, body)
    } else {
      const body: EventInput = { title: title.value.trim(), location: location.value.trim(), notes: notes.value, ...tm }
      if (rule.value) body.rrule = rule.value
      if (color.value) body.color = color.value
      if (remind.value !== '') body.remind_minutes = Number(remind.value)
      await calendarApi.createEvent(wsSel.value || props.ws, body)
      if (showPicker.value) {
        try {
          localStorage.setItem('orbit_cal_ws', wsSel.value)
        } catch {
          /* private mode */
        }
      }
    }
    emit('saved')
    emit('close')
  } catch (err: any) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- A series: only this one, this and the following, or all ----
const askScope = ref(false)
const pending = ref<EventInput | null>(null)
async function saveScope(scope: 'this' | 'following' | 'all') {
  const o = props.occurrence
  const e = ev.value
  const body = pending.value
  askScope.value = false
  if (!o || !e || !body) return
  saving.value = true
  try {
    if (scope === 'all') {
      // The occurrence's change, made to the series: moved by as much.
      if (body.starts_at && body.ends_at) {
        const shift = Date.parse(body.starts_at) - Date.parse(o.start)
        const length = Date.parse(body.ends_at) - Date.parse(body.starts_at)
        const start = Date.parse(e.starts_at) + shift
        body.starts_at = new Date(start).toISOString()
        body.ends_at = new Date(start + length).toISOString()
      }
      await calendarApi.updateEvent(e.id, body)
    } else {
      if (scope === 'this') delete body.rrule
      await calendarApi.updateEvent(e.id, body, { scope, occurrence: o.start })
    }
    emit('saved')
    emit('close')
  } catch (err: any) {
    toast.error(err.message)
  } finally {
    saving.value = false
    pending.value = null
  }
}
// The repeat changed: "only this one" doesn't apply.
const ruleChanged = computed(() => !!ev.value && rule.value !== ev.value.rrule)

// ---- Delete ----
const askDelete = ref(false)
async function remove(onlyThis: boolean, following = false) {
  const o = props.occurrence
  if (!o) return
  askDelete.value = false
  try {
    await calendarApi.removeEvent(o.id, onlyThis || following ? o.start : undefined, following)
    emit('saved')
    emit('close')
  } catch (err: any) {
    toast.error(err.message)
  }
}
</script>

<template>
  <Overlay :open="open" :title="editing ? t('cal.editEvent') : t('cal.newEvent')" size="md" @close="emit('close')">
    <form class="space-y-4" :class="{ 'pointer-events-none opacity-60': loading }" @submit.prevent="save" @keydown.meta.enter="save" @keydown.ctrl.enter="save">
      <div class="group">
        <div class="group-row">
          <input ref="titleEl" v-model="title" class="w-full bg-transparent text-[16px] font-semibold outline-none placeholder:text-text-faint focus-visible:!shadow-none" :placeholder="t('cal.titlePlaceholder')" maxlength="300" />
        </div>
        <div class="group-row">
          <input v-model="location" class="w-full bg-transparent text-[14px] outline-none placeholder:text-text-faint focus-visible:!shadow-none" :placeholder="t('cal.location')" maxlength="300" />
        </div>
      </div>

      <div class="group">
        <div class="group-row">
          <span class="flex-1 text-[14px]">{{ t('cal.allDay') }}</span>
          <Toggle :model-value="allDay" @update:model-value="setAllDay" />
        </div>
        <label class="group-row flex-wrap !gap-y-1">
          <span class="flex-1 text-[14px]">{{ t('cal.starts') }}</span>
          <input :type="allDay ? 'date' : 'datetime-local'" class="input !w-auto !py-1 text-[13.5px]" :value="startIn" required @change="setStart(($event.target as HTMLInputElement).value)" />
        </label>
        <label class="group-row flex-wrap !gap-y-1">
          <span class="flex-1 text-[14px]">{{ t('cal.ends') }}</span>
          <input
            v-model="endIn"
            :type="allDay ? 'date' : 'datetime-local'"
            class="input !w-auto !py-1 text-[13.5px]"
            :class="{ '!text-crit-text line-through': endBeforeStart }"
            :min="allDay ? startIn : undefined"
            required
          />
        </label>
        <label v-if="!allDay" class="group-row">
          <span class="flex-1 text-[14px]">{{ t('cal.timeZone') }}</span>
          <select v-model="tz" class="input !w-auto max-w-[60%] !py-1 text-[13.5px]" data-testid="event-tz">
            <option v-for="z in zones" :key="z" :value="z">{{ z === deviceTZ() ? t('cal.tzHere', { zone: zoneName(z) }) : z.replace(/_/g, ' ') }}</option>
          </select>
        </label>
        <label class="group-row">
          <span class="flex-1 text-[14px]">{{ t('cal.repeat') }}</span>
          <select v-model="repeat" class="input !w-auto !py-1 text-[13.5px]">
            <option value="">{{ t('repeat.never') }}</option>
            <option value="daily">{{ t('repeat.daily') }}</option>
            <option value="weekdays">{{ t('repeat.weekdays') }}</option>
            <option value="weekly">{{ t('repeat.weekly') }}</option>
            <option value="biweekly">{{ t('repeat.biweekly') }}</option>
            <option value="monthly">{{ t('repeat.monthly') }}</option>
            <option value="yearly">{{ t('repeat.yearly') }}</option>
            <option v-if="customRule && repeatPreset(customRule) === 'custom'" value="custom">{{ repeatLabel(customRule) }}</option>
          </select>
        </label>
      </div>
      <p v-if="endBeforeStart" class="-mt-2 px-1 text-[12.5px] text-crit-text">{{ t('cal.endBeforeStart') }}</p>
      <p v-else-if="localTimes" class="-mt-2 px-1 text-[12.5px] text-text-3" data-testid="local-times">{{ t('cal.yourTime', { when: localTimes }) }}</p>

      <div class="group">
        <label v-if="showPicker" class="group-row">
          <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: wsColor }" />
          <span class="flex-1 text-[14px]">{{ t('cal.workspace') }}</span>
          <select v-model="wsSel" class="input !w-auto max-w-[60%] !py-1 text-[13.5px]">
            <option v-for="w in choices" :key="w.id" :value="w.id">{{ w.name }}</option>
          </select>
        </label>
        <label class="group-row">
          <span class="flex-1 text-[14px]">{{ t('cal.alert') }}</span>
          <select v-model="remind" class="input !w-auto !py-1 text-[13.5px]">
            <option v-for="o in remindOptions" :key="o.v" :value="o.v">{{ o.label }}</option>
          </select>
        </label>
        <div class="group-row flex-wrap">
          <span class="flex-1 text-[14px]">{{ t('cal.color') }}</span>
          <div class="flex flex-wrap items-center gap-1.5" role="radiogroup" :aria-label="t('cal.color')">
            <button
              type="button"
              class="swatch auto"
              role="radio"
              :aria-checked="!color"
              :title="t('cal.colorAuto')"
              :style="{ '--sw': wsColor }"
              @click="color = ''"
            >
              <Check v-if="!color" class="h-3 w-3" :stroke-width="3.4" />
            </button>
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

      <textarea
        v-model="notes"
        rows="3"
        class="w-full resize-y rounded-xl bg-fill-2 px-3 py-2.5 text-[14px] outline-none placeholder:text-text-faint focus:shadow-[0_0_0_3px_var(--ring)]"
        :placeholder="t('cal.notes')"
        maxlength="20000"
      />

      <p v-if="editing && ev?.rrule" class="footnote px-1">{{ t('cal.seriesNote') }}</p>
      <button type="submit" class="hidden" />
    </form>

    <template #footer>
      <button v-if="editing" class="btn-ghost mr-auto !text-danger" :disabled="loading" @click="askDelete = true"><Trash2 class="h-4 w-4" />{{ t('common.delete') }}</button>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="btn-primary" :disabled="!canSave" @click="save">{{ editing ? t('common.save') : t('common.add') }}</button>
    </template>
  </Overlay>

  <!-- A single event: confirm. A series: this one or all of them. -->
  <Confirm
    :open="askDelete && !occurrence?.recurring"
    :title="t('cal.deleteTitle', { title: title || t('cal.untitled') })"
    :message="t('cal.deleteBody')"
    :confirm-label="t('common.delete')"
    danger
    @confirm="remove(false)"
    @close="askDelete = false"
  />
  <Overlay :open="askDelete && !!occurrence?.recurring" size="sm" @close="askDelete = false">
    <div class="pt-3 text-center">
      <h2 class="title-3">{{ t('cal.deleteRecurringTitle') }}</h2>
      <p class="mt-1.5 text-[13px] leading-relaxed text-text-2">{{ t('cal.deleteRecurringBody') }}</p>
    </div>
    <div class="mt-5 grid gap-2">
      <button class="btn-danger" data-testid="delete-this" @click="remove(true)">{{ t('cal.deleteThis') }}</button>
      <button class="btn !text-danger" data-testid="delete-following" @click="remove(false, true)">{{ t('cal.deleteFollowing') }}</button>
      <button class="btn !text-danger" data-testid="delete-all" @click="remove(false)">{{ t('cal.deleteAll') }}</button>
      <button class="btn" @click="askDelete = false">{{ t('common.cancel') }}</button>
    </div>
  </Overlay>
  <Overlay :open="askScope" size="sm" @close="(askScope = false), (pending = null)">
    <div class="pt-3 text-center">
      <h2 class="title-3">{{ t('cal.scopeTitle') }}</h2>
      <p class="mt-1.5 text-[13px] leading-relaxed text-text-2">{{ t('cal.scopeBody') }}</p>
    </div>
    <div class="mt-5 grid gap-2" data-testid="event-scope">
      <button class="btn-primary justify-center" :disabled="ruleChanged" :title="ruleChanged ? t('cal.scopeThisNoRepeat') : ''" data-testid="scope-this" @click="saveScope('this')">{{ t('cal.scopeThis') }}</button>
      <button class="btn justify-center" data-testid="scope-following" @click="saveScope('following')">{{ t('cal.scopeFollowing') }}</button>
      <button class="btn justify-center" data-testid="scope-all" @click="saveScope('all')">{{ t('cal.scopeAll') }}</button>
      <button class="btn justify-center" @click="(askScope = false), (pending = null)">{{ t('common.cancel') }}</button>
    </div>
  </Overlay>
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
/* "Workspace colour": the colour with a ring, as Calendar's default. */
.swatch.auto {
  background: conic-gradient(var(--sw) 0 50%, color-mix(in srgb, var(--sw) 55%, #fff) 50% 100%);
}
</style>

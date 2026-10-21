<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { MapPin, Repeat, CalendarDays, Lock, GraduationCap, CalendarX } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import { calendarApi } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { whenLines, eventColor, type Entry } from './model'

// A read-only look at an event: one from a subscribed calendar (changed
// where it comes from), one in a workspace the user can only view, or a
// course's class (changed in Courses; an editor can cancel this one).
const props = defineProps<{ open: boolean; entry: Entry | null; calendarName: string; canCancel?: boolean }>()
const emit = defineEmits<{ close: []; cancelled: [] }>()
const { t } = useI18n()
const router = useRouter()
const toast = useToastStore()

const when = computed(() => (props.entry ? whenLines(props.entry) : []))
const external = computed(() => props.entry?.occ.kind === 'external')
const course = computed(() => props.entry?.occ.course_id ?? '')

function openCourse() {
  if (!props.entry) return
  emit('close')
  router.push({ name: 'ws-courses', params: { ws: props.entry.occ.workspace_id }, query: { course: course.value } })
}
async function cancelClass() {
  if (!props.entry) return
  try {
    await calendarApi.removeEvent(props.entry.occ.id, props.entry.occ.start)
    toast.success(t('school.cancelled'))
    emit('cancelled')
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open && !!entry" size="sm" @close="emit('close')">
    <template v-if="entry">
      <div class="flex items-start gap-3 pt-2">
        <span class="mt-1 w-1 shrink-0 self-stretch rounded-full" :style="{ background: eventColor(entry) }" />
        <div class="min-w-0 flex-1">
          <h2 class="title-2 break-words">{{ entry.occ.title || t('cal.untitled') }}</h2>
          <p v-for="(l, i) in when" :key="i" class="mt-0.5 text-[14px]" :class="i ? 'text-text-2' : ''">{{ l }}</p>
          <p v-if="entry.occ.recurring" class="mt-0.5 flex items-center gap-1 text-[13px] text-text-2"><Repeat class="h-3.5 w-3.5" />{{ t('cal.repeats') }}</p>
        </div>
      </div>

      <div class="group mt-5">
        <div v-if="entry.occ.location" class="group-row">
          <MapPin class="h-4 w-4 shrink-0 text-text-3" />
          <span class="min-w-0 flex-1 break-words text-[14px]">{{ entry.occ.location }}</span>
        </div>
        <div class="group-row">
          <CalendarDays class="h-4 w-4 shrink-0 text-text-3" />
          <span class="flex-1 text-[14px]">{{ t('cal.calendar') }}</span>
          <span class="flex min-w-0 items-center gap-1.5 text-[14px] text-text-2">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full" :style="{ background: eventColor(entry) }" />
            <span class="truncate">{{ calendarName }}</span>
          </span>
        </div>
      </div>

      <p v-if="entry.occ.notes" class="mt-4 whitespace-pre-wrap break-words rounded-xl bg-fill-2 px-3 py-2.5 text-[14px]">{{ entry.occ.notes }}</p>

      <div v-if="course" class="mt-4 grid gap-2">
        <button class="btn justify-start" @click="openCourse"><GraduationCap class="h-4 w-4" />{{ t('school.openCourse') }}</button>
        <button v-if="canCancel" class="btn justify-start !text-danger" @click="cancelClass"><CalendarX class="h-4 w-4" />{{ t('school.cancelClass') }}</button>
      </div>

      <p class="footnote mt-4 flex items-start gap-1.5 px-1">
        <Lock class="mt-0.5 h-3 w-3 shrink-0" />{{ entry.occ.kind === 'holiday' ? t('cal.holidayNote') : course ? t('school.classHint') : external ? t('cal.externalReadOnly') : t('cal.viewOnlyEvent') }}
      </p>
    </template>
    <template #footer>
      <button class="btn-primary" @click="emit('close')">{{ t('common.done') }}</button>
    </template>
  </Overlay>
</template>

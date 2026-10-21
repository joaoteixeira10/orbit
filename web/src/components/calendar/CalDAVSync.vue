<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RefreshCw, AlertCircle, ArrowLeftRight, Check } from 'lucide-vue-next'
import Confirm from '@/components/ui/Confirm.vue'
import { calendarApi, type CalDAVLink, type CalDAVCalendar } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { locale } from '@/lib/dates'

// Two-way sync with a calendar on a CalDAV server: owners connect an account
// (server, username, app password), pick one of its calendars and sync;
// editors can sync now. Shown at the top of the calendars sheet.
const props = defineProps<{ ws: string; open: boolean; owner: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const toast = useToastStore()
const { t } = useI18n()

const link = ref<CalDAVLink | null>(null)
const loaded = ref(false)
const syncing = ref(false)

async function load() {
  try {
    link.value = (await calendarApi.caldav(props.ws)).link
  } catch {
    link.value = null
  } finally {
    loaded.value = true
  }
}

// ---- Setup ----
const setup = ref(false)
const server = ref('')
const username = ref('')
const password = ref('')
const calendars = ref<CalDAVCalendar[] | null>(null)
const chosen = ref('')
const finding = ref(false)
const linking = ref(false)
const error = ref('')
const presets = [
  { name: 'iCloud', url: 'https://caldav.icloud.com' },
  { name: 'Fastmail', url: 'https://caldav.fastmail.com' },
  { name: 'Nextcloud', url: 'https://' },
]
watch(
  () => [props.open, props.ws] as const,
  ([o]) => {
    if (!o) return
    loaded.value = false
    setup.value = false
    void load()
  },
  { immediate: true },
)

function start() {
  server.value = ''
  username.value = ''
  password.value = ''
  calendars.value = null
  chosen.value = ''
  error.value = ''
  setup.value = true
}
const canFind = computed(() => server.value.trim().length > 8 && !!username.value.trim() && !!password.value && !finding.value)
async function find() {
  if (!canFind.value) return
  finding.value = true
  error.value = ''
  try {
    calendars.value = await calendarApi.caldavCalendars(props.ws, { url: server.value.trim(), username: username.value.trim(), password: password.value })
    chosen.value = calendars.value[0]?.url ?? ''
  } catch (e: any) {
    error.value = e.message
  } finally {
    finding.value = false
  }
}
async function connect() {
  const cal = calendars.value?.find((c) => c.url === chosen.value)
  if (!cal || linking.value) return
  linking.value = true
  error.value = ''
  try {
    link.value = (
      await calendarApi.setCalDAV(props.ws, { url: server.value.trim(), username: username.value.trim(), password: password.value, calendar_url: cal.url, calendar_name: cal.name })
    ).link
    password.value = ''
    setup.value = false
    emit('changed')
  } catch (e: any) {
    error.value = e.message
  } finally {
    linking.value = false
  }
}

// ---- Linked ----
async function syncNow() {
  syncing.value = true
  try {
    link.value = (await calendarApi.syncCalDAV(props.ws)).link
    emit('changed')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    syncing.value = false
  }
}
const stopping = ref(false)
async function stop() {
  stopping.value = false
  try {
    await calendarApi.unlinkCalDAV(props.ws)
    link.value = null
  } catch (e: any) {
    toast.error(e.message)
  }
}

function ago(iso: string): string {
  const s = (new Date(iso).getTime() - Date.now()) / 1000
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  const a = Math.abs(s)
  if (a < 60) return rtf.format(0, 'minute')
  if (a < 3600) return rtf.format(Math.round(s / 60), 'minute')
  if (a < 86400) return rtf.format(Math.round(s / 3600), 'hour')
  return rtf.format(Math.round(s / 86400), 'day')
}
</script>

<template>
  <section v-if="loaded && (link || owner)" class="space-y-2" data-testid="caldav">
    <h3 class="px-1 text-[13px] font-semibold text-text-2">{{ t('cal.dav.title') }}</h3>

    <!-- Linked -->
    <div v-if="link" class="group">
      <div class="group-row !items-start">
        <ArrowLeftRight class="mt-1 h-4 w-4 shrink-0 text-accent" />
        <div class="min-w-0 flex-1">
          <div class="truncate text-[14.5px] font-medium">{{ link.calendar_name || link.host }}</div>
          <div class="flex flex-wrap gap-x-2 text-[12.5px] text-text-3">
            <span class="truncate">{{ link.host }}</span>
            <span class="truncate">{{ t('cal.dav.as', { user: link.username }) }}</span>
            <span>{{ t('cal.nEvents', link.event_count) }}</span>
            <span>{{ link.synced_at ? t('cal.dav.synced', { when: ago(link.synced_at) }) : t('cal.dav.never') }}</span>
          </div>
          <p v-if="link.last_error" class="mt-1 flex items-start gap-1 text-[12.5px] text-crit-text">
            <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span class="break-words">{{ link.last_error }}</span>
          </p>
        </div>
        <div class="-mr-1.5 flex shrink-0 items-center">
          <button class="icon-btn" :title="t('cal.dav.syncNow')" :aria-label="t('cal.dav.syncNow')" :disabled="syncing" @click="syncNow">
            <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': syncing }" />
          </button>
        </div>
      </div>
      <button v-if="owner" class="group-row w-full text-left text-[14px] !text-danger" @click="stopping = true">{{ t('cal.dav.stop') }}</button>
    </div>

    <!-- Set up -->
    <form v-else-if="setup" class="card space-y-3 p-4" @submit.prevent="calendars ? connect() : find()">
      <p class="text-[13px] leading-relaxed text-text-2">{{ t('cal.dav.intro') }}</p>
      <template v-if="!calendars">
        <div>
          <label class="label" for="dav-server">{{ t('cal.dav.server') }}</label>
          <input id="dav-server" v-model="server" class="input" type="url" inputmode="url" autocomplete="off" spellcheck="false" :placeholder="t('cal.dav.serverPlaceholder')" :disabled="finding" />
          <div class="mt-1.5 flex flex-wrap gap-1.5">
            <button v-for="p in presets" :key="p.name" type="button" class="pill hover:bg-fill" @click="server = p.url">{{ p.name }}</button>
          </div>
        </div>
        <div>
          <label class="label" for="dav-user">{{ t('cal.dav.username') }}</label>
          <input id="dav-user" v-model="username" class="input" autocomplete="off" spellcheck="false" :disabled="finding" />
          <p class="mt-1 text-[12px] text-text-3">{{ t('cal.dav.usernameHint') }}</p>
        </div>
        <div>
          <label class="label" for="dav-pass">{{ t('cal.dav.password') }}</label>
          <input id="dav-pass" v-model="password" class="input" type="password" autocomplete="new-password" :disabled="finding" />
          <p class="mt-1 text-[12px] leading-relaxed text-text-3">{{ t('cal.dav.passwordHint') }}</p>
        </div>
      </template>
      <template v-else>
        <div v-if="calendars.length" class="group" role="radiogroup" :aria-label="t('cal.dav.pick')">
          <button v-for="c in calendars" :key="c.url" type="button" class="group-row w-full text-left" role="radio" :aria-checked="chosen === c.url" @click="chosen = c.url">
            <span class="h-3 w-3 shrink-0 rounded-full" :style="{ background: c.color || 'var(--ws)' }" />
            <span class="flex-1 truncate text-[14px]">{{ c.name }}</span>
            <Check v-if="chosen === c.url" class="h-4 w-4 text-accent" />
          </button>
        </div>
        <p v-else class="footnote">{{ t('cal.dav.noCalendars') }}</p>
        <p class="text-[12.5px] leading-relaxed text-text-3">{{ t('cal.dav.first') }}</p>
      </template>
      <p v-if="error" class="text-[12.5px] text-crit-text">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" :disabled="finding || linking" @click="calendars ? (calendars = null) : (setup = false)">{{ calendars ? t('common.back') : t('common.cancel') }}</button>
        <button v-if="!calendars" type="submit" class="btn-primary" :disabled="!canFind">
          <RefreshCw v-if="finding" class="h-4 w-4 animate-spin" />{{ finding ? t('cal.dav.finding') : t('cal.dav.find') }}
        </button>
        <button v-else type="submit" class="btn-primary" :disabled="!chosen || linking">
          <RefreshCw v-if="linking" class="h-4 w-4 animate-spin" />{{ linking ? t('cal.dav.linking') : t('cal.dav.link') }}
        </button>
      </div>
    </form>
    <template v-else>
      <button class="btn-tinted w-full" data-testid="caldav-start" @click="start"><ArrowLeftRight class="h-4 w-4" />{{ t('cal.dav.start') }}</button>
      <p class="footnote px-1 leading-relaxed">{{ t('cal.dav.google') }}</p>
    </template>
  </section>

  <Confirm
    :open="stopping"
    :title="t('cal.dav.stopTitle', { name: link?.calendar_name ?? '' })"
    :message="t('cal.dav.stopBody')"
    :confirm-label="t('cal.dav.stop')"
    danger
    @confirm="stop"
    @close="stopping = false"
  />
</template>

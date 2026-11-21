<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RefreshCw, AlertCircle, ArrowLeftRight, Check, Plus, X, Zap } from 'lucide-vue-next'
import Confirm from '@/components/ui/Confirm.vue'
import { calendarApi, type AccountCalendar, type CalDAVLink, type CalDAVCalendar } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { locale } from '@/lib/dates'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'

// Two-way sync with a calendar on a CalDAV server: owners connect an account
// (server, username, app password), pick one of its calendars and sync;
// editors can sync now. Shown at the top of the calendars sheet.
// googlePending: back from signing in with Google (a calendar to pick).
const props = defineProps<{ ws: string; open: boolean; owner: boolean; googlePending?: string }>()
const emit = defineEmits<{ changed: []; googleDone: [] }>()
const toast = useToastStore()
const { t } = useI18n()

const link = ref<CalDAVLink | null>(null)
const links = ref<CalDAVLink[]>([])
const loaded = ref(false)
const syncing = ref(false)
// More calendars of the same account, each its own calendar here.
const extras = computed(() => links.value.filter((l) => l.extra))
function take(r: { link: CalDAVLink | null; links?: CalDAVLink[] }) {
  link.value = r.link
  links.value = r.links ?? (r.link ? [r.link] : [])
}

async function load() {
  try {
    take(await calendarApi.caldav(props.ws))
  } catch {
    take({ link: null })
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
    void calendarApi.googleStatus().then((s) => (googleOn.value = s.available), () => (googleOn.value = false))
    if (props.googlePending) void loadGoogle(props.googlePending)
  },
  { immediate: true },
)

// ---- Google Calendar: sign in with Google, then pick a calendar ----
const googleOn = ref(false)
const google = ref<{ pending: string; email: string; calendars: { id: string; name: string; color: string; writable?: boolean }[] } | null>(null)
const googleChosen = ref('')
async function signInGoogle() {
  try {
    location.href = (await calendarApi.googleStart(props.ws)).url
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function loadGoogle(pending: string) {
  error.value = ''
  try {
    const r = await calendarApi.googleCalendars(props.ws, pending)
    google.value = { pending, ...r }
    // The workspace's own Events go up to it: one that may be written.
    googleChosen.value = r.calendars.find((c) => c.writable !== false)?.id ?? ''
  } catch (e: any) {
    toast.error(e.message)
    emit('googleDone')
  }
}
async function linkGoogle() {
  const g = google.value
  const cal = g?.calendars.find((c) => c.id === googleChosen.value)
  if (!g || !cal || linking.value) return
  linking.value = true
  error.value = ''
  try {
    take(await calendarApi.googleLink(props.ws, { pending: g.pending, calendar_id: cal.id, calendar_name: cal.name }))
    google.value = null
    emit('googleDone')
    emit('changed')
  } catch (e: any) {
    error.value = e.message
  } finally {
    linking.value = false
  }
}
function cancelGoogle() {
  google.value = null
  emit('googleDone')
}

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
    take(await calendarApi.setCalDAV(props.ws, { url: server.value.trim(), username: username.value.trim(), password: password.value, calendar_url: cal.url, calendar_name: cal.name }))
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
    take(await calendarApi.syncCalDAV(props.ws))
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
    take({ link: null })
    emit('changed')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- More calendars of the account ----
const more = ref<AccountCalendar[] | null>(null)
const moreChosen = ref('')
const adding = ref(false)
async function openMore() {
  if (!link.value) return
  error.value = ''
  try {
    more.value = (await calendarApi.accountCalendars(props.ws, link.value.id)).filter((c) => !c.linked)
    moreChosen.value = more.value[0]?.key ?? ''
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function addMore() {
  if (!link.value || !moreChosen.value || adding.value) return
  adding.value = true
  error.value = ''
  try {
    take(await calendarApi.addCalendar(props.ws, link.value.id, { key: moreChosen.value }))
    more.value = null
    emit('changed')
  } catch (e: any) {
    error.value = e.message
  } finally {
    adding.value = false
  }
}
const removing = ref<CalDAVLink | null>(null)
async function removeExtra() {
  const l = removing.value
  removing.value = null
  if (!l) return
  try {
    take(await calendarApi.removeCalendar(props.ws, l.id))
    emit('changed')
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
      <div class="group-row items-start!">
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
            <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span class="wrap-break-word">{{ link.last_error }}</span>
          </p>
          <!-- Google: changes there come at once while watched; else every 5 minutes, and why -->
          <p v-if="link.provider === 'google' && link.watch_until && !link.watch_error" class="mt-1 flex items-start gap-1 text-[12.5px] text-text-3" data-testid="dav-instant">
            <Zap class="mt-0.5 h-3.5 w-3.5 shrink-0 text-accent" /><span>{{ link.pushed_at ? t('cal.dav.instantLast', { when: ago(link.pushed_at) }) : t('cal.dav.instant') }}</span>
          </p>
          <p v-else-if="link.provider === 'google' && link.watch_error" class="mt-1 flex items-start gap-1 text-[12.5px] text-text-3" data-testid="dav-instant-off">
            <Zap class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span class="wrap-break-word">{{ t('cal.dav.instantOff', { why: link.watch_error }) }}</span>
          </p>
        </div>
        <div class="-mr-1.5 flex shrink-0 items-center">
          <Button variant="ghost" size="icon" class="text-text-2" :title="t('cal.dav.syncNow')" :aria-label="t('cal.dav.syncNow')" :disabled="syncing" @click="syncNow">
            <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': syncing }" />
          </Button>
        </div>
      </div>
      <!-- More calendars of the account, each its own calendar here -->
      <div v-for="x in extras" :key="x.id" class="group-row items-start!" data-testid="dav-extra">
        <span class="mt-1.5 h-3 w-3 shrink-0 rounded-full" :style="{ background: x.color || 'var(--ws)' }" />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="truncate text-[14.5px] font-medium">{{ x.calendar_name || x.host }}</span>
            <span v-if="x.read_only" class="pill shrink-0">{{ t('cal.dav.readOnly') }}</span>
          </div>
          <div class="flex flex-wrap gap-x-2 text-[12.5px] text-text-3">
            <span>{{ t('cal.nEvents', x.event_count) }}</span>
            <span>{{ x.synced_at ? t('cal.dav.synced', { when: ago(x.synced_at) }) : t('cal.dav.never') }}</span>
          </div>
          <p v-if="x.last_error" class="mt-1 flex items-start gap-1 text-[12.5px] text-crit-text">
            <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span class="wrap-break-word">{{ x.last_error }}</span>
          </p>
        </div>
        <Button v-if="owner" variant="ghost" size="icon" class="-mr-1.5 text-text-2" :title="t('cal.dav.removeOne', { name: x.calendar_name })" :aria-label="t('cal.dav.removeOne', { name: x.calendar_name })" @click="removing = x"><X class="h-4 w-4" /></Button>
      </div>
      <template v-if="owner">
        <div v-if="more" class="group-row block! space-y-2" data-testid="dav-more">
          <p class="text-[12.5px] leading-relaxed text-text-3">{{ t('cal.dav.moreIntro') }}</p>
          <div v-if="more.length" role="radiogroup" :aria-label="t('cal.dav.pick')">
            <button v-for="c in more" :key="c.key" type="button" class="flex w-full items-center gap-2.5 rounded-lg px-1 py-1.5 text-left" role="radio" :aria-checked="moreChosen === c.key" @click="moreChosen = c.key">
              <span class="h-3 w-3 shrink-0 rounded-full" :style="{ background: c.color || 'var(--ws)' }" />
              <span class="flex-1 truncate text-[14px]">{{ c.name }}</span>
              <span v-if="!c.writable" class="pill shrink-0">{{ t('cal.dav.readOnly') }}</span>
              <Check v-if="moreChosen === c.key" class="h-4 w-4 text-accent" />
            </button>
          </div>
          <p v-else class="footnote">{{ t('cal.dav.noMore') }}</p>
          <p v-if="error" class="text-[12.5px] text-crit-text">{{ error }}</p>
          <div class="flex justify-end gap-2">
            <Button variant="secondary" size="sm" type="button" :disabled="adding" @click="more = null">{{ t('common.cancel') }}</Button>
            <Button size="sm" type="button" :disabled="!moreChosen || adding" data-testid="dav-more-add" @click="addMore">
              <RefreshCw v-if="adding" class="h-4 w-4 animate-spin" />{{ adding ? t('cal.dav.linking') : t('cal.dav.add') }}
            </Button>
          </div>
        </div>
        <button v-else class="group-row w-full text-left text-[14px] text-accent" data-testid="dav-more-open" @click="openMore"><Plus class="h-4 w-4" />{{ t('cal.dav.more') }}</button>
        <button class="group-row w-full text-left text-[14px] text-danger!" @click="stopping = true">{{ t('cal.dav.stop') }}</button>
      </template>
    </div>

<!-- Back from Google: one of the account's calendars -->
    <form v-else-if="google" class="card space-y-3 p-4" data-testid="google-pick" @submit.prevent="linkGoogle">
      <p class="text-[13px] leading-relaxed text-text-2">{{ t('cal.dav.googlePick', { email: google.email }) }}</p>
      <div v-if="google.calendars.length" class="group" role="radiogroup" :aria-label="t('cal.dav.pick')">
        <button
          v-for="c in google.calendars"
          :key="c.id"
          type="button"
          class="group-row w-full text-left disabled:opacity-50"
          role="radio"
          :aria-checked="googleChosen === c.id"
          :disabled="c.writable === false"
          :title="c.writable === false ? t('cal.dav.readOnlyFirst') : undefined"
          @click="googleChosen = c.id"
        >
          <span class="h-3 w-3 shrink-0 rounded-full" :style="{ background: c.color || 'var(--ws)' }" />
          <span class="flex-1 truncate text-[14px]">{{ c.name }}</span>
          <span v-if="c.writable === false" class="pill shrink-0">{{ t('cal.dav.readOnly') }}</span>
          <Check v-if="googleChosen === c.id" class="h-4 w-4 text-accent" />
        </button>
      </div>
      <p v-else class="footnote">{{ t('cal.dav.noCalendars') }}</p>
      <p class="text-[12.5px] leading-relaxed text-text-3">{{ t('cal.dav.first') }}</p>
      <p v-if="error" class="text-[12.5px] text-crit-text">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="secondary" type="button" :disabled="linking" @click="cancelGoogle">{{ t('common.cancel') }}</Button>
        <Button type="submit" :disabled="!googleChosen || linking" data-testid="google-link">
          <RefreshCw v-if="linking" class="h-4 w-4 animate-spin" />{{ linking ? t('cal.dav.linking') : t('cal.dav.link') }}
        </Button>
      </div>
    </form>

    <!-- Set up -->
    <form v-else-if="setup" class="card space-y-3 p-4" @submit.prevent="calendars ? connect() : find()">
      <p class="text-[13px] leading-relaxed text-text-2">{{ t('cal.dav.intro') }}</p>
      <template v-if="!calendars">
        <div>
          <label class="label" for="dav-server">{{ t('cal.dav.server') }}</label>
          <Input id="dav-server" v-model="server" type="url" inputmode="url" autocomplete="off" spellcheck="false" :placeholder="t('cal.dav.serverPlaceholder')" :disabled="finding" />
          <div class="mt-1.5 flex flex-wrap gap-1.5">
            <button v-for="p in presets" :key="p.name" type="button" class="pill hover:bg-fill" @click="server = p.url">{{ p.name }}</button>
          </div>
        </div>
        <div>
          <label class="label" for="dav-user">{{ t('cal.dav.username') }}</label>
          <Input id="dav-user" v-model="username" autocomplete="off" spellcheck="false" :disabled="finding" />
          <p class="mt-1 text-[12px] text-text-3">{{ t('cal.dav.usernameHint') }}</p>
        </div>
        <div>
          <label class="label" for="dav-pass">{{ t('cal.dav.password') }}</label>
          <Input id="dav-pass" v-model="password" type="password" autocomplete="new-password" :disabled="finding" />
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
        <Button variant="secondary" type="button" :disabled="finding || linking" @click="calendars ? (calendars = null) : (setup = false)">{{ calendars ? t('common.back') : t('common.cancel') }}</Button>
        <Button v-if="!calendars" type="submit" :disabled="!canFind">
          <RefreshCw v-if="finding" class="h-4 w-4 animate-spin" />{{ finding ? t('cal.dav.finding') : t('cal.dav.find') }}
        </Button>
        <Button v-else type="submit" :disabled="!chosen || linking">
          <RefreshCw v-if="linking" class="h-4 w-4 animate-spin" />{{ linking ? t('cal.dav.linking') : t('cal.dav.link') }}
        </Button>
      </div>
    </form>
    <template v-else>
      <Button variant="outline" v-if="googleOn" class="w-full" data-testid="google-start" @click="signInGoogle"><ArrowLeftRight class="h-4 w-4" />{{ t('cal.dav.googleStart') }}</Button>
      <Button variant="outline" class="w-full" data-testid="caldav-start" @click="start"><ArrowLeftRight class="h-4 w-4" />{{ t('cal.dav.start') }}</Button>
      <p v-if="!googleOn" class="footnote px-1 leading-relaxed">{{ t('cal.dav.google') }}</p>
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
  <Confirm
    :open="!!removing"
    :title="t('cal.dav.removeTitle', { name: removing?.calendar_name ?? '' })"
    :message="t('cal.dav.removeBody')"
    :confirm-label="t('cal.dav.remove')"
    danger
    @confirm="removeExtra"
    @close="removing = null"
  />
</template>

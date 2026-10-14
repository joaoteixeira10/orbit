<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, RefreshCw, Pencil, Trash2, Check, Link2, AlertCircle } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { calendarApi, type Subscription } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { COLORS } from '@/lib/icons'
import { locale } from '@/lib/dates'

// A workspace's subscribed calendars (ICS feeds from Google, iCloud,
// Outlook…): add one by its secret address, rename or recolour it, fetch it
// now, or remove it. Only the host of an address is ever shown again.
const props = defineProps<{ open: boolean; ws: string }>()
const emit = defineEmits<{ close: []; changed: [] }>()
const toast = useToastStore()
const { t } = useI18n()

const subs = ref<Subscription[]>([])
const loaded = ref(false)
const busy = ref<Record<string, boolean>>({})

async function load() {
  try {
    subs.value = await calendarApi.subscriptions(props.ws)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(
  () => [props.open, props.ws] as const,
  ([o]) => {
    if (!o) return
    loaded.value = false
    adding.value = false
    editId.value = ''
    void load()
  },
  { immediate: true },
)
// Someone else (or the hourly refresh) changed them.
const off = onLiveEvent('ws', (e) => {
  if (!props.open || e.origin === CLIENT_ID || String(e.ws) !== props.ws || e.what !== 'subscriptions') return
  void load()
})
onUnmounted(off)

function put(s: Subscription) {
  const i = subs.value.findIndex((x) => x.id === s.id)
  if (i >= 0) subs.value[i] = s
  else subs.value.push(s)
  emit('changed')
}

// ---- Add ----
const adding = ref(false)
const name = ref('')
const url = ref('')
const color = ref(COLORS[0])
const addError = ref('')
const subscribing = ref(false)
function startAdd() {
  name.value = ''
  url.value = ''
  color.value = COLORS[subs.value.length % COLORS.length]
  addError.value = ''
  adding.value = true
}
const urlOk = computed(() => /^(https?|webcals?):\/\/\S+$/i.test(url.value.trim()))
async function add() {
  if (!urlOk.value || subscribing.value) return
  subscribing.value = true
  addError.value = ''
  try {
    // The server fetches it at once: a wrong address answers with why.
    put(await calendarApi.subscribe(props.ws, { name: name.value.trim() || undefined, url: url.value.trim(), color: color.value }))
    adding.value = false
  } catch (e: any) {
    addError.value = e.message
  } finally {
    subscribing.value = false
  }
}

// ---- Edit, refresh, remove ----
const editId = ref('')
const editName = ref('')
const editColor = ref('')
function startEdit(s: Subscription) {
  editId.value = s.id
  editName.value = s.name
  editColor.value = s.color
}
async function saveEdit(s: Subscription) {
  const body: { name?: string; color?: string } = {}
  if (editName.value.trim() && editName.value.trim() !== s.name) body.name = editName.value.trim()
  if (editColor.value !== s.color) body.color = editColor.value
  editId.value = ''
  if (!body.name && !body.color) return
  try {
    put(await calendarApi.updateSubscription(s.id, body))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function refresh(s: Subscription) {
  busy.value[s.id] = true
  try {
    put(await calendarApi.refreshSubscription(s.id))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value[s.id] = false
  }
}
const removing = ref<Subscription | null>(null)
async function remove() {
  const s = removing.value
  if (!s) return
  removing.value = null
  try {
    await calendarApi.unsubscribe(s.id)
    subs.value = subs.value.filter((x) => x.id !== s.id)
    emit('changed')
  } catch (e: any) {
    toast.error(e.message)
  }
}

// "5 minutes ago", in the UI's language.
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
  <Overlay :open="open" :title="t('cal.subsTitle')" size="md" @close="emit('close')">
    <div class="space-y-4">
      <div v-if="subs.length" class="group">
        <div v-for="s in subs" :key="s.id">
          <!-- Editing: name and colour -->
          <div v-if="editId === s.id" class="space-y-3 px-3.5 py-3">
            <input v-model="editName" class="input" maxlength="120" :aria-label="t('cal.subName')" @keydown.enter="saveEdit(s)" @keydown.esc.stop="editId = ''" />
            <div class="flex flex-wrap items-center gap-1.5" role="radiogroup" :aria-label="t('cal.color')">
              <button v-for="c in COLORS" :key="c" type="button" class="swatch" role="radio" :aria-checked="editColor.toLowerCase() === c.toLowerCase()" :aria-label="c" :style="{ '--sw': c }" @click="editColor = c">
                <Check v-if="editColor.toLowerCase() === c.toLowerCase()" class="h-3 w-3" :stroke-width="3.4" />
              </button>
            </div>
            <div class="flex justify-end gap-2">
              <button class="btn btn-sm" @click="editId = ''">{{ t('common.cancel') }}</button>
              <button class="btn-primary btn-sm" @click="saveEdit(s)">{{ t('common.save') }}</button>
            </div>
          </div>
          <div v-else class="group-row !items-start">
            <span class="mt-1.5 h-3 w-3 shrink-0 rounded-full" :style="{ background: s.color }" />
            <div class="min-w-0 flex-1">
              <div class="truncate text-[14.5px] font-medium">{{ s.name }}</div>
              <div class="flex flex-wrap gap-x-2 text-[12.5px] text-text-3">
                <span class="truncate">{{ s.url_host }}</span>
                <span>{{ t('cal.nEvents', s.event_count) }}</span>
                <span>{{ s.fetched_at ? t('cal.fetched', { when: ago(s.fetched_at) }) : t('cal.neverFetched') }}</span>
              </div>
              <p v-if="s.last_error" class="mt-1 flex items-start gap-1 text-[12.5px] text-crit-text">
                <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" /><span class="break-words">{{ s.last_error }}</span>
              </p>
            </div>
            <div class="-mr-1.5 flex shrink-0 items-center">
              <button class="icon-btn" :title="t('cal.refresh')" :aria-label="t('cal.refresh')" :disabled="busy[s.id]" @click="refresh(s)">
                <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': busy[s.id] }" />
              </button>
              <button class="icon-btn" :title="t('common.edit')" :aria-label="t('common.edit')" @click="startEdit(s)"><Pencil class="h-4 w-4" /></button>
              <button class="icon-btn !text-danger" :title="t('cal.unsubscribe')" :aria-label="t('cal.unsubscribe')" @click="removing = s"><Trash2 class="h-4 w-4" /></button>
            </div>
          </div>
        </div>
      </div>
      <p v-else-if="loaded && !adding" class="footnote py-4 text-center">{{ t('cal.subsEmpty') }}</p>

      <!-- Add one -->
      <form v-if="adding" class="card space-y-3 p-4" @submit.prevent="add">
        <div>
          <label class="label" for="sub-url">{{ t('cal.subUrl') }}</label>
          <div class="relative">
            <Link2 class="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-3" />
            <input
              id="sub-url"
              v-model="url"
              class="input !pl-8"
              type="url"
              inputmode="url"
              autocomplete="off"
              spellcheck="false"
              :placeholder="t('cal.subUrlPlaceholder')"
              :disabled="subscribing"
              required
            />
          </div>
          <p v-if="addError" class="mt-1.5 text-[12.5px] text-crit-text">{{ addError }}</p>
        </div>
        <div>
          <label class="label" for="sub-name">{{ t('cal.subName') }}</label>
          <input id="sub-name" v-model="name" class="input" maxlength="120" :placeholder="t('cal.subNamePlaceholder')" :disabled="subscribing" />
        </div>
        <div class="flex flex-wrap items-center gap-1.5" role="radiogroup" :aria-label="t('cal.color')">
          <button v-for="c in COLORS" :key="c" type="button" class="swatch" role="radio" :aria-checked="color === c" :aria-label="c" :style="{ '--sw': c }" @click="color = c">
            <Check v-if="color === c" class="h-3 w-3" :stroke-width="3.4" />
          </button>
        </div>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn" :disabled="subscribing" @click="adding = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary" :disabled="!urlOk || subscribing">
            <RefreshCw v-if="subscribing" class="h-4 w-4 animate-spin" />{{ subscribing ? t('cal.subscribing') : t('cal.subscribe') }}
          </button>
        </div>
      </form>
      <button v-else class="btn-tinted w-full" @click="startAdd"><Plus class="h-4 w-4" />{{ t('cal.addCalendar') }}</button>

      <p class="footnote px-1 leading-relaxed">{{ t('cal.subsHint') }}</p>
    </div>
    <template #footer>
      <button class="btn-primary" @click="emit('close')">{{ t('common.done') }}</button>
    </template>
  </Overlay>

  <Confirm
    :open="!!removing"
    :title="t('cal.unsubscribeTitle', { name: removing?.name ?? '' })"
    :message="t('cal.unsubscribeBody')"
    :confirm-label="t('cal.unsubscribe')"
    danger
    @confirm="remove"
    @close="removing = null"
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

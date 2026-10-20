<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { BellRing, Smartphone, X } from 'lucide-vue-next'
import Toggle from '@/components/ui/Toggle.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import PersonalChannels from './PersonalChannels.vue'
import { prefsApi, notifyApi } from '@/api'
import { pushApi, type PushDevice } from '@/api/planner'
import type { Delivery, NotifyKind, NotifyPrefs } from '@/api/types'
import { pushState, refreshPush, enablePush, disablePush } from '@/lib/push'
import { parseUA } from '@/lib/format'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Settings → Notifications: push on this device (and the others), what each
// kind of notification reaches besides the bell (devices, channels), the
// daily summary, and the channels to use. Everything saves as it changes.
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const KINDS: NotifyKind[] = ['reminder', 'assigned', 'comment', 'workspace', 'activity', 'digest']
const prefs = ref<NotifyPrefs | null>(null)
const enabledChannels = ref<string[]>([])
const personalKinds = ref<string[]>([])
const devices = ref<PushDevice[]>([])
const pushAvailable = ref(true)
const busy = ref(false)

async function loadDevices() {
  const info = await pushApi.info()
  pushAvailable.value = info.available
  devices.value = info.devices
}
onMounted(async () => {
  try {
    const [p, chans] = await Promise.all([prefsApi.get(), notifyApi.channels(), loadDevices(), refreshPush()])
    prefs.value = p
    enabledChannels.value = chans.enabled ?? []
    personalKinds.value = chans.personal ?? []
  } catch (e: any) {
    toast.error(e.message)
  }
})

async function save(patch: Partial<NotifyPrefs>) {
  try {
    prefs.value = await prefsApi.update(patch)
  } catch (e: any) {
    toast.error(e.message)
  }
}
function setKind(k: NotifyKind, field: keyof Delivery, on: boolean) {
  if (!prefs.value) return
  const d = { ...prefs.value.notify_prefs[k], [field]: on }
  prefs.value.notify_prefs[k] = d
  void save({ notify_prefs: { [k]: d } as NotifyPrefs['notify_prefs'] })
}
function toggleChannel(ch: string, on: boolean) {
  if (!prefs.value) return
  const list = on ? [...prefs.value.notify_channels, ch] : prefs.value.notify_channels.filter((c) => c !== ch)
  void save({ notify_channels: list })
}
const channelLabel = (ch: string) => (ch === 'email' ? t('notif.chEmail', { email: auth.user?.email || '—' }) : ch.charAt(0).toUpperCase() + ch.slice(1))
// Their own channels, when the owner allows them.
const ownOn = computed(() => auth.features.personal_channels !== false)
const ownCount = ref(0)
// The first channel of one's own: what's about you starts going there (the
// Channels column was hidden until now, all off).
function onOwnAdded(n: number) {
  if (n !== 1 || enabledChannels.value.length || !prefs.value) return
  const p = prefs.value.notify_prefs
  if ((Object.keys(p) as NotifyKind[]).some((k) => p[k].channels)) return
  const next = { ...p }
  for (const k of ['reminder', 'assigned', 'comment', 'workspace'] as NotifyKind[]) next[k] = { ...next[k], channels: true }
  void save({ notify_prefs: next })
}
const hasChannels = computed(() => enabledChannels.value.length > 0 || ownCount.value > 0)

// This device.
async function turnOn() {
  busy.value = true
  try {
    await enablePush()
    await loadDevices()
  } catch (e: any) {
    toast.error(e.message === 'push unavailable' ? t('notif.unavailable') : e.message)
  } finally {
    busy.value = false
  }
}
async function turnOff() {
  busy.value = true
  try {
    await disablePush()
    await loadDevices()
  } finally {
    busy.value = false
  }
}
async function test() {
  try {
    const r = await pushApi.test()
    toast.success(t('notif.testSent', { n: r.devices }, r.devices))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function removeDevice(d: PushDevice) {
  await pushApi.remove({ id: d.id }).catch((e) => toast.error(e.message))
  await loadDevices()
  await refreshPush()
}
const deviceName = (d: PushDevice) => {
  const ua = parseUA(d.user_agent)
  return `${ua.browser} · ${ua.os}`
}
const isThis = (d: PushDevice) => pushState.value === 'on' && d.user_agent === navigator.userAgent
</script>

<template>
  <div v-if="prefs" class="flex flex-col gap-5">
    <p class="text-[13px] text-text-2">{{ t('notif.intro') }}</p>

    <!-- This device -->
    <div class="flex items-start justify-between gap-4">
      <div class="flex min-w-0 gap-3">
        <span class="tile mt-0.5 h-8 w-8 shrink-0" style="--tile: #ff3b30"><BellRing class="h-[17px] w-[17px]" /></span>
        <div class="min-w-0">
          <div class="text-[13px] font-semibold">{{ t('notif.device') }}</div>
          <div class="text-[12px] text-text-3">
            <template v-if="!pushAvailable">{{ t('notif.unavailable') }}</template>
            <template v-else-if="pushState === 'ios-install'">{{ t('notif.iosHint') }}</template>
            <template v-else-if="pushState === 'unsupported'">{{ t('notif.unsupported') }}</template>
            <template v-else-if="pushState === 'denied'">{{ t('notif.blocked') }}</template>
            <template v-else>{{ t('notif.deviceHint') }}</template>
          </div>
        </div>
      </div>
      <div v-if="pushAvailable" class="flex shrink-0 items-center gap-2">
        <template v-if="pushState === 'on'">
          <span class="pill !bg-ok-bg !text-ok-text">{{ t('notif.deviceOn') }}</span>
          <button class="btn btn-sm" @click="test">{{ t('notif.sendTest') }}</button>
          <button class="btn-plain !text-[13px]" :disabled="busy" @click="turnOff">{{ t('notif.turnOff') }}</button>
        </template>
        <button v-else-if="pushState === 'off'" class="btn-primary btn-sm" :disabled="busy" @click="turnOn">{{ t('notif.turnOn') }}</button>
      </div>
    </div>

    <div v-if="devices.length" class="rounded-xl bg-fill-2 px-3 py-2">
      <div class="cap mb-1">{{ t('notif.otherDevices') }}</div>
      <div v-for="d in devices" :key="d.id" class="flex items-center gap-2.5 py-1.5 text-[13px]">
        <Smartphone class="h-4 w-4 shrink-0 text-text-3" />
        <span class="min-w-0 flex-1 truncate">
          {{ deviceName(d) }}
          <span v-if="isThis(d)" class="text-text-3">· {{ t('notif.thisDevice') }}</span>
          <span v-if="d.last_used_at" class="text-[12px] text-text-3"> · {{ t('notif.lastUsed', { when: ago(d.last_used_at, locale()) }) }}</span>
        </span>
        <button class="icon-btn h-6 w-6" :title="t('notif.removeDevice')" :aria-label="t('notif.removeDevice')" @click="removeDevice(d)"><X class="h-3.5 w-3.5" /></button>
      </div>
    </div>

    <!-- Daily summary -->
    <div class="flex items-center justify-between gap-4 border-t border-border pt-5">
      <div class="min-w-0">
        <div class="text-[13px] font-semibold">{{ t('notif.digest') }}</div>
        <div class="text-[12px] text-text-3">{{ t('notif.digestHint') }}</div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <label v-if="prefs.digest_enabled" class="flex items-center gap-1.5 text-[13px] text-text-2">
          {{ t('notif.digestAt') }}
          <input type="time" class="input !w-auto !py-1 text-[13px]" :value="prefs.digest_time" @change="save({ digest_time: ($event.target as HTMLInputElement).value })" />
        </label>
        <Toggle :model-value="prefs.digest_enabled" :aria-label="t('notif.digest')" @update:model-value="(v) => save({ digest_enabled: v })" />
      </div>
    </div>

    <!-- What reaches you -->
    <div class="border-t border-border pt-5">
      <div class="mb-2 flex items-end gap-3">
        <div class="flex-1 text-[13px] font-semibold">{{ t('notif.whatTitle') }}</div>
        <div class="col">{{ t('notif.colDevice') }}</div>
        <div v-if="hasChannels" class="col">{{ t('notif.colChannels') }}</div>
      </div>
      <div class="group">
        <div v-for="k in KINDS.filter((x) => x !== 'digest' || prefs!.digest_enabled)" :key="k" class="group-row">
          <div class="min-w-0 flex-1">
            <div class="text-[13.5px]">{{ t(`notif.kinds.${k}.label`) }}</div>
            <div class="text-[12px] text-text-3">{{ t(`notif.kinds.${k}.desc`) }}</div>
          </div>
          <div class="col"><Toggle :model-value="prefs.notify_prefs[k].push" :aria-label="`${t(`notif.kinds.${k}.label`)} — ${t('notif.colDevice')}`" @update:model-value="(v) => setKind(k, 'push', v)" /></div>
          <div v-if="hasChannels" class="col"><Toggle :model-value="prefs.notify_prefs[k].channels" :aria-label="`${t(`notif.kinds.${k}.label`)} — ${t('notif.colChannels')}`" @update:model-value="(v) => setKind(k, 'channels', v)" /></div>
        </div>
      </div>
    </div>

    <!-- Channels -->
    <div class="border-t border-border pt-5">
      <div class="text-[13px] font-semibold">{{ t('notif.channels') }}</div>
      <template v-if="enabledChannels.length">
        <div class="text-[12px] text-text-3">{{ t('notif.channelsHint') }}</div>
        <div class="mt-3 flex flex-wrap gap-x-5 gap-y-2">
          <Checkbox v-for="ch in enabledChannels" :key="ch" :model-value="prefs.notify_channels.includes(ch)" @update:model-value="(v) => toggleChannel(ch, v)">
            {{ channelLabel(ch) }}
          </Checkbox>
        </div>
        <!-- Chats, topics and webhooks are the owner's: others may read them. -->
        <div v-if="prefs.notify_channels.some((c) => c !== 'email')" class="mt-4 flex items-start gap-3 rounded-xl bg-fill-2 p-3" data-testid="channel-details">
          <div class="min-w-0 flex-1">
            <div class="text-[13.5px]">{{ t('notif.details') }}</div>
            <div class="mt-0.5 text-[12px] text-text-3">{{ prefs.channel_details ? t('notif.detailsOn') : t('notif.detailsOff') }}</div>
          </div>
          <Toggle :model-value="prefs.channel_details" :aria-label="t('notif.details')" @update:model-value="(v) => save({ channel_details: v })" />
        </div>
      </template>
      <p v-else-if="!ownOn" class="mt-2 rounded-xl bg-fill-2 p-3 text-[12px] text-text-3">{{ t('notif.noChannels') }}</p>
      <PersonalChannels v-if="ownOn" class="mt-5" :allowed="personalKinds" @count="(n) => (ownCount = n)" @added="onOwnAdded" />
    </div>
  </div>
</template>

<style scoped>
.col {
  display: flex;
  justify-content: center;
  width: 84px;
  flex: none;
  font-size: 11.5px;
  font-weight: 500;
  color: var(--text-3);
  text-align: center;
}
</style>

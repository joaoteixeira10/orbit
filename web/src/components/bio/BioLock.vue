<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Fingerprint, Lock } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useBrandingStore } from '@/stores/branding'
import { bioApi, bioKind, deviceBio, lastActive, lockAfter, lockChoice, markActive } from '@/lib/biometric'

// The lock: on a device with Face ID turned on and a lock chosen (or the
// owner's rule asking for one), Orbit covers itself after that long idle —
// or, with "when I come back", each time you come back to it — until Face
// ID; Sign out is the other way. A cover for prying eyes: the session stays.
const auth = useAuthStore()
const branding = useBrandingStore()
const router = useRouter()
const { t } = useI18n()
const kind = computed(() => t(`bio.kind.${bioKind()}`))

const choice = ref(lockChoice())
const minutes = computed(() => {
  const d = deviceBio()
  if (!branding.biometricEnabled || !auth.user || auth.impersonator || !d || d.user !== auth.user.id) return -1
  return lockAfter(choice.value, branding.bioLockMax, branding.bioLockRequired)
})
const locked = ref(false)
const busy = ref(false)
const failed = ref('')

function lock() {
  if (locked.value) return
  locked.value = true
  void unlock(true)
}
// unlock asks for Face ID (at once when the lock comes down; a browser may
// want a tap first — then the button).
async function unlock(auto = false) {
  if (busy.value) return
  busy.value = true
  failed.value = ''
  try {
    await bioApi.unlock()
    locked.value = false
    markActive()
  } catch (e: any) {
    if (!auto) failed.value = e?.name === 'NotAllowedError' ? t('bio.cancelled', { kind: kind.value }) : t('bio.failed', { kind: kind.value, error: e?.message ?? '' })
  } finally {
    busy.value = false
  }
}
async function signOut() {
  locked.value = false
  await auth.logout()
  void router.push('/login')
}

// Idle: any touch, key or wheel counts (written now and then, shared by tabs).
let wrote = 0
function activity() {
  if (locked.value || Date.now() - wrote < 5000) return
  wrote = Date.now()
  markActive(wrote)
}
function check() {
  const m = minutes.value
  if (m > 0 && !locked.value && Date.now() - lastActive() > m * 60000) lock()
}
let hiddenAt = 0
function onVisibility() {
  if (document.hidden) {
    hiddenAt = Date.now()
    return
  }
  if (minutes.value === 0 && hiddenAt && Date.now() - hiddenAt > 1000) lock()
  else check()
}
const rechoose = () => (choice.value = lockChoice())
const EVENTS = ['pointerdown', 'keydown', 'wheel', 'touchstart'] as const
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  const m = minutes.value
  // Opening Orbit: locked when coming back is the rule, or it sat idle too long.
  if (m === 0 || (m > 0 && lastActive() && Date.now() - lastActive() > m * 60000)) lock()
  else markActive()
  for (const e of EVENTS) window.addEventListener(e, activity, { passive: true, capture: true })
  document.addEventListener('visibilitychange', onVisibility)
  window.addEventListener('orbit:bio-lock', rechoose)
  timer = setInterval(check, 15000)
})
onUnmounted(() => {
  for (const e of EVENTS) window.removeEventListener(e, activity, { capture: true })
  document.removeEventListener('visibilitychange', onVisibility)
  window.removeEventListener('orbit:bio-lock', rechoose)
  clearInterval(timer)
})
</script>

<template>
  <Teleport to="body">
    <div v-if="locked" class="bio-lock" role="dialog" aria-modal="true" :aria-label="t('bio.locked')" data-testid="bio-lock-screen">
      <div class="flex w-full max-w-[320px] flex-col items-center text-center">
        <span class="mb-5 grid h-16 w-16 place-items-center rounded-full bg-fill"><Lock class="h-7 w-7 text-text-2" /></span>
        <h1 class="title-2">{{ t('bio.locked') }}</h1>
        <p class="mt-1 text-[14px] text-text-2">{{ t('bio.lockedHint', { kind }) }}</p>
        <button type="button" class="btn-primary mt-6 w-full justify-center" :disabled="busy" data-testid="bio-unlock" @click="unlock()">
          <Fingerprint class="h-4 w-4" />{{ t('bio.unlock', { kind }) }}
        </button>
        <p v-if="failed" class="mt-3 text-[13px] text-crit-text" role="alert">{{ failed }}</p>
        <button type="button" class="btn-plain mt-4 !text-text-3" @click="signOut">{{ t('bio.signOut') }}</button>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* Over everything, nothing showing through. */
.bio-lock {
  position: fixed;
  inset: 0;
  z-index: 300;
  display: grid;
  place-items: center;
  padding: 24px;
  background: var(--bg);
}
</style>

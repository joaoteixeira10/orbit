<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Fingerprint, X } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useBrandingStore } from '@/stores/branding'
import { useToastStore } from '@/stores/toast'
import { parseUA } from '@/lib/format'
import { bioApi, bioAvailable, bioKind, deviceBio, markActive } from '@/lib/biometric'

// After signing in with a password or SSO on a device that has Face ID (and
// isn't using it yet), Orbit asks once: Face ID next time? "Not now" isn't
// asked again on this device (Settings still has the switch).
const auth = useAuthStore()
const branding = useBrandingStore()
const toast = useToastStore()
const { t } = useI18n()
const kind = computed(() => t(`bio.kind.${bioKind()}`))
const show = ref(false)
const busy = ref(false)
const noKey = () => `orbit_bio_offer_no:${auth.user?.id ?? ''}`

onMounted(async () => {
  let asked = false
  try {
    asked = sessionStorage.getItem('orbit_bio_offer') === '1'
    sessionStorage.removeItem('orbit_bio_offer')
    if (localStorage.getItem(noKey())) return
  } catch {
    return
  }
  const d = deviceBio()
  if (!asked || !branding.biometricEnabled || !auth.webauthnEnabled || !auth.user || auth.impersonator || (d && d.user === auth.user.id)) return
  show.value = await bioAvailable()
})
async function yes() {
  busy.value = true
  try {
    await bioApi.turnOn(auth.user!.id, `${kind.value} · ${parseUA(navigator.userAgent).os}`)
    await auth.fetchMe()
    markActive()
    toast.success(t('bio.turnedOn', { kind: kind.value }))
    show.value = false
  } catch (e: any) {
    toast.error(e?.name === 'NotAllowedError' ? t('bio.cancelled', { kind: kind.value }) : e?.name === 'InvalidStateError' ? t('bio.already', { kind: kind.value }) : t('bio.failed', { kind: kind.value, error: e?.message ?? '' }))
  } finally {
    busy.value = false
  }
}
function no() {
  try {
    localStorage.setItem(noKey(), '1')
  } catch {
    /* private window */
  }
  show.value = false
}
</script>

<template>
  <Transition name="fade">
    <div v-if="show" class="bio-offer" role="dialog" :aria-label="t('bio.offerTitle', { kind })" data-testid="bio-offer">
      <span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-accent-bg text-accent-text"><Fingerprint class="h-5 w-5" /></span>
      <div class="min-w-0 flex-1">
        <p class="text-[14px] font-semibold">{{ t('bio.offerTitle', { kind }) }}</p>
        <p class="mt-0.5 text-[12.5px] text-text-2">{{ t('bio.offerBody', { kind }) }}</p>
        <div class="mt-3 flex gap-2">
          <button type="button" class="btn-primary btn-sm" :disabled="busy" data-testid="bio-offer-yes" @click="yes">{{ t('bio.use', { kind }) }}</button>
          <button type="button" class="btn btn-sm" :disabled="busy" @click="no">{{ t('bio.offerNo') }}</button>
        </div>
      </div>
      <button type="button" class="icon-btn -mr-1 -mt-1 h-7 w-7" :aria-label="t('bio.offerNo')" @click="no"><X class="h-4 w-4" /></button>
    </div>
  </Transition>
</template>

<style scoped>
.bio-offer {
  position: fixed;
  right: 16px;
  bottom: 16px;
  z-index: 70;
  display: flex;
  width: min(380px, calc(100vw - 32px));
  gap: 12px;
  padding: 14px 16px;
  border-radius: 16px;
  background: var(--surface);
  box-shadow: var(--shadow-lg), 0 0 0 0.5px var(--border);
}
</style>

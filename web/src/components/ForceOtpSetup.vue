<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ShieldAlert, Fingerprint } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { passkeysApi, webauthnSupported } from '@/api/webauthn'
import OtpSetup from '@/components/ui/OtpSetup.vue'

// Non-dismissible overlay shown when two-factor is enforced and the user hasn't
// set it up yet. Mirrors ForcePasswordChange. A passkey also satisfies the
// requirement, so we offer it as an alternative to TOTP.
const auth = useAuthStore()
const toast = useToastStore()
const router = useRouter()

const canPasskey = auth.webauthnEnabled && webauthnSupported()
const pkBusy = ref(false)
const pkErr = ref('')

async function onVerified() {
  await auth.fetchMe() // clears must_setup_otp
  toast.success('Two-factor authentication enabled')
}
async function addPasskey() {
  pkErr.value = ''
  pkBusy.value = true
  try {
    await passkeysApi.register('Passkey')
    await auth.fetchMe() // clears must_setup_otp (now has a passkey)
    toast.success('Passkey added')
  } catch (e: any) {
    pkErr.value = e?.name === 'NotAllowedError' ? 'Registration was cancelled' : e?.message || 'Could not add passkey'
  } finally {
    pkBusy.value = false
  }
}
async function signOut() {
  await auth.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="fixed inset-0 z-[200] flex items-center justify-center bg-[rgba(15,23,42,.45)] p-4">
    <div class="w-full max-w-md rounded-2xl border border-border bg-surface p-6 shadow-soft">
      <div class="mb-4 flex flex-col items-center gap-2 text-center">
        <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-warn-bg">
          <ShieldAlert class="h-6 w-6 text-warn-text" />
        </div>
        <h2 class="text-[15px] font-semibold">Set up two-factor authentication</h2>
        <p class="text-[12px] text-text-2">Your administrator requires 2FA before you can continue.</p>
      </div>
      <OtpSetup @verified="onVerified" />

      <template v-if="canPasskey">
        <div class="my-3 flex items-center gap-3 text-[11px] text-text-3">
          <span class="h-px flex-1 bg-border" /> or <span class="h-px flex-1 bg-border" />
        </div>
        <button
          type="button"
          :disabled="pkBusy"
          class="flex w-full items-center justify-center gap-2 rounded-xl border border-border bg-surface-2 px-3 py-2.5 text-[13px] font-medium transition hover:bg-surface-3 disabled:opacity-50"
          @click="addPasskey"
        >
          <Fingerprint class="h-4 w-4 text-accent" /> Set up a passkey instead
        </button>
        <p v-if="pkErr" class="mt-2 rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ pkErr }}</p>
      </template>

      <button type="button" class="mt-3 w-full text-[12px] text-text-3 hover:text-text" @click="signOut">Sign out instead</button>
    </div>
  </div>
</template>

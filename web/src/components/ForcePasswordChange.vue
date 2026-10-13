<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ShieldAlert } from 'lucide-vue-next'
import { api, setAccessToken } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

const router = useRouter()
async function signOut() {
  await auth.logout()
  router.push({ name: 'login' })
}

// Non-dismissible overlay shown when the account must change its password before
// doing anything else. Backed by the same `must_change_password` flag the API
// enforces server-side.
const auth = useAuthStore()
const toast = useToastStore()

const oldPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const err = ref('')
const busy = ref(false)

async function submit() {
  err.value = ''
  if (newPw.value !== confirmPw.value) {
    err.value = 'New password and confirmation do not match.'
    return
  }
  if (newPw.value.length < 8) {
    err.value = 'New password must be at least 8 characters.'
    return
  }
  busy.value = true
  try {
    // The API signs out every other session and returns a fresh access token
    // for this one (the old token's version is now stale).
    const res = await api<{ access_token?: string }>('/me/password', {
      method: 'PUT',
      body: JSON.stringify({ old_password: oldPw.value, new_password: newPw.value }),
    })
    if (res?.access_token) setAccessToken(res.access_token)
    await auth.fetchMe() // clears must_change_password
    toast.success('Password updated')
  } catch (e: any) {
    err.value = e?.message || 'Could not change password'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="fixed inset-0 z-[200] flex items-center justify-center bg-[rgba(15,23,42,.45)]">
    <div class="w-full max-w-sm rounded-2xl border border-border bg-surface p-6 shadow-soft">
      <div class="mb-4 flex flex-col items-center gap-2 text-center">
        <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-warn-bg">
          <ShieldAlert class="h-6 w-6 text-warn-text" />
        </div>
        <h2 class="text-[15px] font-semibold">Set a new password</h2>
        <p class="text-[12px] text-text-2">You must change your password before continuing.</p>
      </div>
      <form class="flex flex-col gap-3" @submit.prevent="submit">
        <input v-model="oldPw" type="password" autocomplete="current-password" class="input" placeholder="Current password" />
        <input v-model="newPw" type="password" autocomplete="new-password" class="input" placeholder="New password (min 8)" />
        <input v-model="confirmPw" type="password" autocomplete="new-password" class="input" placeholder="Confirm new password" />
        <p v-if="err" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ err }}</p>
        <button type="submit" :disabled="busy" class="btn-primary mt-1">{{ busy ? 'Saving…' : 'Update password' }}</button>
        <button type="button" class="text-[12px] text-text-3 hover:text-text" @click="signOut">Sign out instead</button>
      </form>
    </div>
  </div>
</template>

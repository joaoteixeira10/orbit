<script setup lang="ts">
import { ref, onMounted } from 'vue'
import QRCode from 'qrcode'
import { meApi } from '@/api'
import { saveText } from '@/lib/api'
import { useToastStore } from '@/stores/toast'
import CodeInput from '@/components/ui/CodeInput.vue'

// Self-contained TOTP enrollment: requests a secret on mount, renders the QR,
// verifies a code, then shows one-time recovery codes. Emits `verified` when done.
const emit = defineEmits<{ verified: [] }>()
const toast = useToastStore()

const secret = ref('')
const qr = ref('')
const code = ref('')
const err = ref('')
const loading = ref(true)
const busy = ref(false)
const codes = ref<string[]>([]) // recovery codes shown after verify

onMounted(async () => {
  try {
    const r = await meApi.otpSetup()
    secret.value = r.secret
    qr.value = await QRCode.toDataURL(r.otpauth_url, { margin: 1, width: 200 })
  } catch (e: any) {
    err.value = e?.message || 'Could not start enrollment'
  } finally {
    loading.value = false
  }
})

async function verify() {
  if (busy.value || code.value.length < 6) return
  err.value = ''
  busy.value = true
  try {
    const r = await meApi.otpVerify(code.value.trim())
    codes.value = r.backup_codes
  } catch (e: any) {
    err.value = e?.message || 'Invalid code'
    code.value = ''
  } finally {
    busy.value = false
  }
}
function copyCodes() {
  navigator.clipboard.writeText(codes.value.join('\n')).then(
    () => toast.success('Recovery codes copied'),
    () => toast.error('Could not copy'),
  )
}
function downloadCodes() {
  saveText(codes.value.join('\n') + '\n', 'orbit-recovery-codes.txt', 'text/plain')
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px]">
    <div v-if="loading" class="text-text-3">Generating secret…</div>

    <!-- Recovery codes (after successful verify) -->
    <template v-else-if="codes.length">
      <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
        Save these recovery codes somewhere safe. Each can be used once if you lose your authenticator. They are shown only now.
      </p>
      <div class="grid grid-cols-2 gap-2 rounded-xl border border-border bg-surface-2 p-3 font-mono text-[13px]">
        <span v-for="c in codes" :key="c" class="select-all">{{ c }}</span>
      </div>
      <div class="flex justify-end gap-2">
        <button class="btn-ghost" @click="copyCodes">Copy</button>
        <button class="btn-ghost" @click="downloadCodes">Download</button>
        <button class="btn-primary" @click="emit('verified')">Done</button>
      </div>
    </template>

    <!-- Enrollment -->
    <template v-else>
      <p class="text-text-2">Scan this QR code with your authenticator app (or enter the key manually), then enter the 6-digit code to confirm.</p>
      <div class="flex items-center gap-4">
        <img v-if="qr" :src="qr" alt="TOTP QR" class="h-40 w-40 rounded-lg bg-white p-2" />
        <div class="min-w-0">
          <div class="mb-1 text-[12px] text-text-3">Secret key</div>
          <code class="block break-all rounded bg-surface-2 px-2 py-1 font-mono text-[12px]">{{ secret }}</code>
        </div>
      </div>
      <!-- The code prompt (see docs/AUTH-BLUEPRINT.md, "The two-factor code prompt"). -->
      <form class="flex flex-col gap-3" @submit.prevent="verify">
        <p class="text-center text-text-2">Enter the 6-digit code from your authenticator app.</p>
        <div class="flex justify-center"><CodeInput v-model="code" @complete="verify" /></div>
        <button type="submit" :disabled="busy || code.length < 6" class="btn-primary w-full">{{ busy ? 'Verifying…' : 'Verify & enable' }}</button>
      </form>
      <p v-if="err" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ err }}</p>
    </template>
  </div>
</template>

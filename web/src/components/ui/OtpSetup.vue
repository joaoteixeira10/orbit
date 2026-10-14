<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { meApi } from '@/api'
import { saveText } from '@/lib/api'
import { useToastStore } from '@/stores/toast'
import CodeInput from '@/components/ui/CodeInput.vue'

// Self-contained TOTP enrollment: requests a secret on mount, renders the QR,
// verifies a code, then shows one-time recovery codes. Emits `verified` when done.
const emit = defineEmits<{ verified: [] }>()
const toast = useToastStore()
const { t } = useI18n()

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
    err.value = e?.message || t('acct.otp.cantStart')
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
    err.value = e?.message || t('acct.otp.invalidCode')
    code.value = ''
  } finally {
    busy.value = false
  }
}
function copyCodes() {
  navigator.clipboard.writeText(codes.value.join('\n')).then(
    () => toast.success(t('acct.otp.codesCopied')),
    () => toast.error(t('acct.otp.cantCopy')),
  )
}
function downloadCodes() {
  saveText(codes.value.join('\n') + '\n', 'orbit-recovery-codes.txt', 'text/plain')
}
</script>

<template>
  <div class="flex flex-col gap-4 text-[13px]">
    <div v-if="loading" class="text-text-3">{{ t('acct.otp.generating') }}</div>

    <!-- Recovery codes (after successful verify) -->
    <template v-else-if="codes.length">
      <p class="rounded-lg bg-warn-bg px-3 py-2 text-[12px] text-warn-text">
        {{ t('acct.otp.saveCodes') }}
      </p>
      <div class="grid grid-cols-2 gap-2 rounded-xl border border-border bg-surface-2 p-3 font-mono text-[13px]">
        <span v-for="c in codes" :key="c" class="select-all">{{ c }}</span>
      </div>
      <div class="flex justify-end gap-2">
        <button class="btn-ghost" @click="copyCodes">{{ t('settings.copy') }}</button>
        <button class="btn-ghost" @click="downloadCodes">{{ t('common.download') }}</button>
        <button class="btn-primary" @click="emit('verified')">{{ t('common.done') }}</button>
      </div>
    </template>

    <!-- Enrollment -->
    <template v-else>
      <p class="text-text-2">{{ t('acct.otp.scan') }}</p>
      <div class="flex items-center gap-4">
        <img v-if="qr" :src="qr" :alt="t('acct.otp.qrAlt')" class="h-40 w-40 rounded-lg bg-white p-2" />
        <div class="min-w-0">
          <div class="mb-1 text-[12px] text-text-3">{{ t('acct.otp.secretKey') }}</div>
          <code class="block break-all rounded bg-surface-2 px-2 py-1 font-mono text-[12px]">{{ secret }}</code>
        </div>
      </div>
      <!-- The code prompt (see docs/AUTH-BLUEPRINT.md, "The two-factor code prompt"). -->
      <form class="flex flex-col gap-3" @submit.prevent="verify">
        <p class="text-center text-text-2">{{ t('login.enterCode') }}</p>
        <div class="flex justify-center"><CodeInput v-model="code" @complete="verify" /></div>
        <button type="submit" :disabled="busy || code.length < 6" class="btn-primary w-full">{{ busy ? t('login.verifying') : t('acct.otp.verifyEnable') }}</button>
      </form>
      <p v-if="err" class="rounded-lg bg-crit-bg px-3 py-2 text-[12px] text-crit-text">{{ err }}</p>
    </template>
  </div>
</template>

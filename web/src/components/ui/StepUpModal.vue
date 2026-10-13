<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Overlay from './Overlay.vue'
import { useAuthStore } from '@/stores/auth'
import { stepUpState, resolveStepUp } from '@/lib/stepup'
import { ShieldCheck } from 'lucide-vue-next'

const { t } = useI18n()
const auth = useAuthStore()
const password = ref('')
const busy = ref(false)
const error = ref('')

watch(
  () => stepUpState.open,
  (open) => {
    if (open) {
      password.value = ''
      error.value = ''
      busy.value = false
    }
  },
)

async function run(fn: () => Promise<void>) {
  busy.value = true
  error.value = ''
  try {
    await fn()
    resolveStepUp(true)
  } catch (e: unknown) {
    error.value = (e as Error)?.message || t('stepup.failed')
  } finally {
    busy.value = false
  }
}

const confirmPassword = () => run(() => auth.stepUpPassword(password.value))
const confirmPasskey = () => run(() => auth.stepUpPasskey())
const cancel = () => resolveStepUp(false)
</script>

<template>
  <Overlay :open="stepUpState.open" :title="t('stepup.title')" mode="centered" @close="cancel">
    <div class="flex flex-col gap-3">
      <p class="text-[13px] text-text-2">{{ t('stepup.prompt') }}</p>

      <form v-if="!auth.user?.oidc" class="flex flex-col gap-3" @submit.prevent="confirmPassword">
        <input
          v-model="password"
          type="password"
          class="input"
          :placeholder="t('stepup.password')"
          autocomplete="current-password"
        />
        <p v-if="error" class="text-[12px] text-crit-text">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn-ghost" @click="cancel">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary" :disabled="busy || !password">{{ t('stepup.confirm') }}</button>
        </div>
      </form>

      <template v-else>
        <p v-if="error" class="text-[12px] text-crit-text">{{ error }}</p>
        <div class="flex justify-end">
          <button type="button" class="btn-ghost" @click="cancel">{{ t('common.cancel') }}</button>
        </div>
      </template>

      <button v-if="auth.hasPasskey" type="button" class="btn-ghost" :disabled="busy" @click="confirmPasskey">
        <ShieldCheck class="h-4 w-4" /> {{ t('stepup.usePasskey') }}
      </button>
    </div>
  </Overlay>
</template>

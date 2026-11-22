<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Overlay from './Overlay.vue'
import { useAuthStore } from '@/stores/auth'
import { stepUpState, resolveStepUp } from '@/lib/stepup'
import { ShieldCheck } from '@lucide/vue'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'

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
        <Input
 v-model="password"
 type="password"
 
 :placeholder="t('stepup.password')"
 autocomplete="current-password" />
        <p v-if="error" class="text-[12px] text-crit-text">{{ error }}</p>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" type="button" @click="cancel">{{ t('common.cancel') }}</Button>
          <Button type="submit" :disabled="busy || !password">{{ t('stepup.confirm') }}</Button>
        </div>
      </form>

      <template v-else>
        <p v-if="error" class="text-[12px] text-crit-text">{{ error }}</p>
        <div class="flex justify-end">
          <Button variant="ghost" type="button" @click="cancel">{{ t('common.cancel') }}</Button>
        </div>
      </template>

      <Button variant="ghost" v-if="auth.hasPasskey" type="button" :disabled="busy" @click="confirmPasskey">
        <ShieldCheck class="h-4 w-4" /> {{ t('stepup.usePasskey') }}
      </Button>
    </div>
  </Overlay>
</template>

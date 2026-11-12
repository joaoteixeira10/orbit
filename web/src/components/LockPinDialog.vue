<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LockKeyhole } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import { changePin, lockApi, lockPossible, setUp, unlock, WrongPin, type LockKeyInfo } from '@/lib/lock'
import { answerPin, pinPrompt, type PinMode } from '@/lib/lockPrompt'

// The lock PIN: set the first time (twice, with what forgetting it means),
// entered to open locked notes and pages, or changed (the old one, then the
// new one twice). Asked for "unlock" with none set, it sets one.
const { t } = useI18n()
const mode = ref<PinMode | ''>('')
const info = ref<LockKeyInfo | null>(null)
const pin = ref('')
const pin2 = ref('')
const oldPin = ref('')
const error = ref('')
const busy = ref(false)
const oldEl = ref<HTMLInputElement | null>(null)
const pinEl = ref<HTMLInputElement | null>(null)
const focusFirst = () => (mode.value === 'change' ? oldEl : pinEl).value?.focus()

watch(
  () => pinPrompt.mode,
  async (m) => {
    if (!m) {
      mode.value = ''
      return
    }
    pin.value = pin2.value = oldPin.value = error.value = ''
    info.value = null
    try {
      info.value = await lockApi.get()
    } catch (e: any) {
      error.value = e.message
    }
    mode.value = m === 'unlock' && info.value && !info.value.set ? 'set' : m
    await nextTick()
    focusFirst()
  },
)

const MIN = 4
const title = computed(() => t(`lock.title.${mode.value || 'unlock'}`))
const ready = computed(() => {
  if (busy.value) return false
  if (mode.value === 'unlock') return pin.value.length >= MIN
  if (mode.value === 'change' && oldPin.value.length < MIN) return false
  return pin.value.length >= MIN && pin.value === pin2.value
})

async function submit() {
  if (!ready.value) {
    if (mode.value !== 'unlock' && pin.value.length >= MIN && pin.value !== pin2.value) error.value = t('lock.mismatch')
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (mode.value === 'set') await setUp(pin.value)
    else if (mode.value === 'change') await changePin(oldPin.value, pin.value)
    else await unlock(pin.value, info.value ?? undefined)
    answerPin(true)
  } catch (e: any) {
    error.value = e instanceof WrongPin ? t('lock.wrong') : e.message
    if (e instanceof WrongPin) {
      if (mode.value === 'change') oldPin.value = ''
      else pin.value = ''
      focusFirst()
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Overlay :open="!!mode" :title="title" size="sm" top @close="answerPin(false)">
    <div v-if="!lockPossible()" class="space-y-4" data-testid="lock-insecure">
      <p class="rounded-xl bg-warn-bg px-3 py-2 text-[13px] text-warn-text">{{ t('lock.insecure') }}</p>
      <div class="flex justify-end"><button type="button" class="btn" @click="answerPin(false)">{{ t('common.close') }}</button></div>
    </div>
    <form v-else class="space-y-4" data-testid="lock-pin" @submit.prevent="submit">
      <div class="flex items-start gap-3">
        <span class="tile h-9 w-9 shrink-0" style="--tile: #8e8e93"><LockKeyhole class="h-5 w-5" /></span>
        <p class="text-[13px] leading-snug text-text-2">{{ t(`lock.hint.${mode || 'unlock'}`) }}</p>
      </div>
      <label v-if="mode === 'change'" class="block">
        <span class="mb-1 block text-[12px] font-medium text-text-2">{{ t('lock.oldPin') }}</span>
        <input ref="oldEl" v-model="oldPin" type="password" autocomplete="off" class="input w-full" data-testid="lock-old" />
      </label>
      <label class="block">
        <span class="mb-1 block text-[12px] font-medium text-text-2">{{ mode === 'unlock' ? t('lock.pin') : t('lock.newPin') }}</span>
        <input ref="pinEl" v-model="pin" type="password" autocomplete="off" class="input w-full" data-testid="lock-pin-input" />
      </label>
      <label v-if="mode !== 'unlock'" class="block">
        <span class="mb-1 block text-[12px] font-medium text-text-2">{{ t('lock.again') }}</span>
        <input v-model="pin2" type="password" autocomplete="off" class="input w-full" data-testid="lock-pin-again" />
      </label>
      <p v-if="mode !== 'unlock'" class="rounded-xl bg-warn-bg px-3 py-2 text-[12.5px] text-warn-text">{{ t('lock.warning') }}</p>
      <p v-if="error" class="text-[12.5px] text-crit-text" role="alert">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" @click="answerPin(false)">{{ t('common.cancel') }}</button>
        <button type="submit" class="btn-primary" :disabled="!ready" data-testid="lock-pin-ok">{{ mode === 'unlock' ? t('lock.open') : t('common.save') }}</button>
      </div>
    </form>
  </Overlay>
</template>

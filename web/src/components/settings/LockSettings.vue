<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Confirm from '@/components/ui/Confirm.vue'
import { lockApi, lockNow, lockPossible, unlocked, type LockKeyInfo } from '@/lib/lock'
import { askPin } from '@/lib/lockPrompt'
import { useToastStore } from '@/stores/toast'

// Settings → Locked notes and pages: the lock PIN set, changed, or forgotten
// (and what it locked deleted, as nothing could open it again).
const { t } = useI18n()
const toast = useToastStore()
const info = ref<LockKeyInfo | null>(null)
const forgetting = ref(false)

async function load() {
  try {
    info.value = await lockApi.get()
  } catch (e: any) {
    toast.error(e.message)
  }
}
onMounted(load)

async function set(mode: 'set' | 'change') {
  if (await askPin(mode)) toast.success(t(mode === 'set' ? 'lock.settings.setDone' : 'lock.settings.changed'))
  await load()
}
async function forget() {
  forgetting.value = false
  try {
    await lockApi.forget()
    lockNow()
    toast.info(t('lock.settings.forgot'))
  } catch (e: any) {
    toast.error(e.message)
  }
  await load()
}
</script>

<template>
  <div class="space-y-3" data-testid="lock-settings">
    <p class="text-[13px] text-text-2">{{ t('lock.settings.hint') }}</p>
    <p class="text-[13px]">{{ info?.set ? t('lock.settings.on', { n: info.sealed }) : t('lock.settings.none') }}</p>
    <p v-if="!lockPossible()" class="rounded-xl bg-warn-bg px-3 py-2 text-[12.5px] text-warn-text" data-testid="lock-insecure-note">{{ t('lock.insecure') }}</p>
    <div class="flex flex-wrap gap-2">
      <button v-if="info && !info.set" class="btn btn-sm" data-testid="lock-set" @click="set('set')">{{ t('lock.settings.set') }}</button>
      <template v-else-if="info">
        <button class="btn btn-sm" data-testid="lock-change" @click="set('change')">{{ t('lock.settings.change') }}</button>
        <button v-if="unlocked" class="btn btn-sm" @click="lockNow()">{{ t('lock.lockNow') }}</button>
        <button class="btn btn-sm text-crit-text" data-testid="lock-forget" @click="forgetting = true">{{ t('lock.settings.forget') }}</button>
      </template>
    </div>
    <Confirm :open="forgetting" :title="t('lock.settings.forget')" :message="t('lock.settings.forgetAsk', { n: info?.sealed ?? 0 })" :confirm-label="t('lock.settings.forget')" danger @confirm="forget" @close="forgetting = false" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink, ScrollText } from 'lucide-vue-next'
import { settingsApi } from '@/api'
import { useBrandingStore } from '@/stores/branding'
import { useToastStore } from '@/stores/toast'
import { Button } from '@/components/shadcn/button'
import { Textarea } from '@/components/shadcn/textarea'

// Owner → Branding: the privacy policy and terms of service, public at
// /privacy and /terms and linked from the sign-in page. Empty privacy shows
// Orbit's own text (in each reader's language); empty terms, no page.
const { t } = useI18n()
const branding = useBrandingStore()
const toast = useToastStore()
const privacy = ref('')
const terms = ref('')
const saving = ref(false)
onMounted(async () => {
  try {
    privacy.value = (await settingsApi.legal('privacy')).text
  } catch {
    /* left empty */
  }
  if (branding.hasTerms) {
    try {
      terms.value = (await settingsApi.legal('terms')).text
    } catch {
      /* left empty */
    }
  }
})
function useDefault() {
  privacy.value = t('legal.defaultPrivacy', { app: branding.appName })
}
async function save() {
  if (saving.value) return
  saving.value = true
  try {
    branding.apply(await settingsApi.update({ privacy_policy: privacy.value, terms_of_service: terms.value }))
    toast.success(t('legal.card.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card overflow-hidden" data-testid="legal-admin">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3">
      <ScrollText class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('legal.card.title') }}</h3>
    </div>
    <form class="flex flex-col gap-4 p-5" @submit.prevent="save">
      <p class="text-[12px] text-text-3">{{ t('legal.card.hint') }}</p>
      <div>
        <div class="mb-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="text-[13px] font-semibold">{{ t('legal.card.privacy') }}</span>
          <a href="/privacy" target="_blank" rel="noopener" class="inline-flex items-center gap-1 text-[12px] text-accent hover:underline">{{ t('legal.card.open') }}<ExternalLink class="h-3 w-3" /></a>
          <Button v-if="!privacy.trim()" variant="ghost" size="sm" type="button" class="ml-auto h-7 text-[12px]" data-testid="legal-default" @click="useDefault">{{ t('legal.card.useDefault') }}</Button>
        </div>
        <Textarea v-model="privacy" class="h-40 font-mono text-[12.5px]" :placeholder="t('legal.card.privacyHint')" data-testid="legal-privacy" />
      </div>
      <div>
        <div class="mb-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="text-[13px] font-semibold">{{ t('legal.card.terms') }}</span>
          <a v-if="branding.hasTerms" href="/terms" target="_blank" rel="noopener" class="inline-flex items-center gap-1 text-[12px] text-accent hover:underline">{{ t('legal.card.open') }}<ExternalLink class="h-3 w-3" /></a>
        </div>
        <Textarea v-model="terms" class="h-28 font-mono text-[12.5px]" :placeholder="t('legal.card.termsHint')" data-testid="legal-terms" />
      </div>
      <div><Button size="sm" type="submit" :disabled="saving" data-testid="legal-save">{{ t('common.save') }}</Button></div>
    </form>
  </div>
</template>

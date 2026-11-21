<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { settingsApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'

// /privacy and /terms: the instance's privacy policy and terms of service,
// open to anyone (the sign-in page links them, and Google asks for the
// privacy one). The owner writes them in the Owner tab; until then privacy
// is Orbit's own text, in the reader's language, and there are no terms.
const props = defineProps<{ doc: 'privacy' | 'terms' }>()
const { t } = useI18n()
const branding = useBrandingStore()
const auth = useAuthStore()

const text = ref<string | null>(null)
const appName = ref(branding.appName)
const missing = ref(false)
const title = computed(() => (props.doc === 'privacy' ? t('legal.privacyTitle') : t('legal.termsTitle')))
const content = computed(() => {
  if (text.value === null) return ''
  if (text.value.trim()) return text.value
  return props.doc === 'privacy' ? t('legal.defaultPrivacy', { app: appName.value }) : ''
})
onMounted(async () => {
  document.title = `${title.value} · ${branding.appName}`
  try {
    const r = await settingsApi.legal(props.doc)
    appName.value = r.app_name || appName.value
    text.value = r.text
    document.title = `${title.value} · ${appName.value}`
  } catch {
    missing.value = true
  }
})
</script>

<template>
  <div class="legal">
    <header class="l-top">
      <img :src="branding.logoMode === 'theme' ? branding.crestUrl : branding.logoMode === 'upload' ? branding.logoDataURL : DEFAULT_MARK" alt="" class="h-7 w-7 rounded-lg" />
      <span class="min-w-0 flex-1 truncate text-[13.5px] font-semibold">{{ appName }}</span>
      <RouterLink :to="auth.isAuthenticated ? '/' : '/login'" class="text-[13px] text-accent hover:underline">
        {{ auth.isAuthenticated ? t('legal.back', { app: appName }) : t('legal.signIn') }}
      </RouterLink>
    </header>
    <main class="l-page">
      <h1 class="mb-5 text-[32px] leading-tight font-bold" data-testid="legal-title">{{ title }}</h1>
      <p v-if="missing" class="text-[14px] text-text-3" data-testid="legal-missing">{{ t('legal.missing') }}</p>
      <MarkdownView v-else-if="content" :content="content" :editable="false" data-testid="legal-text" />
    </main>
  </div>
</template>

<style scoped>
.legal {
  min-height: 100dvh;
  background: var(--bg);
  color: var(--text);
}
.l-top {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px clamp(16px, 3vw, 28px);
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--bg) 88%, transparent);
  backdrop-filter: blur(12px);
}
.l-page {
  max-width: 760px;
  margin: 0 auto;
  padding: 28px clamp(16px, 3vw, 28px) 60px;
}
</style>

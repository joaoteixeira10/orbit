<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import FormFill from '@/components/forms/FormFill.vue'
import { guestApi, shareApi, type FillForm } from '@/lib/shareApi'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'

// A form to answer: a public one by its link (/form/<link>, no account), or
// one of a workspace's, in Orbit (/w/<ws>/forms/<id>).
const route = useRoute()
const branding = useBrandingStore()
const { t } = useI18n()

const token = computed(() => (route.params.token ? String(route.params.token) : ''))
const id = computed(() => String(route.params.id ?? ''))
const form = ref<FillForm | null>(null)
const gone = ref('')
onMounted(async () => {
  if (token.value) {
    const m = Object.assign(document.createElement('meta'), { name: 'robots', content: 'noindex, nofollow' })
    document.head.append(m)
  }
  try {
    form.value = token.value ? await guestApi.form(token.value) : await shareApi.fill(id.value)
    document.title = form.value.title
  } catch (e: any) {
    gone.value = e.message
  }
})
const send = (values: Record<string, unknown>) => (token.value ? guestApi.answer(token.value, values) : shareApi.answer(id.value, values))
</script>

<template>
  <div class="form-view" :class="{ public: !!token }">
    <header v-if="token" class="fv-top">
      <img :src="branding.logoMode === 'upload' ? branding.logoDataURL : DEFAULT_MARK" alt="" class="h-7 w-7 rounded-lg" />
    </header>
    <main class="fv-card">
      <p v-if="gone" class="py-16 text-center text-[14px] text-text-3" data-testid="form-gone">{{ gone }}</p>
      <FormFill v-else-if="form" :form="form" :send="send" />
    </main>
    <footer v-if="token" class="fv-foot">{{ t('forms.foot', { app: branding.appName }) }}</footer>
  </div>
</template>

<style scoped>
.form-view {
  padding: 32px clamp(16px, 4vw, 48px) 80px;
}
.form-view.public {
  min-height: 100dvh;
  padding-top: 0;
  background: var(--grouped, var(--bg));
}
.fv-top {
  display: flex;
  justify-content: center;
  padding: 20px 0;
}
.fv-card {
  max-width: 640px;
  margin: 0 auto;
  padding: 28px clamp(18px, 4vw, 36px);
  border-radius: 18px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border), 0 4px 24px rgba(0, 0, 0, 0.05);
}
.fv-foot {
  padding: 20px;
  text-align: center;
  font-size: 12px;
  color: var(--text-3);
}
</style>

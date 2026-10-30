<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Bot } from 'lucide-vue-next'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'

// /oauth/authorize: an app (an assistant, through MCP) asks to use Orbit as
// the person signed in. It says which app and where it goes back to; the
// person allows it — to read and change, or only to read — or not. Allowed,
// the app gets an API token of theirs (Settings → API tokens lists it).
const route = useRoute()
const auth = useAuthStore()
const branding = useBrandingStore()
const { t } = useI18n()

const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')
const app = ref<{ client_name: string; redirect_host: string; redirect_scheme: string } | null>(null)
const error = ref('')
const readOnly = ref(false)
const busy = ref(false)
const me = computed(() => auth.user?.display_name || auth.user?.username || '')

onMounted(async () => {
  if (q('response_type') && q('response_type') !== 'code') {
    error.value = t('oauth.badRequest')
    return
  }
  try {
    app.value = await api(`/oauth/clients/${encodeURIComponent(q('client_id'))}?redirect_uri=${encodeURIComponent(q('redirect_uri'))}`)
  } catch (e: any) {
    error.value = e.message
  }
})

async function answer(deny: boolean) {
  busy.value = true
  try {
    const out = await api<{ redirect: string }>('/oauth/authorize', {
      method: 'POST',
      body: JSON.stringify({
        client_id: q('client_id'),
        redirect_uri: q('redirect_uri'),
        state: q('state'),
        code_challenge: q('code_challenge'),
        code_challenge_method: q('code_challenge_method'),
        read_only: readOnly.value,
        deny,
      }),
    })
    window.location.href = out.redirect
  } catch (e: any) {
    error.value = e.message
    busy.value = false
  }
}
</script>

<template>
  <div class="login" :class="{ themed: !!branding.theme }" :style="branding.theme ? { '--login-ground': branding.theme.ground } : undefined">
    <main class="wrap">
      <div class="brand">
        <img v-if="branding.logoMode === 'theme'" :src="branding.crestUrl" :alt="t('acct.logo')" class="logo" />
        <img v-else-if="branding.logoMode === 'upload'" :src="branding.logoDataURL" :alt="t('acct.logo')" class="brand-logo" />
        <img v-else :src="DEFAULT_MARK" alt="" class="logo" aria-hidden="true" />
      </div>

      <div v-if="error" class="lcard">
        <h1>{{ t('oauth.cant') }}</h1>
        <p class="lead">{{ error }}</p>
      </div>
      <div v-else-if="!app" class="lcard h-40" aria-busy="true" />
      <div v-else class="lcard" data-testid="oauth-consent">
        <div class="mb-3 flex justify-center"><span class="tile h-14 w-14" style="--tile: #5856d6"><Bot class="h-7 w-7" /></span></div>
        <h1>{{ t('oauth.title', { app: app.client_name }) }}</h1>
        <p class="lead">
          {{ t('oauth.lead', { app: app.client_name, name: me }) }}
          <br /><span class="text-[12.5px] text-text-3">{{ t('oauth.back', { host: app.redirect_host || app.redirect_scheme + ':' }) }}</span>
        </p>
        <div class="fields">
          <label class="choice"><input v-model="readOnly" type="radio" :value="false" name="scope" /> <span><b>{{ t('oauth.readWrite') }}</b><br /><small>{{ t('oauth.readWriteHint') }}</small></span></label>
          <label class="choice"><input v-model="readOnly" type="radio" :value="true" name="scope" data-testid="oauth-read-only" /> <span><b>{{ t('oauth.readOnly') }}</b><br /><small>{{ t('oauth.readOnlyHint') }}</small></span></label>
          <button type="button" class="primary" :disabled="busy" data-testid="oauth-allow" @click="answer(false)">{{ t('oauth.allow') }}</button>
          <button type="button" class="alt" :disabled="busy" data-testid="oauth-deny" @click="answer(true)">{{ t('oauth.deny') }}</button>
          <p class="lead center text-[12px]">{{ t('oauth.revoke') }}</p>
        </div>
      </div>

      <div class="foot">{{ t('login.footer', { app: branding.appName }) }}</div>
    </main>
  </div>
</template>

<style scoped src="./signin.css"></style>
<style scoped>
.fields label.choice {
  display: flex;
  flex-direction: row;
  align-items: flex-start;
  gap: 10px;
  color: var(--text);
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--fill-2);
  text-align: left;
  font-size: 13.5px;
  cursor: pointer;
}
.fields .choice input {
  width: auto;
  height: auto;
  margin-top: 3px;
  padding: 0;
  flex: none;
  box-shadow: none;
}
.fields .choice small {
  color: var(--text-3);
  font-size: 12px;
}
</style>

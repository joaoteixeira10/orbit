<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, LogIn, UserPlus } from 'lucide-vue-next'
import WsIcon from '@/components/ui/WsIcon.vue'
import { invitesApi, type InvitePreview } from '@/api/planner'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// /join/<token>: an invite link to a shared workspace. It shows what the
// link is for; signed in, one button joins; otherwise make an account here
// (when the instance lets people), or sign in and come back.
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const branding = useBrandingStore()
const wss = useWorkspacesStore()
const toast = useToastStore()
const { t } = useI18n()

const token = computed(() => String(route.params.token ?? ''))
const preview = ref<InvitePreview | null>(null)
const gone = ref(false)
const step = ref<'choose' | 'signup'>('choose')
const busy = ref(false)
const error = ref('')

const username = ref('')
const displayName = ref('')
const email = ref('')
const password = ref('')

const me = computed(() => auth.user?.display_name || auth.user?.username || '')

onMounted(async () => {
  try {
    preview.value = await invitesApi.preview(token.value)
  } catch {
    gone.value = true
  }
})

async function enter(wsId: string, already: boolean) {
  await wss.load().catch(() => {})
  toast.success(t(already ? 'invites.already' : 'invites.joined', { ws: preview.value?.workspace ?? '' }))
  router.replace(`/w/${wsId}`)
}

async function join() {
  busy.value = true
  error.value = ''
  try {
    const r = await invitesApi.accept(token.value)
    await enter(r.workspace_id, r.already_member)
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  } finally {
    busy.value = false
  }
}

async function signup() {
  busy.value = true
  error.value = ''
  try {
    const r = await invitesApi.signup(token.value, {
      username: username.value.trim().toLowerCase(),
      display_name: displayName.value.trim(),
      email: email.value.trim(),
      password: password.value,
    })
    await auth.adopt(r.access_token)
    await enter(r.workspace_id, false)
  } catch (e: any) {
    error.value = e?.message ?? String(e)
  } finally {
    busy.value = false
  }
}

function signIn() {
  router.push({ name: 'login', query: { redirect: route.fullPath } })
}
async function signOut() {
  await auth.logout()
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

      <!-- Expired, revoked, used up, or not a link at all -->
      <div v-if="gone" class="lcard">
        <h1>{{ t('invites.gone') }}</h1>
        <p class="lead">{{ t('invites.goneHint') }}</p>
        <div class="fields">
          <button type="button" class="alt" @click="router.replace('/')">{{ t('invites.home') }}</button>
        </div>
      </div>

      <div v-else-if="!preview" class="lcard h-40" aria-busy="true" />

      <!-- What the link is for, and how to join -->
      <div v-else-if="step === 'choose'" class="lcard">
        <div class="mb-3 flex justify-center"><WsIcon :icon="preview.icon" :color="preview.color" :size="56" :name="preview.workspace" /></div>
        <h1>{{ t('invites.joinTitle', { ws: preview.workspace }) }}</h1>
        <p class="lead">
          {{ t('invites.invitedBy', { name: preview.invited_by, role: t(`roles.${preview.role}`).toLowerCase() }) }}
          <br /><span class="text-[12.5px] text-text-3">{{ t('invites.members', { n: preview.member_count }, preview.member_count) }}</span>
        </p>
        <div class="fields">
          <p v-if="error" class="error" role="alert">{{ error }}</p>
          <template v-if="auth.isAuthenticated">
            <button type="button" class="primary" :disabled="busy" data-testid="join" @click="join">{{ busy ? t('invites.joining') : t('invites.join') }}</button>
            <p class="lead center text-[13px]">
              {{ t('invites.signedInAs', { name: me }) }} ·
              <button type="button" class="link !inline" @click="signOut">{{ t('invites.notYou') }}</button>
            </p>
          </template>
          <template v-else>
            <button v-if="preview.can_sign_up" type="button" class="primary" @click="step = 'signup'"><UserPlus :size="16" /> {{ t('invites.newAccount') }}</button>
            <p v-else class="lead center">{{ t('invites.ssoOnly') }}</p>
            <button type="button" class="alt" @click="signIn"><LogIn :size="16" /> {{ t('invites.haveAccount') }}</button>
          </template>
        </div>
      </div>

      <!-- A new account through the link -->
      <form v-else class="lcard" @submit.prevent="signup">
        <h1>{{ t('invites.newAccount') }}</h1>
        <p class="lead">{{ t('invites.signUpLead') }}</p>
        <div class="fields">
          <p v-if="error" class="error" role="alert">{{ error }}</p>
          <label>
            {{ t('invites.displayName') }}
            <input v-model="displayName" type="text" autocomplete="name" maxlength="80" />
          </label>
          <label>
            {{ t('invites.username') }}
            <input v-model="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" required pattern="[a-zA-Z0-9][a-zA-Z0-9._\-]{2,31}" :title="t('invites.usernameHint')" />
          </label>
          <label>
            {{ t('invites.email') }}
            <input v-model="email" type="email" autocomplete="email" maxlength="255" />
          </label>
          <label>
            {{ t('invites.password') }}
            <input v-model="password" type="password" autocomplete="new-password" required minlength="8" :placeholder="t('invites.passwordHint')" />
          </label>
          <button type="submit" class="primary" :disabled="busy">{{ busy ? t('invites.creating') : t('invites.create') }}</button>
          <div class="links">
            <button type="button" class="link" @click="(step = 'choose'), (error = '')"><ArrowLeft :size="14" /> {{ t('invites.back') }}</button>
          </div>
        </div>
      </form>

      <div class="foot">{{ t('login.footer', { app: branding.appName }) }}</div>
    </main>
  </div>
</template>

<style scoped src="./signin.css"></style>

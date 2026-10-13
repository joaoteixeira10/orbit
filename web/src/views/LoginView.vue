<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { KeyRound, ArrowLeft, Fingerprint, LogIn } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { webauthnSupported, webauthnAutofillSupported, cancelWebauthnCeremony } from '@/api/webauthn'
import CodeInput from '@/components/ui/CodeInput.vue'

const auth = useAuthStore()
const branding = useBrandingStore()
const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const username = ref('')
const password = ref('')
const otp = ref('')
const recovery = ref('')
const step = ref<'credentials' | 'otp'>('credentials')
const useRecovery = ref(false)
const error = ref('')
const loading = ref(false)

// Second-factor options surfaced after a password login.
const offerOtp = ref(false)
const passkeyChallenge = ref<{ sessionId: string; options: any } | null>(null)

// Local password form: hidden when the admin enabled "hide password login" and
// SSO is available — always reachable at /login?local=1 as an escape hatch.
const showLocal = computed(() => !(branding.hideLocalLogin && branding.oidcEnabled) || route.query.local === '1')
// ?local=1 also disables any auto-login so you can always reach the manual form.
const autoLoginAllowed = computed(() => route.query.local !== '1')
// Passwordless passkey button: needs browser support + the admin toggle.
const showPasskey = computed(() => webauthnSupported() && branding.passwordlessEnabled)

const ssoErrors: Record<string, string> = {
  disabled: 'SSO is not enabled.',
  unavailable: 'The identity provider is unavailable.',
  idp_error: 'The identity provider returned an error.',
  auth_failed: 'SSO sign-in failed.',
  provisioning: 'Your account could not be provisioned (it may be disabled).',
  token: 'Could not complete sign-in.',
}

function done() {
  // Only honour an internal, single-leading-slash path (no //host or scheme:),
  // so a crafted ?redirect= can't bounce the user off-site after login.
  const r = route.query.redirect
  const safe = typeof r === 'string' && r.startsWith('/') && !r.startsWith('//') ? r : '/'
  router.push(safe)
}

// Start SSO: full-page redirect to the backend, which redirects to the IdP.
function signInWithSSO() {
  window.location.href = '/api/v1/auth/oidc/login'
}

// Conditional-UI passkey autofill: armed on mount, cancelled on submit/unmount.
const autofillArmed = ref(false)
async function armPasskeyAutofill() {
  if (!showLocal.value || !branding.passwordlessEnabled) return
  if (!(await webauthnAutofillSupported())) return
  autofillArmed.value = true
  try {
    await auth.loginWithPasskeyAutofill()
    done()
  } catch {
    // Aborted (user took another path) or cancelled — silent; buttons remain.
  } finally {
    autofillArmed.value = false
  }
}
function cancelAutofill() {
  if (autofillArmed.value) cancelWebauthnCeremony()
}
onBeforeUnmount(cancelAutofill)

// Handle the SSO redirect back to the SPA: ?sso=<one-time code> or ?sso_error=.
onMounted(async () => {
  const code = route.query.sso as string | undefined
  const ssoErr = route.query.sso_error as string | undefined
  if (ssoErr) {
    error.value = ssoErrors[ssoErr] || 'SSO sign-in failed.'
    router.replace({ query: {} })
    return
  }
  if (code) {
    loading.value = true
    try {
      await auth.exchangeSSO(code)
      done()
    } catch (e: any) {
      error.value = e?.message || 'SSO sign-in failed.'
      router.replace({ query: {} })
    } finally {
      loading.value = false
    }
    return
  }
  // Auto-login (unless ?local=1): start SSO (a full-page redirect). On failure
  // the user is bounced back with ?sso_error, so this doesn't re-fire.
  if (autoLoginAllowed.value && branding.autoSso && branding.oidcEnabled) {
    signInWithSSO()
    return
  }
  armPasskeyAutofill()
})

async function attempt(code?: string) {
  error.value = ''
  loading.value = true
  try {
    const res = await auth.login(username.value, password.value, code)
    if (res.status === '2fa') {
      step.value = 'otp'
      offerOtp.value = res.otp
      passkeyChallenge.value = res.webauthn ?? null
      useRecovery.value = false
      return
    }
    done()
  } catch (e: any) {
    error.value = e?.message || 'Login failed'
    otp.value = ''
  } finally {
    loading.value = false
  }
}

function submitCredentials() {
  cancelAutofill()
  attempt()
}
function submitOtp() {
  attempt(useRecovery.value ? recovery.value.trim() : otp.value)
}
function onComplete() {
  if (!loading.value) submitOtp()
}

// Passwordless sign-in from step 1.
async function signInWithPasskey() {
  cancelAutofill()
  error.value = ''
  loading.value = true
  try {
    await auth.loginWithPasskey()
    done()
  } catch (e: any) {
    error.value = e?.name === 'NotAllowedError' ? 'Passkey sign-in was cancelled' : e?.message || 'Passkey sign-in failed'
  } finally {
    loading.value = false
  }
}

// Passkey as the second factor after a password login.
async function usedSecondFactorPasskey() {
  if (!passkeyChallenge.value) return
  error.value = ''
  loading.value = true
  try {
    await auth.completeWithPasskey(passkeyChallenge.value.sessionId, passkeyChallenge.value.options)
    done()
  } catch (e: any) {
    error.value = e?.name === 'NotAllowedError' ? 'Passkey was cancelled' : e?.message || 'Passkey authentication failed'
  } finally {
    loading.value = false
  }
}

function backToCredentials() {
  step.value = 'credentials'
  otp.value = ''
  recovery.value = ''
  error.value = ''
  useRecovery.value = false
  offerOtp.value = false
  passkeyChallenge.value = null
}
</script>

<template>
  <!-- Orbit v0's sign-in card; the code step keeps PKI's six-box input. -->
  <div class="login">
    <main class="wrap">
      <div class="brand">
        <template v-if="branding.showMark">
          <img v-if="branding.logoMode === 'theme'" :src="branding.crestUrl" alt="logo" class="logo" />
          <img v-else-if="branding.logoMode === 'upload'" :src="branding.logoDataURL" alt="logo" class="brand-logo" />
          <img v-else :src="DEFAULT_MARK" alt="" class="logo" aria-hidden="true" />
        </template>
        <template v-if="branding.showName">
          <b v-if="branding.wordmarkIsTheme && branding.theme" class="brand-word blackletter" :style="{ color: branding.theme.mark, '-webkit-text-stroke': `1.8px ${branding.theme.halo}` }">UBonito</b>
          <b v-else>{{ branding.appName }}</b>
        </template>
      </div>

      <!-- Step 1: credentials -->
      <form v-if="step === 'credentials'" class="lcard" @submit.prevent="submitCredentials">
        <h1>{{ t('login.signIn') }}</h1>
        <p class="lead">{{ branding.loginMessage || t('login.useAccount', { app: branding.appName }) }}</p>
        <div class="fields">
          <p v-if="error" class="error" role="alert">{{ error }}</p>
          <template v-if="showLocal">
            <label>
              {{ t('login.username') }}
              <input v-model="username" type="text" autocomplete="username webauthn" autocapitalize="none" spellcheck="false" />
            </label>
            <label>
              {{ t('login.password') }}
              <input v-model="password" type="password" autocomplete="current-password" />
            </label>
            <button type="submit" :disabled="loading" class="primary">{{ loading ? t('login.signingIn') : t('login.signIn') }}</button>
          </template>

          <template v-if="showPasskey || branding.oidcEnabled">
            <div v-if="showLocal" class="or"><span />{{ t('login.or') }}<span /></div>
            <button v-if="showPasskey" type="button" :disabled="loading" class="alt" @click="signInWithPasskey">
              <Fingerprint :size="16" /> {{ t('login.withPasskey') }}
            </button>
            <button v-if="branding.oidcEnabled" type="button" :disabled="loading" class="alt" @click="signInWithSSO">
              <LogIn :size="16" /> {{ t('login.withSSO') }}
            </button>
          </template>
        </div>
      </form>

      <!-- Step 2: second factor (PKI's code prompt) -->
      <form v-else class="lcard" @submit.prevent="submitOtp">
        <h1>{{ t('login.twoFactor') }}</h1>
        <div class="fields">
          <template v-if="offerOtp && !useRecovery">
            <p class="lead center">{{ t('login.enterCode') }}</p>
            <div class="flex justify-center">
              <CodeInput v-model="otp" @complete="onComplete" />
            </div>
          </template>
          <template v-else-if="offerOtp">
            <p class="lead center">{{ t('login.enterRecovery') }}</p>
            <input v-model="recovery" class="code" placeholder="xxxxx-xxxxx" autofocus />
          </template>

          <p v-if="error" class="error" role="alert">{{ error }}</p>

          <button v-if="offerOtp" type="submit" :disabled="loading" class="primary">
            {{ loading ? t('login.verifying') : t('login.verify') }}
          </button>

          <template v-if="passkeyChallenge">
            <div v-if="offerOtp" class="or"><span />{{ t('login.or') }}<span /></div>
            <button type="button" :disabled="loading" class="alt" @click="usedSecondFactorPasskey">
              <Fingerprint :size="16" /> {{ t('login.usePasskey') }}
            </button>
          </template>

          <div class="links">
            <button type="button" class="link" @click="backToCredentials">
              <ArrowLeft :size="14" /> {{ t('login.back') }}
            </button>
            <button v-if="offerOtp" type="button" class="link accent" @click="useRecovery = !useRecovery">
              <KeyRound :size="14" /> {{ useRecovery ? t('login.useAuthenticator') : t('login.useRecovery') }}
            </button>
          </div>
        </div>
      </form>

      <div class="foot">{{ t('login.footer', { app: branding.appName }) }}</div>
    </main>
  </div>
</template>

<style scoped>
/* v0's sign-in page (tag v0-python: idp/pages.py). */
.login {
  min-height: 100%;
  display: grid;
  place-items: center;
  padding: 24px 16px;
  font-size: 14px;
  line-height: 1.5;
}
.wrap {
  width: min(400px, 100%);
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: center;
  margin-bottom: 18px;
}
.brand b {
  font-size: 16px;
}
.logo {
  width: 34px;
  height: 34px;
  flex: none;
}
.brand-word {
  font-size: 24px;
  line-height: 1;
}
.brand-logo {
  height: 34px;
  width: auto;
  max-width: 200px;
  object-fit: contain;
  border-radius: 6px;
}
.lcard {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 26px 28px;
  box-shadow: 0 8px 28px rgba(15, 23, 42, 0.1);
}
h1 {
  margin: 0 0 6px;
  font-size: 19px;
  font-weight: 700;
  letter-spacing: -0.01em;
}
.lead {
  margin: 0 0 16px;
  color: var(--text-2);
}
.lead.center {
  margin: 0;
  text-align: center;
}
.fields {
  display: grid;
  gap: 14px;
}
label {
  display: grid;
  gap: 6px;
  font-weight: 550;
  font-size: 13px;
}
input {
  font: inherit;
  font-weight: 400;
  padding: 9px 11px;
  border-radius: 8px;
  border: 1px solid var(--border-strong);
  background: var(--surface-2);
  color: var(--text);
  width: 100%;
}
input:focus {
  outline: 2px solid var(--accent);
  outline-offset: 1px;
  border-color: var(--accent);
}
input.code {
  font: 600 18px var(--font-mono);
  letter-spacing: 0.1em;
  text-align: center;
}
.primary,
.alt {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font: inherit;
  font-weight: 600;
  cursor: pointer;
  padding: 10px 14px;
  border-radius: 8px;
}
.primary {
  border: 1px solid var(--accent);
  background: var(--accent);
  color: var(--accent-fg);
  margin-top: 4px;
}
.alt {
  border: 1px solid var(--border-strong);
  background: var(--surface);
  color: var(--text);
}
.primary:hover:not(:disabled) {
  filter: brightness(1.08);
}
.alt:hover:not(:disabled) {
  background: var(--surface-hover);
}
.primary:disabled,
.alt:disabled {
  opacity: 0.5;
  cursor: default;
}
.alt svg {
  color: var(--accent);
}
.or {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 11px;
  color: var(--text-3);
}
.or span {
  height: 1px;
  flex: 1;
  background: var(--border);
}
.error {
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--crit-bg);
  color: var(--crit-text);
  border: 1px solid var(--crit-border);
  font-weight: 550;
  font-size: 13px;
}
.links {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12.5px;
}
.link {
  display: flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: none;
  padding: 0;
  color: var(--text-3);
  cursor: pointer;
}
.link:hover {
  color: var(--text);
}
.link.accent {
  color: var(--accent);
}
.link.accent:hover {
  text-decoration: underline;
}
.foot {
  text-align: center;
  margin-top: 16px;
  color: var(--text-3);
  font-size: 12px;
}
</style>

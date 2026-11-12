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
  disabled: t('acct.signin.ssoDisabled'),
  unavailable: t('acct.signin.ssoUnavailable'),
  idp_error: t('acct.signin.ssoIdpError'),
  auth_failed: t('acct.signin.ssoFailed'),
  provisioning: t('acct.signin.ssoProvisioning'),
  token: t('acct.signin.ssoToken'),
  domain: t('acct.signin.ssoDomain'),
}

// SSO goes off to the identity provider and comes back to /login?sso=…, so
// where to go afterwards (?redirect=, an invite link, say) waits here.
const AFTER_SSO = 'orbit_after_sso'

function done() {
  // Only honour an internal, single-leading-slash path (no //host or scheme:),
  // so a crafted ?redirect= can't bounce the user off-site after login.
  let r: unknown = route.query.redirect
  try {
    r ??= sessionStorage.getItem(AFTER_SSO) ?? undefined
    sessionStorage.removeItem(AFTER_SSO)
  } catch {
    /* no storage */
  }
  const safe = typeof r === 'string' && r.startsWith('/') && !r.startsWith('//') ? r : '/'
  router.push(safe)
}

// Start SSO: full-page redirect to the backend, which redirects to the IdP.
function signInWithSSO(provider = '') {
  try {
    if (typeof route.query.redirect === 'string') sessionStorage.setItem(AFTER_SSO, route.query.redirect)
  } catch {
    /* no storage */
  }
  window.location.href = '/api/v1/auth/oidc/login' + (provider ? `?provider=${encodeURIComponent(provider)}` : '')
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
    error.value = ssoErrors[ssoErr] || t('acct.signin.ssoFailed')
    router.replace({ query: {} })
    return
  }
  if (code) {
    loading.value = true
    try {
      await auth.exchangeSSO(code)
      done()
    } catch (e: any) {
      error.value = e?.message || t('acct.signin.ssoFailed')
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
    error.value = e?.message || t('acct.signin.failed')
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
    error.value = e?.name === 'NotAllowedError' ? t('acct.signin.passkeyCancelled') : e?.message || t('acct.signin.passkeyFailed')
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
    error.value = e?.name === 'NotAllowedError' ? t('acct.signin.secondPasskeyCancelled') : e?.message || t('acct.signin.secondPasskeyFailed')
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
  <!-- The sign-in card; the code step keeps PKI's six-box input. -->
  <!-- With a developer theme, the page stands on the theme's dark ground. -->
  <div class="login" :class="{ themed: !!branding.theme }" :style="branding.theme ? { '--login-ground': branding.theme.ground } : undefined">
    <main class="wrap">
      <div class="brand">
        <template v-if="branding.showMark">
          <img v-if="branding.logoMode === 'theme'" :src="branding.crestUrl" :alt="t('acct.logo')" class="logo" />
          <img v-else-if="branding.logoMode === 'upload'" :src="branding.logoDataURL" :alt="t('acct.logo')" class="brand-logo" />
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
            <!-- One button per identity provider ("Sign in with Google"…). -->
            <template v-if="branding.ssoProviders.length > 1 || branding.ssoProviders[0]?.name !== 'SSO'">
              <button v-for="p in branding.ssoProviders" :key="p.id" type="button" :disabled="loading" class="alt" :data-testid="`sso-${p.id}`" @click="signInWithSSO(p.id)">
                <LogIn :size="16" /> {{ t('login.withProvider', { name: p.name }) }}
              </button>
            </template>
            <button v-else-if="branding.oidcEnabled" type="button" :disabled="loading" class="alt" @click="signInWithSSO()">
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

<!-- Shared with the join page (JoinView). -->
<style scoped src="./signin.css"></style>

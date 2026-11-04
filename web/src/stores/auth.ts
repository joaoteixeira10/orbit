import { defineStore } from 'pinia'
import {
  api,
  setAccessToken,
  getAccessToken,
  tryRefresh,
  onSessionExpired,
  beginImpersonation,
  endImpersonation,
  onImpersonationEnded,
} from '@/lib/api'
import { startAuthentication } from '@simplewebauthn/browser'
import { usePrefsStore } from '@/stores/prefs'
import { passkeysApi } from '@/api/webauthn'
import type { User, Privileges } from '@/api/types'

interface MeResponse {
  user: User
  privileges: Privileges
  must_setup_otp: boolean
  otp_backup_remaining: number
  has_passkey: boolean
  webauthn_enabled: boolean
  is_owner: boolean
  features: Record<string, boolean>
  can_create_workspaces: boolean
  impersonator?: { id: string; username: string }
}

type WebAuthnChallenge = { sessionId: string; options: any }

interface TokenResponse {
  access_token?: string
  refresh_token?: string
  expires_in?: number
  user?: User
  otp_required?: boolean
  webauthn_required?: boolean
  webauthn?: { session_id: string; options: any }
}

// Result of a password login: either fully authenticated, or a second factor
// is required (TOTP and/or a passkey challenge to offer the user).
export type LoginResult = { status: 'ok' } | { status: '2fa'; otp: boolean; webauthn?: WebAuthnChallenge }

const emptyPrivs: Privileges = { superuser: false, global: [] }

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as User | null,
    privileges: emptyPrivs as Privileges,
    hasToken: !!getAccessToken(),
    mustSetupOtp: false,
    otpBackupRemaining: 0,
    hasPasskey: false,
    webauthnEnabled: false,
    isOwner: false,
    features: {} as Record<string, boolean>,
    canCreateWorkspaces: false,
    // The real admin when the current session is an impersonation, else null.
    impersonator: null as { id: string; username: string } | null,
  }),
  getters: {
    isAuthenticated: (s) => s.hasToken,
    isSuperuser: (s) => s.privileges.superuser,
    // Admin-ish: can reach any admin-area capability (owner always can). The
    // audit log has its own page (Security → Audit), so audit.read isn't one.
    isAdminArea: (s) =>
      s.isOwner ||
      s.privileges.superuser ||
      ['users.manage', 'roles.manage'].some((p) => s.privileges.global.includes(p)) ||
      ['settings.branding', 'settings.notifications'].some((p) => s.privileges.global.includes(p)),
  },
  actions: {
    // Global privilege check.
    can(priv: string): boolean {
      return this.privileges.superuser || this.privileges.global.includes(priv)
    },
    // Owner-gated privilege check: the superuser flag does NOT grant these
    // (branding/notifications/impersonate) — only the owner, or a role that
    // explicitly lists the privilege.
    canOwnerGated(priv: string): boolean {
      return this.isOwner || this.privileges.global.includes(priv)
    },

    // Password login: returns {status:'ok'} when authenticated, or {status:'2fa'}
    // listing the available second factors (TOTP and/or a passkey challenge).
    async login(username: string, password: string, otp?: string): Promise<LoginResult> {
      const res = await api<TokenResponse>('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password, otp }),
      })
      if (res.access_token) {
        await this.adopt(res.access_token)
        return { status: 'ok' }
      }
      return {
        status: '2fa',
        otp: !!res.otp_required,
        webauthn:
          res.webauthn_required && res.webauthn
            ? { sessionId: res.webauthn.session_id, options: res.webauthn.options }
            : undefined,
      }
    },

    // loginWithPasskey performs a passwordless (discoverable) passkey sign-in.
    async loginWithPasskey(): Promise<void> {
      const res = await passkeysApi.loginDiscoverable()
      if (!res.access_token) throw new Error('Passkey sign-in failed')
      await this.adopt(res.access_token)
    },

    // loginWithPasskeyAutofill arms conditional UI (passkey autofill on the
    // username field); resolves once the user picks a credential.
    async loginWithPasskeyAutofill(): Promise<void> {
      const res = await passkeysApi.loginAutofill()
      if (!res.access_token) throw new Error('Passkey sign-in failed')
      await this.adopt(res.access_token)
    },

    // completeWithPasskey finishes the second-factor passkey challenge issued
    // during a password login.
    async completeWithPasskey(sessionId: string, options: unknown): Promise<void> {
      const res = await passkeysApi.finish2fa(sessionId, options)
      if (!res.access_token) throw new Error('Passkey authentication failed')
      await this.adopt(res.access_token)
    },

    // adopt takes the in-memory access token after a successful auth; the
    // refresh token is set server-side as the HttpOnly cookie (not handled here).
    async adopt(access: string): Promise<void> {
      setAccessToken(access)
      this.hasToken = true
      await this.fetchMe()
    },

    // exchangeSSO swaps a one-time OIDC code (from the SSO redirect) for tokens.
    async exchangeSSO(code: string): Promise<void> {
      const { oidcApi } = await import('@/api')
      const res = await oidcApi.exchange(code)
      if (!res.access_token) throw new Error('SSO sign-in failed')
      await this.adopt(res.access_token)
    },

    async fetchMe(): Promise<boolean> {
      if (!getAccessToken()) return false
      try {
        const me = await api<MeResponse>('/auth/me')
        this.user = me.user
        this.privileges = me.privileges || emptyPrivs
        this.mustSetupOtp = me.must_setup_otp
        this.otpBackupRemaining = me.otp_backup_remaining
        this.hasPasskey = me.has_passkey
        this.webauthnEnabled = me.webauthn_enabled
        this.isOwner = me.is_owner ?? false
        this.features = me.features ?? {}
        this.canCreateWorkspaces = me.can_create_workspaces ?? false
        this.impersonator = me.impersonator ?? null
        this.hasToken = true
        if (me.user.prefs) usePrefsStore().hydrateFromServer(me.user.prefs)
        // Open the live-updates stream so role changes apply without a reload.
        import('@/lib/live').then((m) => m.startLiveUpdates())
        return true
      } catch {
        this.clear()
        return false
      }
    },

    // Session restore on app start. There's no in-memory access token on a fresh
    // load, so silently mint one from the HttpOnly refresh cookie, then load /me.
    async restore(): Promise<boolean> {
      if (!getAccessToken() && !(await tryRefresh())) return false
      return this.fetchMe()
    },

    async logout() {
      try {
        // The refresh cookie + CSRF header are sent automatically by api().
        await api('/auth/logout', { method: 'POST' })
      } catch {
        /* best-effort */
      }
      this.clear()
    },

    clear() {
      setAccessToken(null)
      this.user = null
      this.privileges = emptyPrivs
      this.hasToken = false
      this.mustSetupOtp = false
      this.hasPasskey = false
      this.isOwner = false
      this.features = {}
      this.canCreateWorkspaces = false
      this.impersonator = null
      import('@/lib/live').then((m) => m.stopLiveUpdates())
      // Signed out: the instance's developer theme again (the sign-in page's).
      localStorage.removeItem('orbit_dev_theme')
      import('@/stores/branding').then((m) => {
        const b = m.useBrandingStore()
        b.userDevTheme = ''
        b.applyAccent()
        b.applyFavicon()
      })
    },

    // startImpersonation swaps the current (admin) session for a short-lived
    // session as the target user, stashing the admin tokens to return to.
    async startImpersonation(userId: string): Promise<void> {
      const { usersApi } = await import('@/api')
      const res = await usersApi.impersonate(userId)
      if (!res.access_token) throw new Error('Impersonation failed')
      beginImpersonation(res.access_token)
      await this.fetchMe()
      void import('@/lib/live').then((m) => m.restartLiveUpdates())
    },

    // stopImpersonation restores the stashed admin session.
    async stopImpersonation(): Promise<void> {
      const whom = this.user?.id
      if (!endImpersonation()) {
        this.clear()
        return
      }
      this.impersonator = null
      if (whom) await recordImpersonationEnd(whom, 'returned')
      await this.fetchMe()
      void import('@/lib/live').then((m) => m.restartLiveUpdates())
    },

    // Step-up: re-authenticate to mint a fresh token carrying the step-up
    // assertion; the gated request is retried by the api() interceptor.
    async stepUpPassword(password: string): Promise<void> {
      const res = await api<{ access_token: string }>('/auth/step-up', {
        method: 'POST',
        body: JSON.stringify({ password }),
      })
      setAccessToken(res.access_token)
    },

    async stepUpPasskey(): Promise<void> {
      const begin = await api<{ session_id: string; options: unknown }>('/auth/step-up/webauthn/begin', {
        method: 'POST',
      })
      const asr = await startAuthentication({ optionsJSON: begin.options as never })
      const res = await api<{ access_token: string }>(
        `/auth/step-up/webauthn/finish?sid=${encodeURIComponent(begin.session_id)}`,
        { method: 'POST', body: JSON.stringify(asr) },
      )
      setAccessToken(res.access_token)
    },
  },
})

// recordImpersonationEnd tells the audit log an impersonation ended (as the
// admin again); a failure must not keep anyone from getting back.
async function recordImpersonationEnd(id: string, reason: 'returned' | 'expired') {
  try {
    const { usersApi } = await import('@/api')
    await usersApi.endImpersonation(id, reason)
  } catch {
    /* the start and every change are recorded anyway */
  }
}

// An impersonation token that expired mid-session: we've already fallen back to
// the admin's stashed tokens, so just re-sync the store.
onImpersonationEnded.add(async () => {
  const s = useAuthStore()
  const whom = s.user?.id
  s.impersonator = null
  if (whom) await recordImpersonationEnd(whom, 'expired')
  await s.fetchMe()
  void import('@/lib/live').then((m) => m.restartLiveUpdates())
})

// Session expiry (refresh failed / account disabled) → clear + bounce to login.
onSessionExpired.add(async () => {
  useAuthStore().clear()
  const { router } = await import('@/router')
  if (router.currentRoute.value.name !== 'login') {
    router.push({ name: 'login', query: { reason: 'session-expired' } })
  }
})

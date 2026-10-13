// Passkey (WebAuthn) ceremonies. @simplewebauthn/browser handles the base64url
// encoding/decoding of the options and the navigator.credentials calls; the
// server returns { session_id, options } from each *begin, and each *finish
// posts the raw ceremony response with sid as a query param.
import {
  startRegistration,
  startAuthentication,
  browserSupportsWebAuthn,
  browserSupportsWebAuthnAutofill,
  WebAuthnAbortService,
} from '@simplewebauthn/browser'
import { api } from '@/lib/api'
import type { WebAuthnCredential } from './types'

export const webauthnSupported = (): boolean => browserSupportsWebAuthn()
// Conditional-UI (passkey autofill on the username field) is a stricter check.
export const webauthnAutofillSupported = (): Promise<boolean> => browserSupportsWebAuthnAutofill()
// cancelWebauthnCeremony aborts any pending ceremony (e.g. an armed autofill
// request) so a competing sign-in path can proceed.
export const cancelWebauthnCeremony = (): void => WebAuthnAbortService.cancelCeremony()

// Token shape returned by the passkey login finish (same as /auth/login).
export interface PasskeyTokenResponse {
  access_token?: string
  refresh_token?: string
  expires_in?: number
}

interface BeginResponse {
  session_id: string
  options: any
}

const qs = (o: Record<string, string>) =>
  Object.entries(o)
    .map(([k, v]) => `${k}=${encodeURIComponent(v)}`)
    .join('&')

export const passkeysApi = {
  list: () => api<WebAuthnCredential[]>('/me/webauthn'),
  rename: (id: string, name: string) =>
    api<void>(`/me/webauthn/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }),
  remove: (id: string) => api<void>(`/me/webauthn/${id}`, { method: 'DELETE' }),

  // Enroll a new passkey for the signed-in user.
  async register(name: string): Promise<WebAuthnCredential> {
    const begin = await api<BeginResponse>('/me/webauthn/register/begin', { method: 'POST' })
    const att = await startRegistration({ optionsJSON: begin.options })
    return api<WebAuthnCredential>(`/me/webauthn/register/finish?${qs({ sid: begin.session_id, name })}`, {
      method: 'POST',
      body: JSON.stringify(att),
    })
  },

  // Passwordless login: discover a resident credential and assert it.
  async loginDiscoverable(): Promise<PasskeyTokenResponse> {
    const begin = await api<BeginResponse>('/auth/webauthn/login/begin', { method: 'POST' })
    const asr = await startAuthentication({ optionsJSON: begin.options })
    return api<PasskeyTokenResponse>(`/auth/webauthn/login/finish?${qs({ sid: begin.session_id })}`, {
      method: 'POST',
      body: JSON.stringify(asr),
    })
  },

  // Conditional-UI login: arms the browser's autofill so a passkey is offered in
  // the username field. The promise resolves only when the user picks a
  // credential from autofill; call cancelWebauthnCeremony() to abort it (e.g.
  // when the user takes a different sign-in path).
  async loginAutofill(): Promise<PasskeyTokenResponse> {
    const begin = await api<BeginResponse>('/auth/webauthn/login/begin', { method: 'POST' })
    const asr = await startAuthentication({ optionsJSON: begin.options, useBrowserAutofill: true })
    return api<PasskeyTokenResponse>(`/auth/webauthn/login/finish?${qs({ sid: begin.session_id })}`, {
      method: 'POST',
      body: JSON.stringify(asr),
    })
  },

  // Second factor: the server already issued the challenge in the login
  // response; assert it and finish.
  async finish2fa(sessionId: string, options: any): Promise<PasskeyTokenResponse> {
    const asr = await startAuthentication({ optionsJSON: options })
    return api<PasskeyTokenResponse>(`/auth/webauthn/login/finish?${qs({ sid: sessionId })}`, {
      method: 'POST',
      body: JSON.stringify(asr),
    })
  },
}

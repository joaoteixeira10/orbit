// Face ID, Touch ID, Windows Hello, a fingerprint: this device's own lock,
// as Orbit uses it. Each person turns it on for their device ("Use Face
// ID"); underneath it's a passkey made on this device and verified (the
// server marks it biometric), remembered here — which one, and whose — so
// the sign-in page asks for it at once and the lock knows what to ask for.
// The owner allows it, and may cap or require the lock (branding).
import { startAuthentication, startRegistration } from '@simplewebauthn/browser'
import { api, setAccessToken } from '@/lib/api'
import type { WebAuthnCredential } from '@/api/types'

// ---- What this device has ----
const KEY = 'orbit_biometric'
export interface DeviceBio {
  id: string // the server's credential row (to remove it)
  cred: string // the credential id, base64url (to ask for it alone)
  user: string // whose
}

export function deviceBio(): DeviceBio | null {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) || 'null')
    return v && v.id && v.cred && v.user ? v : null
  } catch {
    return null
  }
}
function setDeviceBio(v: DeviceBio | null) {
  try {
    if (v) localStorage.setItem(KEY, JSON.stringify(v))
    else localStorage.removeItem(KEY)
  } catch {
    /* private window: for this visit only */
  }
}
// forgetDeviceBio drops this device's record (its passkey gone elsewhere).
export const forgetDeviceBio = () => setDeviceBio(null)

// bioAvailable: does this device have its own lock a page may use?
export async function bioAvailable(): Promise<boolean> {
  try {
    return !!window.PublicKeyCredential && (await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable())
  } catch {
    return false
  }
}

// bioKind is what the device calls it (the i18n key under bio.kind).
export type BioKind = 'faceId' | 'touchId' | 'hello' | 'fingerprint' | 'device'
export function bioKind(ua = navigator.userAgent, touch = navigator.maxTouchPoints): BioKind {
  if (/iPhone|iPad|iPod/.test(ua) || (/Macintosh/.test(ua) && touch > 1)) return 'faceId'
  if (/Macintosh|Mac OS X/.test(ua)) return 'touchId'
  if (/Windows/.test(ua)) return 'hello'
  if (/Android/.test(ua)) return 'fingerprint'
  return 'device'
}

// ---- The ceremonies ----
interface Begin {
  session_id: string
  options: any
}
const sid = (b: Begin) => `sid=${encodeURIComponent(b.session_id)}`

export const bioApi = {
  // turnOn makes this device's own passkey ("Use Face ID") for the
  // signed-in person and remembers it here.
  async turnOn(userId: string, name: string): Promise<void> {
    const begin = await api<Begin>('/me/webauthn/register/begin?biometric=1', { method: 'POST' })
    const att = await startRegistration({ optionsJSON: begin.options })
    const row = await api<WebAuthnCredential>(`/me/webauthn/register/finish?${sid(begin)}&name=${encodeURIComponent(name)}`, {
      method: 'POST',
      body: JSON.stringify(att),
    })
    setDeviceBio({ id: String(row.id), cred: att.id, user: userId })
  },
  // turnOff removes it (here and on the server).
  async turnOff(): Promise<void> {
    const d = deviceBio()
    if (d) await api<void>(`/me/webauthn/${d.id}`, { method: 'DELETE' }).catch(() => {})
    setDeviceBio(null)
  },
  // signIn: this device's Face ID, alone; the access token.
  async signIn(): Promise<string> {
    const d = deviceBio()
    const begin = await api<Begin>(`/auth/webauthn/login/begin?biometric=1${d ? `&cred=${d.cred}` : ''}`, { method: 'POST' })
    const asr = await startAuthentication({ optionsJSON: begin.options })
    const res = await api<{ access_token?: string }>(`/auth/webauthn/login/finish?${sid(begin)}`, { method: 'POST', body: JSON.stringify(asr) })
    if (!res.access_token) throw new Error('Face ID sign-in failed')
    return res.access_token
  },
  // unlock: the signed-in person's own Face ID again (Orbit was locked).
  async unlock(): Promise<void> {
    const d = deviceBio()
    const begin = await api<Begin>(`/auth/step-up/webauthn/begin?biometric=1${d ? `&cred=${d.cred}` : ''}`, { method: 'POST' })
    const asr = await startAuthentication({ optionsJSON: begin.options })
    const res = await api<{ access_token: string }>(`/auth/step-up/webauthn/finish?${sid(begin)}`, { method: 'POST', body: JSON.stringify(asr) })
    setAccessToken(res.access_token)
  },
}

// ---- The lock ----
// Each device chooses when Orbit asks for Face ID again: never (-1), at
// once on coming back (0), or after so many idle minutes. The owner may cap
// it (max minutes) or require it (never isn't offered then).
const LOCK = 'orbit_bio_lock'
const ACTIVE = 'orbit_last_active'
export const LOCK_CHOICES = [-1, 0, 1, 5, 15, 60]

export function lockChoice(): number {
  try {
    const n = Number(localStorage.getItem(LOCK) ?? '-1')
    return Number.isFinite(n) ? n : -1
  } catch {
    return -1
  }
}
export function setLockChoice(n: number) {
  try {
    localStorage.setItem(LOCK, String(n))
  } catch {
    /* private window */
  }
}

// lockAfter is the lock in force: the device's choice within the owner's
// rule (-1 = none).
export function lockAfter(choice: number, max: number, required: boolean): number {
  if (choice < 0) return required ? max || 5 : -1
  return max > 0 && choice > max ? max : choice
}
// lockChoices are the choices the owner's rule leaves.
export function lockChoices(max: number, required: boolean): number[] {
  return LOCK_CHOICES.filter((n) => !(required && n < 0) && !(max > 0 && n > max))
}

export function lastActive(): number {
  try {
    return Number(localStorage.getItem(ACTIVE) || 0)
  } catch {
    return 0
  }
}
export function markActive(at = Date.now()) {
  try {
    localStorage.setItem(ACTIVE, String(at))
  } catch {
    /* private window */
  }
}

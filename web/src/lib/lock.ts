// Locked notes and pages: everything here happens in the browser. Each
// person has one lock key — 256 random bits — which Orbit keeps only wrapped
// (AES-GCM) by a key made from their PIN (PBKDF2-SHA-256, 600 000 rounds).
// A locked note's or page's text is sealed with the lock key (AES-GCM, a new
// IV each time). Orbit never sees the PIN or the key: forgetting the PIN
// loses what it locked. Once the PIN is entered the key stays in this tab's
// memory only, until locked again (by hand, after a while unused, or on
// reload).
import { ref } from 'vue'
import { api } from './api'

export const LOCK_ITERATIONS = 600_000

// Browsers give a page its crypto only over HTTPS (or on localhost): locking
// can't work on Orbit opened over plain http.
export const lockPossible = () => typeof window !== 'undefined' && window.isSecureContext && !!globalThis.crypto?.subtle
const IDLE_MS = 10 * 60_000 // unused this long: locked again

export interface LockKeyInfo {
  set: boolean
  salt?: string
  iterations?: number
  iv?: string
  wrapped?: string
  sealed: number // how many notes and pages are locked
}

export const lockApi = {
  get: () => api<LockKeyInfo>('/me/lock'),
  put: (k: { salt: string; iterations: number; iv: string; wrapped: string }) => api<LockKeyInfo>('/me/lock', { method: 'PUT', body: JSON.stringify(k) }),
  forget: () => api<void>('/me/lock', { method: 'DELETE' }),
}

const b64 = (b: ArrayBuffer | Uint8Array) => btoa(String.fromCharCode(...new Uint8Array(b instanceof Uint8Array ? b : new Uint8Array(b))))
const unb64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))
const rand = (n: number) => crypto.getRandomValues(new Uint8Array(n))

async function pinKey(pin: string, salt: Uint8Array, iterations: number): Promise<CryptoKey> {
  const base = await crypto.subtle.importKey('raw', new TextEncoder().encode(pin), 'PBKDF2', false, ['deriveKey'])
  return crypto.subtle.deriveKey({ name: 'PBKDF2', hash: 'SHA-256', salt, iterations }, base, { name: 'AES-GCM', length: 256 }, false, ['encrypt', 'decrypt'])
}

async function wrap(raw: Uint8Array, pin: string) {
  const salt = rand(16)
  const iv = rand(12)
  const k = await pinKey(pin, salt, LOCK_ITERATIONS)
  const wrapped = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, k, raw)
  return { salt: b64(salt), iterations: LOCK_ITERATIONS, iv: b64(iv), wrapped: b64(wrapped) }
}

// The open key, in this tab only.
let key: CryptoKey | null = null
let idle: ReturnType<typeof setTimeout> | undefined
export const unlocked = ref(false)

function keepOpen() {
  clearTimeout(idle)
  idle = setTimeout(lockNow, IDLE_MS)
}
export function lockNow() {
  clearTimeout(idle)
  key = null
  unlocked.value = false
}
async function open(raw: Uint8Array) {
  key = await crypto.subtle.importKey('raw', raw, 'AES-GCM', false, ['encrypt', 'decrypt'])
  raw.fill(0)
  unlocked.value = true
  keepOpen()
}

// setUp makes a lock key and keeps it wrapped by the PIN (the first time).
export async function setUp(pin: string): Promise<LockKeyInfo> {
  const raw = rand(32)
  const info = await lockApi.put(await wrap(raw, pin))
  await open(raw)
  return info
}

export class WrongPin extends Error {}

async function unwrap(info: LockKeyInfo, pin: string): Promise<Uint8Array> {
  if (!info.set || !info.salt || !info.iv || !info.wrapped || !info.iterations) throw new Error('no lock PIN set')
  const k = await pinKey(pin, unb64(info.salt), info.iterations)
  try {
    return new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: unb64(info.iv) }, k, unb64(info.wrapped)))
  } catch {
    throw new WrongPin('wrong PIN')
  }
}

// unlock opens the key with the PIN (WrongPin when it isn't).
export async function unlock(pin: string, info?: LockKeyInfo): Promise<void> {
  await open(await unwrap(info ?? (await lockApi.get()), pin))
}

// changePin wraps the same key with a new PIN (the old one checked here).
export async function changePin(oldPin: string, newPin: string): Promise<void> {
  const raw = await unwrap(await lockApi.get(), oldPin)
  const copy = raw.slice()
  await lockApi.put(await wrap(raw, newPin))
  raw.fill(0)
  await open(copy)
}

// seal encrypts a value (JSON) with the open key; unseal reads it back.
export async function seal(value: unknown): Promise<string> {
  if (!key) throw new Error('locked')
  keepOpen()
  const iv = rand(12)
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv }, key, new TextEncoder().encode(JSON.stringify(value)))
  return JSON.stringify({ v: 1, iv: b64(iv), ct: b64(ct) })
}
export async function unseal<T>(cipher: string): Promise<T> {
  if (!key) throw new Error('locked')
  keepOpen()
  const c = JSON.parse(cipher) as { v: number; iv: string; ct: string }
  const plain = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: unb64(c.iv) }, key, unb64(c.ct))
  return JSON.parse(new TextDecoder().decode(plain)) as T
}

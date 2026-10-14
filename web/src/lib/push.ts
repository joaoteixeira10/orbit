// Web Push on this device: the service worker (public/sw.js), the browser's
// permission and its push subscription, registered with the server so
// notifications arrive even with Orbit closed.
import { ref } from 'vue'
import { pushApi } from '@/api/planner'
import { router } from '@/router'

export type PushState = 'unsupported' | 'ios-install' | 'denied' | 'off' | 'on'

// state is this device's push status (refreshed by refreshPush).
export const pushState = ref<PushState>('off')

const isIOS = () => /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
const standalone = () => window.matchMedia('(display-mode: standalone)').matches || (navigator as any).standalone === true

export function pushSupported(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window && window.isSecureContext
}

let registration: Promise<ServiceWorkerRegistration | null> | null = null

// registerServiceWorker installs sw.js once and follows its messages: a
// notification click asks to go somewhere; a replaced subscription asks to
// be registered again.
export function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) return Promise.resolve(null)
  if (registration) return registration
  registration = navigator.serviceWorker.register('/sw.js').catch(() => null)
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data?.type === 'orbit:navigate' && typeof e.data.url === 'string') {
      const u = new URL(e.data.url, location.origin)
      if (u.origin === location.origin) void router.push(u.pathname + u.search + u.hash)
    } else if (e.data?.type === 'orbit:resubscribe') {
      void enablePush(false)
    }
  })
  return registration
}

// refreshPush works out the state, and re-sends an existing subscription so
// the server has it (for this account: the browser may have changed hands).
export async function refreshPush(): Promise<PushState> {
  if (!pushSupported()) {
    pushState.value = isIOS() && !standalone() ? 'ios-install' : 'unsupported'
    return pushState.value
  }
  if (Notification.permission === 'denied') return (pushState.value = 'denied')
  const reg = await registerServiceWorker()
  const sub = await reg?.pushManager.getSubscription()
  if (sub && Notification.permission === 'granted') {
    await pushApi.subscribe(sub.toJSON()).catch(() => {})
    return (pushState.value = 'on')
  }
  return (pushState.value = 'off')
}

function keyBytes(base64url: string): Uint8Array {
  const pad = '='.repeat((4 - (base64url.length % 4)) % 4)
  const raw = atob((base64url + pad).replace(/-/g, '+').replace(/_/g, '/'))
  return Uint8Array.from(raw, (c) => c.charCodeAt(0))
}

// enablePush asks for permission (when ask) and subscribes this device.
export async function enablePush(ask = true): Promise<PushState> {
  if (!pushSupported()) return refreshPush()
  if (ask && Notification.permission === 'default') await Notification.requestPermission()
  if (Notification.permission !== 'granted') return refreshPush()
  const info = await pushApi.info()
  if (!info.available) throw new Error('push unavailable')
  const reg = await registerServiceWorker()
  if (!reg) throw new Error('service worker unavailable')
  await navigator.serviceWorker.ready
  let sub = await reg.pushManager.getSubscription()
  // A subscription made with another key (the instance's keys changed) won't work.
  const want = keyBytes(info.public_key)
  const have = sub?.options.applicationServerKey ? new Uint8Array(sub.options.applicationServerKey) : null
  if (sub && (!have || have.length !== want.length || have.some((b, i) => b !== want[i]))) {
    await sub.unsubscribe()
    sub = null
  }
  sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: want as BufferSource })
  await pushApi.subscribe(sub.toJSON())
  return (pushState.value = 'on')
}

// disablePush unsubscribes this device.
export async function disablePush(): Promise<PushState> {
  const reg = await registerServiceWorker()
  const sub = await reg?.pushManager.getSubscription()
  if (sub) {
    await pushApi.remove({ endpoint: sub.endpoint }).catch(() => {})
    await sub.unsubscribe()
  }
  return refreshPush()
}

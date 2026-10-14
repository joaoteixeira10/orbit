// Live updates over Server-Sent Events — a small client-side event bus fed by a
// single long-lived stream. The server pushes typed messages ({"type":"…"}) and
// views subscribe to the types they care about:
//   - "privileges": the current user's permissions changed → re-fetch /auth/me
//     (handled internally below) so privilege-gated UI updates without a reload.
//   - "users" / "roles": the lists changed → the admin views re-fetch.
//   - "ws": something in a workspace changed ({ws, what, origin}) → its stores
//     reload (a tab skips the events it caused itself).
//   - "reminder": a task or event is due → a toast and a system notification.
//
// EventSource can't send an Authorization header, so we first fetch a one-time
// ticket (authenticated) and open the stream with ?ticket=. That ticket is
// single-use, so we close on error and reconnect ourselves with a fresh one.
import { api, getAccessToken } from '@/lib/api'

const STREAM = '/api/v1/auth/events'
const RECONNECT_MS = 5000

export interface LiveEvent {
  type: string
  [key: string]: unknown
}
type Handler = (e: LiveEvent) => void

const handlers = new Map<string, Set<Handler>>()
let es: EventSource | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let running = false

// onLiveEvent registers a handler for a server event type and returns an
// unsubscribe function (call it in onUnmounted).
export function onLiveEvent(type: string, handler: Handler): () => void {
  let set = handlers.get(type)
  if (!set) {
    set = new Set()
    handlers.set(type, set)
  }
  set.add(handler)
  return () => {
    handlers.get(type)?.delete(handler)
  }
}

function dispatch(raw: string) {
  let ev: LiveEvent
  try {
    ev = JSON.parse(raw)
  } catch {
    return
  }
  if (!ev?.type) return
  handlers.get(ev.type)?.forEach((h) => h(ev))
}

async function open() {
  if (!running || es || !getAccessToken()) return
  try {
    const { ticket } = await api<{ ticket: string }>('/auth/events/ticket', { method: 'POST' })
    if (!running) return // stopped while the ticket was in flight
    const source = new EventSource(`${STREAM}?ticket=${encodeURIComponent(ticket)}`)
    es = source
    source.onmessage = (e) => dispatch(e.data)
    source.onerror = () => {
      source.close()
      if (es === source) es = null
      scheduleReconnect()
    }
  } catch {
    scheduleReconnect()
  }
}

function scheduleReconnect() {
  if (!running || reconnectTimer) return
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    open()
  }, RECONNECT_MS)
}

// startLiveUpdates opens the stream (idempotent — safe to call on every fetchMe).
export function startLiveUpdates() {
  if (running) return
  running = true
  open()
}

// stopLiveUpdates closes the stream and cancels any pending reconnect.
export function stopLiveUpdates() {
  running = false
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  es?.close()
  es = null
}

// Built-in handler: re-sync the session when the server says privileges changed.
// Dynamic import avoids a static import cycle (the auth store starts the stream).
onLiveEvent('privileges', async () => {
  const { useAuthStore } = await import('@/stores/auth')
  await useAuthStore().fetchMe()
})

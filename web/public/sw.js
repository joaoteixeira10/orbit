// Orbit's service worker: Web Push notifications. It shows what the server
// pushes (see api/internal/inbox) unless an Orbit window is in front — that
// window shows it itself — and a click opens the right place, in an open
// Orbit window when there is one; its Snooze button puts it off an hour. It caches nothing (offline comes later).
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()))

self.addEventListener('push', (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch {
    data = { title: event.data ? event.data.text() : 'Orbit' }
  }
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
      const inFront = windows.some((w) => w.visibilityState === 'visible' && w.focused)
      if (inFront && data.kind !== 'test') return
      return self.registration.showNotification(data.title || 'Orbit', {
        body: data.body || '',
        tag: data.tag || undefined,
        renotify: !!data.tag,
        icon: '/favicon-192.png',
        badge: '/favicon-192.png',
        data: { url: data.url || '/', snooze: data.snooze || '' },
        // A "Snooze" button where the system shows buttons (Android, desktop).
        actions: data.snooze ? [{ action: 'snooze', title: data.snooze_label || 'Snooze 1 h' }] : [],
      })
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  // Snooze: a signed link (the worker has no session); it comes back in an hour.
  if (event.action === 'snooze' && event.notification.data?.snooze) {
    event.waitUntil(fetch(event.notification.data.snooze, { method: 'POST' }).catch(() => {}))
    return
  }
  const url = new URL(event.notification.data?.url || '/', self.location.origin).href
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
      const open = windows.find((w) => new URL(w.url).origin === self.location.origin)
      if (open) {
        // The app routes itself (no reload) when told where to go.
        open.postMessage({ type: 'orbit:navigate', url })
        return open.focus()
      }
      return self.clients.openWindow(url)
    }),
  )
})

// The browser replaced the subscription: tell the app to register the new one.
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    self.clients.matchAll({ type: 'window' }).then((windows) => windows.forEach((w) => w.postMessage({ type: 'orbit:resubscribe' }))),
  )
})

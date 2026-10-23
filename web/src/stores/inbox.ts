import { defineStore } from 'pinia'
import { inboxApi, type InboxItem } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { renderNotification } from '@/lib/inbox'
import { pushState } from '@/lib/push'
import { useToastStore } from '@/stores/toast'

// The notification inbox (the bell): the newest notifications, how many are
// unread, and live updates — a new one is added (or a grouped one moves to
// the top) as it happens.
export const useInboxStore = defineStore('inbox', {
  state: () => ({
    items: [] as InboxItem[],
    unread: 0,
    more: false,
    loaded: false,
  }),
  actions: {
    async load() {
      const r = await inboxApi.list()
      this.items = r.items
      this.unread = r.unread
      this.more = r.more
      this.loaded = true
    },
    async loadMore() {
      const last = this.items[this.items.length - 1]
      if (!last) return
      const r = await inboxApi.list(last.updated_at)
      this.items.push(...r.items.filter((n) => !this.items.some((x) => x.id === n.id)))
      this.more = r.more
    },
    // put adds or replaces one, newest first.
    put(n: InboxItem) {
      const old = this.items.find((x) => x.id === n.id)
      this.items = [n, ...this.items.filter((x) => x.id !== n.id)]
      if (!n.read && (!old || old.read)) this.unread++
    },
    async markRead(ids: string[]) {
      const fresh = this.items.filter((n) => ids.includes(n.id) && !n.read)
      if (!fresh.length) return
      for (const n of fresh) n.read = true
      this.unread = Math.max(0, this.unread - fresh.length)
      await inboxApi.read({ ids: fresh.map((n) => n.id) }).catch(() => {})
    },
    // snooze puts one off until then: it leaves the list (and comes back
    // as a new notification).
    async snooze(id: string, until: string) {
      const n = this.items.find((x) => x.id === id)
      await inboxApi.snooze(id, until)
      this.items = this.items.filter((x) => x.id !== id)
      if (n && !n.read) this.unread = Math.max(0, this.unread - 1)
    },
    async markAll() {
      for (const n of this.items) n.read = true
      this.unread = 0
      await inboxApi.read({ all: true }).catch(() => {})
    },
  },
})

let wired = false
// wireInboxLive keeps the inbox current and shows a toast for what's about
// the user (reminders, a task given to them, a workspace change). With push
// off on this device and Orbit in the background, a reminder still becomes
// a system notification while the tab is open.
export function wireInboxLive() {
  if (wired) return
  wired = true
  const s = useInboxStore()
  onLiveEvent('notification', (e) => {
    const n = e.notification as InboxItem | undefined
    if (!n) return
    s.put(n)
    if (n.count > 1 || n.kind === 'activity' || n.kind === 'digest') return
    const { title, body } = renderNotification(n)
    useToastStore().info(body ? `${title} — ${body}` : title)
    if (n.kind === 'reminder' && pushState.value !== 'on' && 'Notification' in window && Notification.permission === 'granted' && document.visibilityState !== 'visible') {
      new Notification(title, { body, tag: n.id, icon: '/favicon-192.png' })
    }
  })
  // Read on another tab or device.
  onLiveEvent('notifications-read', () => void s.load().catch(() => {}))
}

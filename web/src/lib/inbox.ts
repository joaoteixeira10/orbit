// Wording of inbox notifications, in the interface language. A notification
// carries what happened (verb + params); the sentences are in i18n/inbox.ts
// (and mirrored on the server for push and channels).
import { i18n } from '@/i18n'
import type { InboxItem } from '@/api/planner'

const t = (key: string, params?: Record<string, unknown>) => i18n.global.t(key, params ?? {})
const has = (key: string) => i18n.global.te(key)

// digestLines lists the day: events with their time, then tasks (⚠ when
// overdue) — symbols, as on the server.
function digestLines(p: Record<string, any>): string {
  const items: { k: string; t: string; at?: string }[] = Array.isArray(p.items) ? p.items : []
  return items
    .map((it) => {
      if (it.k === 'event') return `◷ ${it.at ? it.at + ' ' : ''}${it.t}`
      if (it.k === 'overdue') return `⚠ ${it.t}`
      return `○ ${it.t}${it.at ? ` (${it.at})` : ''}`
    })
    .join('\n')
}

// renderNotification is a notification's title and body.
export function renderNotification(n: Pick<InboxItem, 'verb' | 'params' | 'count'>): { title: string; body: string } {
  const p = n.params ?? {}
  const vars: Record<string, unknown> = { ...p, n: n.count, title: p.title || t('inbox.untitled') }
  if (typeof p.role === 'string' && has(`inbox.role.${p.role}`)) vars.role = t(`inbox.role.${p.role}`)
  if (n.verb === 'daily') {
    const tasks = Number(p.tasks ?? 0)
    const events = Number(p.events ?? 0)
    vars.summary = `${i18n.global.t('inbox.digest.tasks', { n: tasks }, tasks)}, ${i18n.global.t('inbox.digest.events', { n: events }, events)}`
    vars.lines = digestLines(p)
  }
  const base = `inbox.v.${n.verb}`
  if (!has(`${base}.title`)) return { title: n.verb, body: '' }
  const many = n.count > 1 && has(`${base}.titleMany`)
  const title = t(`${base}.${many ? 'titleMany' : 'title'}`, vars)
  // No workspace name (a reminder of a workspace since deleted): no dangling dot.
  const body = t(`${base}.${many ? 'bodyMany' : 'body'}`, vars).replace(/ · $/, '')
  return { title, body }
}

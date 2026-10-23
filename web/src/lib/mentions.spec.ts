import { describe, it, expect } from 'vitest'
import { markdownOf, mentionItems } from './mentions'
import { renderMarkdown } from './markdown'

const t = (k: string) => ({ 'dates.today': 'Today', 'dates.tomorrow': 'Tomorrow', 'dates.yesterday': 'Yesterday', 'pages.untitled': 'Untitled', 'mention.page': 'Page' })[k] ?? k
// Friday 23 October 2026.
const now = new Date(2026, 9, 23, 10)
const ctx = {
  ws: 'w1',
  members: [
    { user_id: 'u1', username: 'ana', display_name: 'Ana Silva' },
    { user_id: 'u2', username: 'bob', display_name: 'Bob' },
  ],
  pages: [
    { id: 'p1', title: 'Roadmap', icon: '' },
    { id: 'p2', title: 'Ana’s notes', icon: '📝' },
    { id: 'p3', title: 'Old', icon: '', trashed_at: '2026-10-01' },
    { id: 'self', title: 'Roadmap draft', icon: '' },
  ],
  self: 'self',
  now,
}

describe('mention suggestions', () => {
  it('offers people, then pages, then dates', () => {
    const it = mentionItems('an', ctx, t)
    expect(it.map((x) => x.key)).toEqual(['uu1', 'pp2'])
    expect(it[0].text).toBe('@ana')
    expect(markdownOf(it[1])).toBe('[Ana’s notes](/w/w1/p/p2)')
  })
  it('leaves out the page itself and trashed ones', () => {
    expect(mentionItems('road', ctx, t).map((x) => x.key)).toEqual(['pp1'])
    expect(mentionItems('old', ctx, t)).toEqual([])
  })
  it('knows dates by word, by weekday and written out', () => {
    expect(mentionItems('tom', ctx, t).find((x) => x.kind === 'date')?.text).toBe('@2026-10-24')
    expect(mentionItems('amanh', ctx, t).find((x) => x.kind === 'date')?.text).toBe('@2026-10-24')
    expect(mentionItems('tues', ctx, t).find((x) => x.kind === 'date')?.text).toBe('@2026-10-27')
    expect(mentionItems('2026-12-25', ctx, t)[0].text).toBe('@2026-12-25')
    expect(mentionItems('2026-02-30', ctx, t).some((x) => x.kind === 'date')).toBe(false)
  })
})

describe('mentions read', () => {
  it('shows @people and @dates as chips, in text only', () => {
    const html = renderMarkdown('Ask @ana by @2026-11-05, mail ana@example.org, `@bob` [Roadmap](/w/w1/p/p1)')
    expect(html).toContain('<span class="md-mention">@ana</span>')
    expect(html).toMatch(/<a href="\/calendar\?date=2026-11-05" class="md-internal md-date" title="2026-11-05">[^<]+<\/a>/)
    expect(html).toContain('ana@example.org')
    expect(html).not.toContain('md-mention">@example')
    expect(html).toContain('<code>@bob</code>')
    expect(html).toContain('<a href="/w/w1/p/p1" class="md-internal md-page">Roadmap</a>')
  })
})

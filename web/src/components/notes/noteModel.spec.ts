import { describe, it, expect } from 'vitest'
import type { Note } from '@/api/planner'
import { ago, convertedInput, draftOf, inputOf, isEmptyDraft, matches, newItem, noteText, pageDoc, sameContent, taskTitles, toChecklist, toText } from './noteModel'
import { NOTE_COLORS, noteColorCss, noteColors } from './noteColors'

const note = (over: Partial<Note> = {}): Note => ({
  id: 'n1',
  workspace_id: 'w',
  owner_id: '',
  title: '',
  body: '',
  checklist: false,
  items: [],
  color: '',
  pinned: false,
  rank: 1024,
  created_by: 'u',
  updated_by: 'u',
  created_at: '2026-10-14T10:00:00Z',
  updated_at: '2026-10-14T10:00:00Z',
  ...over,
})

describe('note drafts', () => {
  it('switches text ↔ checklist keeping the lines (without bullets)', () => {
    const d = draftOf(note({ body: '- Milk\n\n• Coffee\n[x] Bread ' }))
    toChecklist(d)
    expect(d.checklist).toBe(true)
    expect(d.items.map((i) => i.text)).toEqual(['Milk', 'Coffee', 'Bread'])
    toText(d)
    expect(d).toMatchObject({ checklist: false, body: 'Milk\nCoffee\nBread', items: [] })
  })

  it('sends checklists without empty rows and knows when it is empty', () => {
    const d = draftOf(null, { checklist: true, items: [newItem(' a '), newItem(''), newItem('b', true, 'x')] })
    expect(inputOf(d).items).toEqual([
      { id: '', text: 'a', done: false },
      { id: 'x', text: 'b', done: true },
    ])
    expect(inputOf(d).body).toBe('')
    expect(isEmptyDraft(draftOf(null, { checklist: true, items: [newItem('  ')] }))).toBe(true)
    expect(isEmptyDraft(draftOf(null, { title: 'T' }))).toBe(false)
  })

  it('compares a draft with the stored note', () => {
    const n = note({ title: 'T', body: 'x' })
    const d = draftOf(n)
    expect(sameContent(d, n)).toBe(true)
    d.body = 'x\n'
    expect(sameContent(d, n)).toBe(true) // the server trims the end
    d.color = 'blue'
    expect(sameContent(d, n)).toBe(false)
  })

  it('builds the conversion update for a stored note', () => {
    expect(convertedInput(note({ body: 'a\nb' }))).toMatchObject({ checklist: true, body: '', items: [{ text: 'a' }, { text: 'b' }] })
    expect(convertedInput(note({ checklist: true, items: [{ id: '1', text: 'a', done: true }] }))).toEqual({ checklist: false, body: 'a', items: [] })
  })
})

describe('note text', () => {
  const list = note({ title: 'Shop', checklist: true, items: [{ id: '1', text: 'Bread', done: true }, { id: '2', text: 'Milk', done: false }] })

  it('searches title, body and items', () => {
    expect(matches(list, 'milk')).toBe(true)
    expect(matches(list, 'shop')).toBe(true)
    expect(matches(list, 'tea')).toBe(false)
    expect(matches(note({ body: 'Call Ana' }), 'ana')).toBe(true)
  })

  it('copies open items first', () => {
    expect(noteText(list)).toBe('Shop\n\n☐ Milk\n☑ Bread')
  })

  it('turns into tasks: open items, else lines, else the title (max 50)', () => {
    expect(taskTitles(list)).toEqual(['Milk'])
    expect(taskTitles(note({ body: '- a\n\nb' }))).toEqual(['a', 'b'])
    expect(taskTitles(note({ title: 'Only' }))).toEqual(['Only'])
    expect(taskTitles(note({ body: Array.from({ length: 80 }, (_, i) => `t${i}`).join('\n') }))).toHaveLength(50)
  })

  it('makes a page (Markdown)', () => {
    expect(pageDoc(note({ body: 'a\nb\n\n*c*' }))).toBe('a  \nb\n\n\\*c\\*')
    expect(pageDoc(list).split('\n')[0]).toBe('- [x] Bread')
  })

  it('says how long ago', () => {
    const now = Date.parse('2026-10-14T12:00:00Z')
    expect(ago('2026-10-14T10:00:00Z', 'en-GB', now)).toBe('2 hours ago')
    expect(ago('2026-10-14T11:59:50Z', 'en-GB', now)).toBe('now')
    expect(ago('2026-10-13T12:00:00Z', 'en-GB', now)).toBe('yesterday')
  })
})

describe('note colours', () => {
  it('has a tint for every colour and a rule for each', () => {
    const css = noteColorCss()
    for (const c of NOTE_COLORS) {
      expect(noteColors[c].light).toMatch(/^#/)
      expect(css).toContain(`[data-note="${c}"]`)
    }
  })
})

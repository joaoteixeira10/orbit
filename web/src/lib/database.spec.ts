import { describe, it, expect } from 'vitest'
import { boardOf, guessType, importValue, passes, rowsOf, shownValue, type Database, type DBProp, type DBRow } from './database'

const db: Database = {
  props: [
    { id: 'st', name: 'Status', type: 'select', options: [{ id: 'todo', name: 'To do' }, { id: 'done', name: 'Done' }] },
    { id: 'n', name: 'Pages', type: 'number' },
    { id: 'tg', name: 'Tags', type: 'multi_select', options: [{ id: 'a', name: 'sci-fi' }, { id: 'b', name: 'classic' }] },
    { id: 'due', name: 'Due', type: 'date' },
    { id: 'ok', name: 'Owned', type: 'checkbox' },
    { id: 'who', name: 'Reader', type: 'person' },
  ],
  views: [],
}
const rows: DBRow[] = [
  { id: '1', title: 'Dune', rank: 3, props: { st: 'done', n: 412, tg: ['a'], due: '2026-10-20', ok: true, who: 'me1' } },
  { id: '2', title: 'Émile', rank: 1, props: { st: 'todo', n: 90, tg: ['b'] } },
  { id: '3', title: 'Neuromancer', rank: 2, props: { n: 271, tg: ['a', 'b'], due: '2026-10-27' } },
]
const c = { me: 'me1', today: '2026-10-23' }
const ids = (rs: DBRow[]) => rs.map((r) => r.id)

describe('database views', () => {
  it('filters each type', () => {
    const f = (prop: string, op: string, value?: string | number) => ids(rows.filter((r) => passes(r, { prop, op, value }, prop === 'title' ? 'title' : db.props.find((p) => p.id === prop)!.type, c)))
    expect(f('title', 'contains', 'emile')).toEqual(['2']) // accents and case aside
    expect(f('n', 'gt', 100)).toEqual(['1', '3'])
    expect(f('st', 'is', 'done')).toEqual(['1'])
    expect(f('st', 'empty')).toEqual(['3'])
    expect(f('tg', 'has', 'b')).toEqual(['2', '3'])
    expect(f('tg', 'has_not', 'b')).toEqual(['1'])
    expect(f('due', 'next_week')).toEqual(['3'])
    expect(f('due', 'before', '2026-10-25')).toEqual(['1'])
    expect(f('ok', 'unchecked')).toEqual(['2', '3'])
    expect(f('who', 'is', 'me')).toEqual(['1'])
  })
  it('sorts (empty last), and keeps the pages’ order otherwise', () => {
    expect(ids(rowsOf(db, { id: 'v', name: 'V', type: 'table' }, rows, c))).toEqual(['2', '3', '1'])
    expect(ids(rowsOf(db, { id: 'v', name: 'V', type: 'table', sorts: [{ prop: 'n', desc: true }] }, rows, c))).toEqual(['1', '3', '2'])
    expect(ids(rowsOf(db, { id: 'v', name: 'V', type: 'table', sorts: [{ prop: 'st' }] }, rows, c))).toEqual(['2', '1', '3'])
    expect(ids(rowsOf(db, { id: 'v', name: 'V', type: 'table', filters: [{ prop: 'n', op: 'lt', value: 300 }], sorts: [{ prop: 'title' }] }, rows, c))).toEqual(['2', '3'])
  })
  it('groups a board by a select, rows without a value first', () => {
    const b = boardOf(db, { id: 'v', name: 'V', type: 'board', group_by: 'st' }, rows, 'None')
    expect(b.map((x) => [x.key, ids(x.rows)])).toEqual([['', ['3']], ['todo', ['2']], ['done', ['1']]])
  })
  it('reads values', () => {
    const names = new Map([['me1', 'Ana']])
    expect(shownValue(db.props[2], ['a', 'b'], names)).toBe('sci-fi, classic')
    expect(shownValue(db.props[5], 'me1', names)).toBe('Ana')
    expect(shownValue(db.props[4], false, names)).toBe('')
  })
  it('guesses imported columns and reads their values', () => {
    expect(guessType(['12', '3.5', ''])).toBe('number')
    expect(guessType(['Yes', 'No'])).toBe('checkbox')
    expect(guessType(['2026-10-01', '2026-11-02'])).toBe('date')
    expect(guessType(['Done', 'Doing', 'Done', 'Done'])).toBe('select')
    expect(guessType(['a long, free sentence', 'another one, too', 'third, here'])).toBe('text')
    const p: DBProp = { id: 's', name: 'S', type: 'select' }
    const a = importValue(p, 'Done')
    expect(importValue(p, 'Done')).toBe(a)
    expect(p.options!.map((o) => o.name)).toEqual(['Done'])
    expect(importValue({ id: 'c', name: 'C', type: 'checkbox' }, 'Yes')).toBe(true)
  })
})

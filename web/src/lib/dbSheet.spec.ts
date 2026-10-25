import { describe, expect, it } from 'vitest'
import { gridOf, planImport, sheetOf, typed } from './dbSheet'
import { readCsv } from './xlsx'
import type { Database, DBProp, DBRow } from './database'

const status: DBProp = { id: 's', name: 'Status', type: 'select', options: [{ id: 'o1', name: 'Done' }] }
const tags: DBProp = { id: 'g', name: 'Tags', type: 'multi_select', options: [{ id: 'a', name: 'red' }, { id: 'b', name: 'blue' }] }
const cost: DBProp = { id: 'c', name: 'Cost', type: 'number' }
const due: DBProp = { id: 'd', name: 'Due', type: 'date' }
const ok: DBProp = { id: 'k', name: 'Paid', type: 'checkbox' }
const note: DBProp = { id: 'n', name: 'Note', type: 'text' }
const who: DBProp = { id: 'w', name: 'Owner', type: 'person' }
const twice: DBProp = { id: 'f', name: 'Twice', type: 'formula', formula: '=[Cost]*2' }
const names = new Map([['u1', 'Ana']])

describe('typed', () => {
  it('writes values as a cell takes them', () => {
    expect(typed(cost, 12.5, names)).toBe('12.5')
    expect(typed(due, '2026-10-25', names)).toBe('2026-10-25')
    expect(typed(ok, true, names)).toBe('TRUE')
    expect(typed(ok, undefined, names)).toBe('FALSE')
    expect(typed(status, 'o1', names)).toBe('Done')
    expect(typed(tags, ['a', 'b'], names)).toBe('red, blue')
    expect(typed(who, 'u1', names)).toBe('Ana')
    expect(typed(twice, 25, names)).toBe('25')
    // Text that would read as something else stays text.
    expect(typed(note, '007', names)).toBe("'007")
    expect(typed(note, '=1+1', names)).toBe("'=1+1")
    expect(typed(note, 'hello', names)).toBe('hello')
  })
})

describe('sheetOf and gridOf', () => {
  it('goes out and comes back the same', () => {
    const rows: DBRow[] = [
      { id: 'r1', title: 'Rent', rank: 1, props: { c: 650, d: '2026-11-01', k: true, s: 'o1', g: ['a'], n: '42' } },
      { id: 'r2', title: '2026', rank: 2, props: {} },
    ]
    const data = sheetOf(rows, [cost, due, ok, status, tags, note], names, { name: 'Name', tab: 'Bills' })
    const tab = data.tabs[0]
    expect(tab.name).toBe('Bills')
    expect(tab.freeze).toBe(1)
    expect(tab.cells.A1).toEqual({ v: 'Name', b: true })
    expect(tab.cells.C2).toEqual({ v: '2026-11-01', nf: 'date' })
    expect(tab.cells.A3.v).toBe("'2026")
    const grid = gridOf(data, false, '2026-10-25')
    expect(grid.headers).toEqual(['Name', 'Cost', 'Due', 'Paid', 'Status', 'Tags', 'Note'])
    expect(grid.rows[0]).toEqual(['Rent', '650', '2026-11-01', 'true', 'Done', 'red', '42'])
    expect(grid.rows[1]).toEqual(['2026', '', '', 'false', '', '', ''])
  })

  it('reads a Portuguese CSV, skipping empty rows', () => {
    const grid = gridOf(readCsv('Nome;Valor;Dia\nRenda;1.234,50;25/10/2026\n;;\nLuz;12,5;\n', 'CSV'), true, '2026-10-25')
    expect(grid.headers).toEqual(['Nome', 'Valor', 'Dia'])
    expect(grid.rows).toEqual([
      ['Renda', '1234.5', '2026-10-25'],
      ['Luz', '12.5', ''],
    ])
  })
})

describe('planImport', () => {
  const db: Database = { props: [status, cost, twice, who], views: [] }
  it('matches columns by name, makes new ones, finds the name column', () => {
    const plan = planImport(
      db,
      {
        headers: ['cost', 'Nome', 'STATUS', 'Twice', 'Owner', 'Paid', 'Tags'],
        rows: [
          ['10', 'Rent', 'Done', '99', 'ana', 'true', 'x, y'],
          ['', 'Food', 'Open', '', 'nobody', 'false', 'y'],
        ],
      },
      [{ id: 'u1', names: ['Ana Silva', 'ana'] }],
      ['Name', 'Nome'],
    )
    expect(plan.added).toEqual(['Paid', 'Tags'])
    const paid = plan.db.props.find((p) => p.name === 'Paid')!
    const tg = plan.db.props.find((p) => p.name === 'Tags')!
    expect(paid.type).toBe('checkbox')
    // Status gained the option it didn't have; the original is untouched.
    const st = plan.db.props.find((p) => p.id === 's')!
    expect(st.options!.map((o) => o.name)).toEqual(['Done', 'Open'])
    expect(status.options!.length).toBe(1)
    const open = st.options![1].id
    expect(plan.rows[0]).toEqual({ title: 'Rent', props: { c: 10, s: 'o1', w: 'u1', [paid.id]: true, [tg.id]: expect.any(String) } })
    expect(plan.rows[1].title).toBe('Food')
    expect(plan.rows[1].props.s).toBe(open)
    // A formula's column is left out (it works itself out); unknown people too.
    expect(plan.rows[0].props.f).toBeUndefined()
    expect(plan.rows[1].props.w).toBeUndefined()
  })

  it('splits a multi-select column it already has', () => {
    const plan = planImport({ props: [tags], views: [] }, { headers: ['Title', 'Tags'], rows: [['A', 'red; green, red']] }, [], ['Name'])
    const g = plan.db.props[0]
    expect(g.options!.map((o) => o.name)).toEqual(['red', 'blue', 'green'])
    expect(plan.rows[0].props.g).toEqual(['a', g.options![2].id])
    // No name column: the first one names the rows.
    expect(plan.rows[0].title).toBe('A')
  })
})

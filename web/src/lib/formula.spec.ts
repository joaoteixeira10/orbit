import { describe, it, expect } from 'vitest'
import { dayOf, evaluate, parse, type Ctx, type FValue } from './formula'
import { total, withFormulas, type Database, type DBRow } from './database'

const row: Record<string, FValue> = { price: 12.5, qty: 4, done: false, name: 'Pens', due: { date: dayOf('2026-11-05') }, notes: null }
const col: Record<string, FValue[]> = { qty: [4, 6, null], price: [12.5, 3, 1] }
const c: Ctx = { today: dayOf('2026-10-23'), row: (n) => row[n.toLowerCase()], column: (n) => col[n.toLowerCase()] }
const f = (src: string) => evaluate(parse(src), c)

describe('formulas, as in Excel', () => {
  it('does arithmetic with Excel’s precedence', () => {
    expect(f('=[@Price]*[@Qty]')).toBe(50)
    expect(f('Price * Qty + 1')).toBe(51)
    expect(f('=2+3*4^2')).toBe(50)
    expect(f('=-2^2')).toBe(4) // Excel: the minus binds first
    expect(f('=50%*[@Qty]')).toBe(2)
    expect(f('=[@Qty]/0')).toEqual({ error: '#DIV/0!' })
  })
  it('knows functions in English and Portuguese, with , or ;', () => {
    expect(f('=SUM([Qty])')).toBe(10)
    expect(f('=SOMA([Qty]; 1)')).toBe(11)
    expect(f('=AVERAGE([Price])')).toBe(5.5)
    expect(f('=ROUND(2.345, 2)')).toBe(2.35)
    expect(f('=ARRED(2.345; 1)')).toBe(2.3)
    expect(f('=IF([@Done], 0, [@Qty])')).toBe(4)
    expect(f('=SE(NÃO([@Done]); "open"; "closed")')).toBe('open')
    expect(f('=IFERROR(1/0, "n/a")')).toBe('n/a')
    expect(f('=COUNTIF([Price], ">2")')).toBe(2)
    expect(f('=SUMIF([Price], ">2", [Qty])')).toBe(10)
    expect(f('=COUNTA([Qty])')).toBe(2)
    expect(f('=NOPE(1)')).toEqual({ error: '#NAME?' })
    expect(f('=[@Missing]+1')).toEqual({ error: '#NAME?' })
  })
  it('joins text and compares', () => {
    expect(f('=[@Name] & " × " & [@Qty]')).toBe('Pens × 4')
    expect(f('=UPPER(LEFT([@Name], 3))')).toBe('PEN')
    expect(f('=[@Qty] >= 4')).toBe(true)
    expect(f('="a" <> "A"')).toBe(false) // as in Excel: case aside
    expect(f('=ISBLANK([@Notes])')).toBe(true)
  })
  it('works with dates', () => {
    expect(f('=DAYS([@Due], TODAY())')).toBe(13)
    expect(f('=[@Due] - TODAY()')).toBe(13)
    expect(f('=[@Due] + 7')).toEqual({ date: dayOf('2026-11-12') })
    expect(f('=YEAR([@Due]) & "-" & MONTH([@Due])')).toBe('2026-11')
    expect(f('=DATEDIF(DATE(2026,1,31), DATE(2026,3,30), "M")')).toBe(1)
    expect(f('=EDATE(DATE(2026,1,31), 1)')).toEqual({ date: dayOf('2026-02-28') })
    expect(f('=NETWORKDAYS(DATE(2026,10,19), DATE(2026,10,25))')).toBe(5)
  })
  it('says what’s wrong with a formula that can’t be read', () => {
    expect(() => parse('=SUM(1, 2')).toThrow()
    expect(() => parse('="open')).toThrow()
    expect(() => parse('')).toThrow()
  })
})

describe('formula properties', () => {
  const db: Database = {
    props: [
      { id: 'p', name: 'Price', type: 'number' },
      { id: 'q', name: 'Qty', type: 'number' },
      { id: 's', name: 'Status', type: 'select', options: [{ id: 'o', name: 'Paid' }] },
      { id: 't', name: 'Total', type: 'formula', formula: '=[@Price]*[@Qty]' },
      { id: 'sh', name: 'Share', type: 'formula', formula: '=ROUND([@Total]/SUM([Total]), 2)' },
      { id: 'loop', name: 'Loop', type: 'formula', formula: '=[@Loop]+1' },
      { id: 'lbl', name: 'Label', type: 'formula', formula: '=[@Name] & " (" & [@Status] & ")"' },
    ],
    views: [],
  }
  const rows: DBRow[] = [
    { id: '1', title: 'Pens', rank: 1, props: { p: 2, q: 10, s: 'o' } },
    { id: '2', title: 'Ink', rank: 2, props: { p: 15, q: 2 } },
  ]
  it('works each out per row, columns and other formulas included; loops are #REF!', () => {
    const out = withFormulas(db, rows, new Map(), '2026-10-23')
    expect(out.map((r) => r.props.t)).toEqual([20, 30])
    expect(out.map((r) => r.props.sh)).toEqual([0.4, 0.6])
    expect(out[0].props.loop).toBe('#REF!')
    expect(out.map((r) => r.props.lbl)).toEqual(['Pens (Paid)', 'Ink ()'])
    expect(rows[0].props.t).toBeUndefined() // the rows themselves untouched
    expect(total(out, 't', 'sum')).toBe(50)
    expect(total(out, 'p', 'avg')).toBe(8.5)
    expect(total(out, 's', 'empty')).toBe(1)
    expect(total(out, 'title', 'count')).toBe(2)
  })
})

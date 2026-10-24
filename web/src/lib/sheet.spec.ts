import { describe, it, expect } from 'vitest'
import { Book, fromTSV, literal, reshape, series, show, toTSV, type SheetData } from './sheet'
import { dayOf, parse, rewriteRefs, shiftFormula } from './formula'

const today = dayOf('2026-10-23')
const sheet = (cells: Record<string, string>, more: Record<string, Record<string, string>> = {}): SheetData => ({
  tabs: [
    { id: 't1', name: 'Budget', rows: 100, cols: 26, cells: Object.fromEntries(Object.entries(cells).map(([a, v]) => [a, { v }])) },
    ...Object.entries(more).map(([name, cs], i) => ({ id: `x${i}`, name, rows: 100, cols: 26, cells: Object.fromEntries(Object.entries(cs).map(([a, v]) => [a, { v }])) })),
  ],
})
const val = (b: Book, a: string) => {
  const m = /^([A-Z]+)(\d+)$/.exec(a)!
  return b.value('t1', Number(m[2]) - 1, m[1].charCodeAt(0) - 65)
}

describe('sheet values', () => {
  it('reads what was typed: numbers both ways, %, €, dates, TRUE, text', () => {
    expect(literal('1,234.5')).toBe(1234.5)
    expect(literal('1234,5')).toBe(1234.5)
    expect(literal('1,234')).toBe(1234)
    expect(literal('1,234', true)).toBe(1.234)
    expect(literal('1.234', true)).toBe(1234)
    expect(literal('15%')).toBe(0.15)
    expect(literal('€ 12')).toBe(12)
    expect(literal('2026-11-05')).toEqual({ date: dayOf('2026-11-05') })
    expect(literal('05/11/2026')).toEqual({ date: dayOf('2026-11-05') })
    expect(literal('true')).toBe(true)
    expect(literal("'42")).toBe('42')
    expect(literal('Rent')).toBe('Rent')
  })
  it('works formulas out: cells, ranges, whole columns, other tabs, lookups', () => {
    const b = new Book(
      sheet(
        { A1: 'Rent', B1: '650', A2: 'Food', B2: '300', A3: 'Total', B3: '=SUM(B1:B2)', C1: '=B1/$B$3', D1: '=SUM(B:B)', E1: '=VLOOKUP("Food", A1:B2, 2, FALSE)', F1: '=Rates!A1*B1', G1: "='Rates'!A1 + 1", H1: '=INDEX(A1:B2, 2, 1)', I1: '=MATCH("Food", A1:A3, 0)' },
        { Rates: { A1: '2' } },
      ),
      today,
    )
    expect(val(b, 'B3')).toBe(950)
    expect(val(b, 'C1')).toBeCloseTo(0.684, 3)
    expect(val(b, 'D1')).toBe(1900) // B1 + B2 + B3
    expect(val(b, 'E1')).toBe(300)
    expect(val(b, 'F1')).toBe(1300)
    expect(val(b, 'G1')).toBe(3)
    expect(val(b, 'H1')).toBe('Food')
    expect(val(b, 'I1')).toBe(2)
  })
  it('reports errors as Excel does; a cell that needs itself is #REF!', () => {
    const b = new Book(sheet({ A1: '=A2', A2: '=A1', B1: '=1/0', C1: '=Nope!A1', D1: '=SUM(', E1: '=SOMA(B2; 3)' }), today)
    expect(val(b, 'A1')).toEqual({ error: '#REF!' })
    expect(val(b, 'B1')).toEqual({ error: '#DIV/0!' })
    expect(val(b, 'C1')).toEqual({ error: '#REF!' })
    expect(val(b, 'D1')).toEqual({ error: '#ERROR!' })
    expect(val(b, 'E1')).toBe(3)
  })
  it('shows values in their format', () => {
    const w = { yes: 'TRUE', no: 'FALSE' }
    expect(show(1234.5, { v: '', nf: 'num' }, 'en-GB', w)).toEqual({ text: '1,234.50', align: 'r' })
    expect(show(0.256, { v: '', nf: 'pct' }, 'en-GB', w).text).toBe('25.6%')
    expect(show(12.5, { v: '', nf: 'cur' }, 'pt-PT', w).text).toMatch(/12,50\s?€/)
    expect(show(0.1 + 0.2, undefined, 'en-GB', w).text).toBe('0.3')
    expect(show(2.5, undefined, 'pt-PT', w).text).toBe('2,5')
    expect(show({ error: '#N/A' }, undefined, 'en-GB', w)).toEqual({ text: '#N/A', align: 'c', error: true })
    expect(show(true, undefined, 'en-GB', w)).toEqual({ text: 'TRUE', align: 'c' })
  })
})

describe('references move as in Excel', () => {
  it('shifts relative references, keeps absolute ones', () => {
    expect(shiftFormula('=A1+$B$1+B$2+$C3', 2, 1)).toBe('=B3+$B$1+C$2+$C5')
    expect(shiftFormula('=SUM(A1:A3)*Tab2!B2', 1, 0)).toBe('=SUM(A2:A4)*Tab2!B3')
    expect(shiftFormula('=A1', -1, 0)).toBe('=#REF!')
    expect(shiftFormula('="A1"&A1', 1, 0)).toBe('="A1"&A2') // text stays
    expect(rewriteRefs("='My tab'!a1", (r) => r)).toBe("='My tab'!A1")
    expect(() => parse('=Sheet!', { cells: true })).toThrow()
  })
  it('inserting and deleting rows and columns moves cells and every reference to them', () => {
    const d = sheet({ A1: '1', A2: '2', A3: '=SUM(A1:A2)', B1: '=A2*2' }, { Other: { A1: '=Budget!A2' } })
    const ins = reshape(d, 't1', 'row', 1, 1) // a row before row 2
    expect(ins.tabs[0].cells.A3.v).toBe('2')
    expect(ins.tabs[0].cells.A4.v).toBe('=SUM(A1:A3)')
    expect(ins.tabs[0].cells.B1.v).toBe('=A3*2')
    expect(ins.tabs[1].cells.A1.v).toBe('=Budget!A3')
    const del = reshape(d, 't1', 'row', 1, -1) // row 2 goes
    expect(del.tabs[0].cells.A2.v).toBe('=SUM(A1:A1)')
    expect(del.tabs[0].cells.B1.v).toBe('=#REF!*2')
    expect(del.tabs[1].cells.A1.v).toBe('=#REF!')
    const col = reshape(d, 't1', 'col', 0, 1) // a column before A
    expect(col.tabs[0].cells.B3.v).toBe('=SUM(B1:B2)')
    expect(col.tabs[0].cells.C1.v).toBe('=B2*2')
  })
  it('fills series and swaps TSV with Excel', () => {
    expect(series(['1', '2'], 3)).toEqual(['3', '4', '5'])
    expect(series(['5'], 2)).toEqual(['5', '5'])
    expect(series(['a', 'b'], 3)).toEqual(['a', 'b', 'a'])
    const rows = [['a', 'b\tc'], ['line\nbreak', '"q"']]
    expect(fromTSV(toTSV(rows))).toEqual(rows)
    expect(fromTSV('1\t2\r\n3\t4\r\n')).toEqual([['1', '2'], ['3', '4']])
  })
})

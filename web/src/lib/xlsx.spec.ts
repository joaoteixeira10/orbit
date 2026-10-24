import { describe, it, expect } from 'vitest'
import { unzipSync, strFromU8 } from 'fflate'
import { Book, type SheetData } from './sheet'
import { dayOf, toExcel } from './formula'
import { csvOf, readCsv, readXlsx, writeXlsx } from './xlsx'

const today = dayOf('2026-10-23')
const data: SheetData = {
  tabs: [
    {
      id: 't1', name: 'Budget', rows: 100, cols: 26, freeze: 1, widths: { A: 140 },
      cells: {
        A1: { v: 'Item', b: true }, B1: { v: 'Cost', b: true },
        A2: { v: 'Rent' }, B2: { v: '650', nf: 'cur' },
        A3: { v: 'Food' }, B3: { v: '12,5' },
        A4: { v: 'Total', i: true }, B4: { v: '=SOMA(B2:B3; 0)', nf: 'num' },
        C2: { v: '2026-11-05' }, D2: { v: 'TRUE' }, E2: { v: "'007" },
      },
    },
    { id: 't2', name: 'Other tab', rows: 100, cols: 26, cells: { A1: { v: '=Budget!B4*2' } } },
  ],
}

describe('Excel files', () => {
  it('writes formulas in Excel’s English', () => {
    expect(toExcel('=SOMA(B2:B3; 0)')).toBe('SUM(B2:B3, 0)')
    expect(toExcel('=SE(A1>2; VERDADEIRO; "x;y")')).toBe('IF(A1>2, TRUE, "x;y")')
  })
  it('goes out and comes back: values, formulas, formats, widths, the frozen row', () => {
    const bytes = writeXlsx(data, new Book(data, today))
    const files = unzipSync(bytes)
    expect(Object.keys(files)).toContain('xl/worksheets/sheet2.xml')
    expect(strFromU8(files['xl/worksheets/sheet1.xml'])).toContain('<f>SUM(B2:B3, 0)</f><v>662.5</v>')
    const back = readXlsx(bytes)
    expect(back.tabs.map((t) => t.name)).toEqual(['Budget', 'Other tab'])
    const t = back.tabs[0]
    expect(t.cells.B4).toEqual({ v: '=SUM(B2:B3, 0)', nf: 'num' })
    expect(t.cells.A4.i).toBe(true)
    expect(t.cells.A1.b).toBe(true)
    expect(t.cells.B2).toMatchObject({ v: '650', nf: 'cur' })
    expect(t.cells.C2.v).toBe('2026-11-05')
    expect(t.cells.D2.v).toBe('TRUE')
    expect(t.cells.E2.v).toBe("'007")
    expect(t.freeze).toBe(1)
    expect(t.widths?.A).toBe(140)
    const b2 = new Book(back, today)
    expect(b2.value(back.tabs[1].id, 0, 0)).toBe(1325)
  })
  it('reads and writes CSV (; too)', () => {
    const d = readCsv('Item;Cost\r\nRent;650\r\n"A; b";"say ""hi"""\r\n', 'Imported')
    expect(d.tabs[0].cells).toMatchObject({ A1: { v: 'Item' }, B2: { v: '650' }, A3: { v: 'A; b' }, B3: { v: 'say "hi"' } })
    const book = new Book(data, today)
    const csv = csvOf(data.tabs[0], (r, c) => String(book.value('t1', r, c) ?? ''))
    expect(csv.split('\r\n')[3]).toBe('Total,662.5,,,')
  })
})

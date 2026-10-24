// Excel files (.xlsx) in and out, and CSV: an .xlsx is a zip of XML, so
// it's read and written here with the unzipping Orbit already has (fflate) —
// cells, formulas (in Excel's English, with their last values), number
// formats, bold / italic, column widths and a frozen top row. Charts,
// pictures, merged cells and conditional formats aren't carried.
import { strFromU8, strToU8, unzipSync, zipSync } from 'fflate'
import { colIndex, colName, isDate, isErr, isoOf, shiftFormula, toExcel, type FValue } from './formula'
import { Book, newId, newTab, parseAddr, type Cell, type NumFmt, type SheetData, type Tab } from './sheet'

const EXCEL_EPOCH = 25569 // Excel's serial number of 1970-01-01
const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
const XML = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'

// ---- Writing ----

// Number formats: Excel's built-in ids, and euros (a custom one).
const NUMFMT: Record<NumFmt, number> = { int: 3, num: 4, pct: 10, date: 14, cur: 164 }

export function writeXlsx(data: SheetData, book: Book): Uint8Array {
  // Styles: each look used gets one entry (index 0: none).
  const xfs = ['0|0|0']
  const styleOf = (c: Cell | undefined, v: FValue): number => {
    const nf = c?.nf ?? (isDate(v) ? 'date' : undefined)
    const key = `${nf ? NUMFMT[nf] : 0}|${c?.b ? 1 : 0}|${c?.i ? 1 : 0}`
    let i = xfs.indexOf(key)
    if (i < 0) i = xfs.push(key) - 1
    return i
  }
  const sheets = data.tabs.map((tab) => {
    const byRow = new Map<number, [number, string, Cell][]>()
    for (const [a, cell] of Object.entries(tab.cells)) {
      const p = parseAddr(a)
      if (!p || (cell.v === '' && !cell.b && !cell.i && !cell.nf)) continue
      if (!byRow.has(p.r)) byRow.set(p.r, [])
      byRow.get(p.r)!.push([p.c, a, cell])
    }
    const rows = [...byRow.entries()].sort((x, y) => x[0] - y[0]).map(([r, cells]) => {
      const cs = cells.sort((x, y) => x[0] - y[0]).map(([c, a, cell]) => {
        const v = book.value(tab.id, r, c)
        const s = styleOf(cell, v)
        const st = s ? ` s="${s}"` : ''
        const f = cell.v.startsWith('=') && cell.v.length > 1 ? `<f>${esc(toExcel(cell.v))}</f>` : ''
        if (v === null || v === '') return f ? `<c r="${a}"${st}>${f}</c>` : `<c r="${a}"${st}/>`
        if (isErr(v)) return `<c r="${a}"${st} t="e">${f}<v>${esc(v.error)}</v></c>`
        if (typeof v === 'boolean') return `<c r="${a}"${st} t="b">${f}<v>${v ? 1 : 0}</v></c>`
        if (typeof v === 'number') return `<c r="${a}"${st}>${f}<v>${v}</v></c>`
        if (isDate(v)) return `<c r="${a}"${st}>${f}<v>${v.date + EXCEL_EPOCH}</v></c>`
        const text = esc(String(v))
        return f ? `<c r="${a}"${st} t="str">${f}<v>${text}</v></c>` : `<c r="${a}"${st} t="inlineStr"><is><t xml:space="preserve">${text}</t></is></c>`
      })
      return `<row r="${r + 1}">${cs.join('')}</row>`
    })
    const widths = Object.entries(tab.widths ?? {})
      .map(([l, px]) => [colIndex(l) + 1, px] as const)
      .sort((a, b) => a[0] - b[0])
      .map(([i, px]) => `<col min="${i}" max="${i}" width="${(px / 7).toFixed(2)}" customWidth="1"/>`)
    const pane = tab.freeze
      ? `<sheetViews><sheetView workbookViewId="0"><pane ySplit="${tab.freeze}" topLeftCell="A${tab.freeze + 1}" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>`
      : ''
    return (
      XML +
      `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">${pane}` +
      (widths.length ? `<cols>${widths.join('')}</cols>` : '') +
      `<sheetData>${rows.join('')}</sheetData></worksheet>`
    )
  })
  const fonts = ['<font><sz val="11"/><name val="Calibri"/></font>', '<font><b/><sz val="11"/><name val="Calibri"/></font>', '<font><i/><sz val="11"/><name val="Calibri"/></font>', '<font><b/><i/><sz val="11"/><name val="Calibri"/></font>']
  const styles =
    XML +
    `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
    `<numFmts count="1"><numFmt numFmtId="164" formatCode="#,##0.00\\ &quot;€&quot;"/></numFmts>` +
    `<fonts count="${fonts.length}">${fonts.join('')}</fonts>` +
    `<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
    `<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
    `<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
    `<cellXfs count="${xfs.length}">${xfs
      .map((k) => {
        const [nf, b, i] = k.split('|').map(Number)
        const font = b && i ? 3 : b ? 1 : i ? 2 : 0
        return `<xf numFmtId="${nf}" fontId="${font}" fillId="0" borderId="0" xfId="0"${nf ? ' applyNumberFormat="1"' : ''}${font ? ' applyFont="1"' : ''}/>`
      })
      .join('')}</cellXfs></styleSheet>`
  const files: Record<string, Uint8Array> = {
    '[Content_Types].xml': strToU8(
      XML +
        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` +
        `<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
        `<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
        sheets.map((_, i) => `<Override PartName="/xl/worksheets/sheet${i + 1}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`).join('') +
        `</Types>`,
    ),
    '_rels/.rels': strToU8(
      XML +
        `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
    ),
    'xl/workbook.xml': strToU8(
      XML +
        `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` +
        data.tabs.map((t, i) => `<sheet name="${esc(t.name.slice(0, 31))}" sheetId="${i + 1}" r:id="rId${i + 1}"/>`).join('') +
        `</sheets><calcPr fullCalcOnLoad="1"/></workbook>`,
    ),
    'xl/_rels/workbook.xml.rels': strToU8(
      XML +
        `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
        sheets.map((_, i) => `<Relationship Id="rId${i + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet${i + 1}.xml"/>`).join('') +
        `<Relationship Id="rId${sheets.length + 1}" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
    ),
    'xl/styles.xml': strToU8(styles),
  }
  sheets.forEach((xml, i) => (files[`xl/worksheets/sheet${i + 1}.xml`] = strToU8(xml)))
  return zipSync(files, { level: 6 })
}

// ---- Reading ----

const DATE_FMTS = new Set([14, 15, 16, 17, 22, 27, 30, 36, 45, 46, 47, 50, 57])
const xml = (s: string) => new DOMParser().parseFromString(s, 'application/xml')
const all = (n: Document | Element, tag: string) => [...n.getElementsByTagName(tag)]
const textOf = (n: Element | undefined) => (n ? all(n, 't').map((t) => t.textContent ?? '').join('') : '')

export function readXlsx(bytes: Uint8Array, maxCells = 500_000): SheetData {
  const files = unzipSync(bytes)
  const read = (p: string) => (files[p] ? strFromU8(files[p]) : '')
  const wb = xml(read('xl/workbook.xml'))
  const rels = xml(read('xl/_rels/workbook.xml.rels'))
  const target = new Map(all(rels, 'Relationship').map((r) => [r.getAttribute('Id') ?? '', r.getAttribute('Target') ?? '']))
  const shared = files['xl/sharedStrings.xml'] ? all(xml(read('xl/sharedStrings.xml')), 'si').map((si) => textOf(si)) : []
  // Styles: which are dates, percentages, euros…, and bold / italic.
  const styles = files['xl/styles.xml'] ? xml(read('xl/styles.xml')) : null
  const custom = new Map<number, string>()
  const fontsBI: { b: boolean; i: boolean }[] = []
  const xfs: { fmt: number; font: number }[] = []
  if (styles) {
    for (const f of all(styles, 'numFmt')) custom.set(Number(f.getAttribute('numFmtId')), f.getAttribute('formatCode') ?? '')
    const fonts = styles.getElementsByTagName('fonts')[0]
    if (fonts) for (const f of all(fonts, 'font')) fontsBI.push({ b: f.getElementsByTagName('b').length > 0, i: f.getElementsByTagName('i').length > 0 })
    const cellXfs = styles.getElementsByTagName('cellXfs')[0]
    if (cellXfs) for (const x of all(cellXfs, 'xf')) xfs.push({ fmt: Number(x.getAttribute('numFmtId') ?? 0), font: Number(x.getAttribute('fontId') ?? 0) })
  }
  const nfOf = (id: number): NumFmt | undefined => {
    if (DATE_FMTS.has(id)) return 'date'
    if (id === 9 || id === 10) return 'pct'
    if (id === 3) return 'int'
    if (id === 4 || id === 2) return 'num'
    const code = custom.get(id)
    if (!code) return undefined
    if (/€|\[\$€|EUR/.test(code)) return 'cur'
    if (/%/.test(code)) return 'pct'
    if (/(^|[^"\\])[dmy]{1,4}/i.test(code.replace(/"[^"]*"/g, '')) && !/0/.test(code)) return 'date'
    return undefined
  }
  let budget = maxCells
  const tabs: Tab[] = []
  for (const sh of all(wb, 'sheet')) {
    const rid = sh.getAttribute('r:id') ?? ''
    const path = (target.get(rid) ?? '').replace(/^\/?(xl\/)?/, 'xl/')
    const doc = files[path] ? xml(read(path)) : null
    const tab = newTab(sh.getAttribute('name') ?? `Sheet ${tabs.length + 1}`)
    tabs.push(tab)
    if (!doc) continue
    const masters = new Map<string, { formula: string; r: number; c: number }>() // shared formulas
    let maxR = 0
    let maxC = 0
    for (const c of all(doc, 'c')) {
      if (budget-- <= 0) break
      const a = c.getAttribute('r') ?? ''
      const p = parseAddr(a)
      if (!p) continue
      const t = c.getAttribute('t') ?? ''
      const s = Number(c.getAttribute('s') ?? 0)
      const xf = xfs[s]
      const nf = xf ? nfOf(xf.fmt) : undefined
      const font = xf ? fontsBI[xf.font] : undefined
      const f = c.getElementsByTagName('f')[0]
      const vEl = c.getElementsByTagName('v')[0]
      const raw = vEl?.textContent ?? ''
      let input = ''
      if (f) {
        let formula = f.textContent ?? ''
        const si = f.getAttribute('si')
        if (f.getAttribute('t') === 'shared' && si !== null) {
          if (formula) masters.set(si, { formula, r: p.r, c: p.c })
          else {
            const m = masters.get(si)
            if (m) formula = shiftFormula('=' + m.formula, p.r - m.r, p.c - m.c).slice(1)
          }
        }
        input = formula ? '=' + formula : raw
      } else if (t === 's') input = shared[Number(raw)] ?? ''
      else if (t === 'inlineStr') input = textOf(c.getElementsByTagName('is')[0])
      else if (t === 'b') input = raw === '1' ? 'TRUE' : 'FALSE'
      else if (t === 'str' || t === 'e') input = raw
      else if (raw !== '' && nf === 'date') input = isoOf(Math.round(Number(raw)) - EXCEL_EPOCH)
      else input = raw
      // Text that would read as a number or a formula stays text.
      if (!f && (t === 's' || t === 'inlineStr' || t === 'str') && /^[=+\-\d.,€$%']/.test(input)) input = "'" + input
      const cell: Cell = { v: input }
      if (nf && nf !== 'date') cell.nf = nf
      if (nf === 'date' && f) cell.nf = 'date'
      if (font?.b) cell.b = true
      if (font?.i) cell.i = true
      if (cell.v === '' && !cell.b && !cell.i && !cell.nf) continue
      tab.cells[a] = cell
      maxR = Math.max(maxR, p.r + 1)
      maxC = Math.max(maxC, p.c + 1)
    }
    tab.rows = Math.max(tab.rows, maxR + 20)
    tab.cols = Math.max(tab.cols, maxC + 2)
    const widths: Record<string, number> = {}
    for (const col of all(doc, 'col')) {
      const w = Number(col.getAttribute('width'))
      if (!w || col.getAttribute('customWidth') !== '1') continue
      const from = Number(col.getAttribute('min'))
      const to = Math.min(Number(col.getAttribute('max')), from + 50)
      for (let i = from; i <= to; i++) widths[colName(i - 1)] = Math.round(Math.min(Math.max(w * 7, 30), 600))
    }
    if (Object.keys(widths).length) tab.widths = widths
    const pane = doc.getElementsByTagName('pane')[0]
    if (pane?.getAttribute('state') === 'frozen' && Number(pane.getAttribute('ySplit')) > 0) tab.freeze = Math.min(Number(pane.getAttribute('ySplit')), 10)
  }
  if (!tabs.length) tabs.push(newTab('Sheet 1'))
  return { tabs }
}

// ---- CSV ----

// csvOf writes a tab's values as they're shown (as Excel saves CSV).
export function csvOf(tab: Tab, shown: (r: number, c: number) => string): string {
  let maxR = -1
  let maxC = -1
  for (const a of Object.keys(tab.cells)) {
    const p = parseAddr(a)
    if (p && tab.cells[a].v !== '') {
      maxR = Math.max(maxR, p.r)
      maxC = Math.max(maxC, p.c)
    }
  }
  const lines: string[] = []
  for (let r = 0; r <= maxR; r++) {
    const row: string[] = []
    for (let c = 0; c <= maxC; c++) {
      const s = shown(r, c)
      row.push(/[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s)
    }
    lines.push(row.join(','))
  }
  return '\ufeff' + lines.join('\r\n') + '\r\n'
}

// readCsv makes a one-tab sheet of a CSV (",", or ";" as Portuguese Excel
// writes it).
export function readCsv(text: string, name: string): SheetData {
  const s = text.replace(/^\ufeff/, '')
  const first = s.split('\n', 1)[0]
  const sep = (first.match(/;/g)?.length ?? 0) > (first.match(/,/g)?.length ?? 0) ? ';' : ','
  const tab = newTab(name)
  let r = 0
  let c = 0
  let cell = ''
  let quoted = false
  const put = () => {
    if (cell !== '') tab.cells[`${colName(c)}${r + 1}`] = { v: cell }
    cell = ''
  }
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]
    if (quoted) {
      if (ch === '"' && s[i + 1] === '"') {
        cell += '"'
        i++
      } else if (ch === '"') quoted = false
      else cell += ch
    } else if (ch === '"' && cell === '') quoted = true
    else if (ch === sep) {
      put()
      c++
    } else if (ch === '\n' || ch === '\r') {
      if (ch === '\r' && s[i + 1] === '\n') i++
      put()
      r++
      c = 0
    } else cell += ch
  }
  put()
  let maxC = 0
  for (const a of Object.keys(tab.cells)) maxC = Math.max(maxC, parseAddr(a)!.c)
  tab.rows = Math.max(tab.rows, r + 20)
  tab.cols = Math.max(tab.cols, maxC + 3)
  return { tabs: [tab] }
}

export { newId }

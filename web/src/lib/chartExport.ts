// Charts out of Orbit: as CSV (the numbers) or as a PNG picture (drawn on a
// canvas from the same numbers, light, with its title — not a screenshot, so
// it looks the same from any screen). One spec per chart feeds both.
import type { BarRow, Series } from '@/components/charts/types'

export type ChartSpec = { title: string; subtitle: string } & (
  | { kind: 'donut'; slices: { label: string; value: number; color: string }[]; center?: string }
  | { kind: 'bars'; series: Series[]; rows: BarRow[] }
  | { kind: 'columns'; series: Series[]; periods: { label: string; values: Record<string, number> }[] }
  | { kind: 'table'; cols: string[]; rows: { label: string; values: number[] }[]; hot?: number[] } // hot: columns tinted red
)

type Cell = string | number

// csvOf writes rows as CSV (RFC 4180: quoted when needed, CRLF), with a BOM
// so spreadsheet apps read it as UTF-8.
export function csvOf(rows: Cell[][]): string {
  const cell = (v: Cell) => {
    const s = String(v)
    return /[",\r\n]/.test(s) || /^\s|\s$/.test(s) ? `"${s.replace(/"/g, '""')}"` : s
  }
  return '﻿' + rows.map((r) => r.map(cell).join(',')).join('\r\n') + '\r\n'
}

// chartRows is a chart's numbers as a table, its header first.
export function chartRows(spec: ChartSpec, totalLabel: string): Cell[][] {
  switch (spec.kind) {
    case 'donut':
      return [['', spec.title], ...spec.slices.map((s) => [s.label, s.value])]
    case 'bars':
      return [
        ['', ...spec.series.map((s) => s.label), totalLabel],
        ...spec.rows.map((r) => {
          const vs = spec.series.map((s) => r.values[s.key] ?? 0)
          return [r.label, ...vs, vs.reduce((a, b) => a + b, 0)]
        }),
      ]
    case 'columns':
      return [['', ...spec.series.map((s) => s.label)], ...spec.periods.map((p) => [p.label, ...spec.series.map((s) => p.values[s.key] ?? 0)])]
    case 'table':
      return [['', ...spec.cols, totalLabel], ...spec.rows.map((r) => [r.label, ...r.values, r.values.reduce((a, b) => a + b, 0)])]
  }
}

// ---- The picture ----

// The light values of the chart colours (tokens.css), whatever the theme.
const LIGHT: Record<string, string> = {
  '--c-done': '#34c759', '--c-progress': '#007aff', '--c-not-started': '#8e8e93', '--c-late': '#ff3b30', '--c-created': '#5856d6',
  '--warn': '#ff9500', '--accent': '#007aff',
}
function paint(c: string): string {
  const m = /^var\((--[\w-]+)\)$/.exec(c.trim())
  if (!m) return c
  return LIGHT[m[1]] ?? (getComputedStyle(document.documentElement).getPropertyValue(m[1]).trim() || '#8e8e93')
}

const FONT = '-apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", system-ui, sans-serif'
const INK = '#1d1d1f'
const INK2 = '#6e6e73'
const LINE = 'rgba(60, 60, 67, 0.12)'
const PAD = 32
const W = 960

interface Ctx {
  g: CanvasRenderingContext2D
  y: number
}

function font(g: CanvasRenderingContext2D, size: number, weight = 400) {
  g.font = `${weight} ${size}px ${FONT}`
}
function fit(g: CanvasRenderingContext2D, s: string, max: number): string {
  if (g.measureText(s).width <= max) return s
  let t = s
  while (t.length > 1 && g.measureText(t + '…').width > max) t = t.slice(0, -1)
  return t + '…'
}
function legend(c: Ctx, items: { label: string; color: string; value?: number }[]) {
  const { g } = c
  font(g, 13)
  let x = PAD
  for (const it of items) {
    const text = it.value === undefined ? it.label : `${it.label} ${it.value}`
    const w = 14 + g.measureText(text).width + 22
    if (x + w > W - PAD) {
      x = PAD
      c.y += 22
    }
    g.fillStyle = paint(it.color)
    g.beginPath()
    g.arc(x + 5, c.y - 4, 5, 0, Math.PI * 2)
    g.fill()
    g.fillStyle = INK2
    g.fillText(text, x + 14, c.y)
    x += w
  }
  c.y += 26
}

function donut(c: Ctx, spec: Extract<ChartSpec, { kind: 'donut' }>) {
  const { g } = c
  const r = 110
  const cx = PAD + r + 10
  const cy = c.y + r
  const total = spec.slices.reduce((a, s) => a + s.value, 0)
  let a0 = -Math.PI / 2
  g.lineWidth = 34
  if (!total) {
    g.strokeStyle = LINE
    g.beginPath()
    g.arc(cx, cy, r - 17, 0, Math.PI * 2)
    g.stroke()
  }
  for (const s of spec.slices) {
    if (!s.value) continue
    const a1 = a0 + (s.value / total) * Math.PI * 2
    g.strokeStyle = paint(s.color)
    g.beginPath()
    g.arc(cx, cy, r - 17, a0, a1)
    g.stroke()
    a0 = a1
  }
  if (spec.center) {
    font(g, 30, 700)
    g.fillStyle = INK
    g.textAlign = 'center'
    g.fillText(spec.center, cx, cy + 10)
    g.textAlign = 'left'
  }
  // The legend beside it, one per line.
  let y = c.y + 40
  font(g, 15)
  for (const s of spec.slices) {
    g.fillStyle = paint(s.color)
    g.beginPath()
    g.arc(cx + r + 60, y - 5, 6, 0, Math.PI * 2)
    g.fill()
    g.fillStyle = INK
    g.fillText(s.label, cx + r + 76, y)
    font(g, 15, 600)
    g.fillText(String(s.value), cx + r + 260, y)
    font(g, 15)
    y += 30
  }
  c.y = Math.max(cy + r, y) + 24
}

function bars(c: Ctx, spec: Extract<ChartSpec, { kind: 'bars' }>) {
  const { g } = c
  const labelW = 200
  const x0 = PAD + labelW
  const maxW = W - PAD - x0 - 48
  const sum = (r: BarRow) => spec.series.reduce((a, s) => a + (r.values[s.key] ?? 0), 0)
  const max = Math.max(1, ...spec.rows.map(sum))
  for (const r of spec.rows) {
    font(g, 14)
    g.fillStyle = INK
    g.fillText(fit(g, r.label, labelW - 12), PAD, c.y + 15)
    let x = x0
    const total = sum(r)
    for (const s of spec.series) {
      const v = r.values[s.key] ?? 0
      if (!v) continue
      const w = (v / max) * maxW
      g.fillStyle = paint(s.color)
      g.fillRect(x, c.y + 2, Math.max(1, w - 1), 18)
      x += w
    }
    font(g, 13, 600)
    g.fillStyle = INK2
    g.fillText(String(total), x + 8, c.y + 16)
    c.y += 30
  }
  c.y += 10
  legend(c, spec.series)
}

function columns(c: Ctx, spec: Extract<ChartSpec, { kind: 'columns' }>) {
  const { g } = c
  const h = 220
  const x0 = PAD + 30
  const plotW = W - PAD - x0
  const max = Math.max(1, ...spec.periods.flatMap((p) => spec.series.map((s) => p.values[s.key] ?? 0)))
  const top = c.y
  // Grid: 0, half, max.
  font(g, 12)
  for (const f of [0, 0.5, 1]) {
    const y = top + h - f * h
    g.strokeStyle = LINE
    g.beginPath()
    g.moveTo(x0, y)
    g.lineTo(W - PAD, y)
    g.stroke()
    g.fillStyle = INK2
    g.textAlign = 'right'
    g.fillText(String(Math.round(f * max)), x0 - 8, y + 4)
  }
  g.textAlign = 'center'
  const slot = plotW / Math.max(1, spec.periods.length)
  const bw = Math.min(18, (slot - 10) / spec.series.length)
  spec.periods.forEach((p, i) => {
    const cx = x0 + slot * i + slot / 2
    spec.series.forEach((s, j) => {
      const v = p.values[s.key] ?? 0
      const bh = (v / max) * h
      g.fillStyle = paint(s.color)
      g.fillRect(cx - (bw * spec.series.length) / 2 + j * bw, top + h - bh, bw - 2, bh)
    })
    g.fillStyle = INK2
    g.fillText(fit(g, p.label, slot - 4), cx, top + h + 18)
  })
  g.textAlign = 'left'
  c.y = top + h + 46
  legend(c, spec.series)
}

function table(c: Ctx, spec: Extract<ChartSpec, { kind: 'table' }>) {
  const { g } = c
  const labelW = 200
  const colW = (W - PAD * 2 - labelW) / spec.cols.length
  const max = Math.max(1, ...spec.rows.flatMap((r) => r.values))
  font(g, 12, 600)
  g.fillStyle = INK2
  g.textAlign = 'center'
  spec.cols.forEach((col, i) => g.fillText(fit(g, col, colW - 6), PAD + labelW + colW * i + colW / 2, c.y + 12))
  c.y += 24
  for (const r of spec.rows) {
    g.textAlign = 'left'
    font(g, 14)
    g.fillStyle = INK
    g.fillText(fit(g, r.label, labelW - 12), PAD, c.y + 20)
    r.values.forEach((v, i) => {
      const x = PAD + labelW + colW * i
      const hot = spec.hot?.includes(i)
      g.fillStyle = v ? `rgba(${hot ? '255, 59, 48' : '0, 122, 255'}, ${0.12 + 0.5 * (v / max)})` : 'rgba(120, 120, 128, 0.06)'
      g.beginPath()
      g.roundRect(x + 3, c.y + 2, colW - 6, 28, 6)
      g.fill()
      if (v) {
        font(g, 14, 600)
        g.fillStyle = INK
        g.textAlign = 'center'
        g.fillText(String(v), x + colW / 2, c.y + 21)
      }
    })
    c.y += 34
  }
  g.textAlign = 'left'
  c.y += 12
}

function draw(g: CanvasRenderingContext2D | null, spec: ChartSpec): number {
  // Without a context (measuring), only the height matters: a generous one.
  const c: Ctx = { g: g ?? (document.createElement('canvas').getContext('2d') as CanvasRenderingContext2D), y: PAD + 26 }
  const gg = c.g
  font(gg, 22, 700)
  gg.fillStyle = INK
  gg.fillText(spec.title, PAD, c.y)
  c.y += 24
  font(gg, 13)
  gg.fillStyle = INK2
  gg.fillText(spec.subtitle, PAD, c.y)
  c.y += 34
  if (spec.kind === 'donut') donut(c, spec)
  else if (spec.kind === 'bars') bars(c, spec)
  else if (spec.kind === 'columns') columns(c, spec)
  else table(c, spec)
  font(gg, 11)
  gg.fillStyle = INK2
  gg.fillText('Orbit', PAD, c.y + 4)
  return c.y + 4 + PAD - 12
}

// chartPng draws the chart (twice the pixels, for sharp screens).
export async function chartPng(spec: ChartSpec): Promise<Blob> {
  const height = draw(null, spec) // a dry run, for the height
  const scale = 2
  const canvas = document.createElement('canvas')
  canvas.width = W * scale
  canvas.height = Math.ceil(height) * scale
  const g = canvas.getContext('2d')!
  g.scale(scale, scale)
  g.fillStyle = '#ffffff'
  g.fillRect(0, 0, W, height)
  draw(g, spec)
  return new Promise((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error('no picture'))), 'image/png'))
}

// saveBlob downloads b as name.
export function saveBlob(name: string, b: Blob) {
  const url = URL.createObjectURL(b)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

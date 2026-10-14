// Plain-data shapes the chart components take, so they stay reusable.

// A series: one colour in a stack or a group (e.g. a task status).
export interface Series {
  key: string
  label: string
  color: string // a CSS colour, usually var(--c-…)
}

// A row of a bar chart: a category and its value per series.
export interface BarRow {
  key: string
  label: string
  values: Record<string, number>
}

// A slice of a donut.
export interface Slice {
  key: string
  label: string
  value: number
  color: string
}

// The hover / focus readout: anchored at a viewport point.
export interface TipRow {
  color: string
  label: string
  value: number | string
  on?: boolean // the mark under the pointer
}
export interface Tip {
  x: number
  y: number
  title: string
  rows: TipRow[]
  hint?: string
}

// tipAnchor is where a readout for an element goes: above its centre (for
// keyboard focus, where there is no pointer).
export function tipAnchor(el: Element): { x: number; y: number } {
  const r = el.getBoundingClientRect()
  return { x: r.left + r.width / 2, y: r.top }
}

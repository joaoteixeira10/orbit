import { describe, it, expect } from 'vitest'
import { chartRows, csvOf } from './chartExport'

describe('chart export', () => {
  it('writes CSV that spreadsheets read', () => {
    expect(csvOf([['a', 'b, c'], ['say "hi"', 3], [' x', 'line\nbreak']])).toBe('﻿a,"b, c"\r\n"say ""hi""",3\r\n" x","line\nbreak"\r\n')
  })
  it('turns each kind of chart into a table', () => {
    const series = [{ key: 'done', label: 'Done', color: '' }, { key: 'late', label: 'Late', color: '' }]
    expect(chartRows({ kind: 'bars', title: 'By list', subtitle: '', series, rows: [{ key: 'l1', label: 'Home', values: { done: 2, late: 1 } }] }, 'Total'))
      .toEqual([['', 'Done', 'Late', 'Total'], ['Home', 2, 1, 3]])
    expect(chartRows({ kind: 'donut', title: 'Status', subtitle: '', slices: [{ label: 'Done', value: 4, color: '' }] }, 'Total')).toEqual([['', 'Status'], ['Done', 4]])
    expect(chartRows({ kind: 'columns', title: 'Trend', subtitle: '', series, periods: [{ label: '5 Oct', values: { done: 1 } }] }, 'Total')).toEqual([['', 'Done', 'Late'], ['5 Oct', 1, 0]])
    expect(chartRows({ kind: 'table', title: 'Workload', subtitle: '', cols: ['Late', 'Today'], rows: [{ label: 'Ana', values: [1, 2] }] }, 'Total')).toEqual([['', 'Late', 'Today', 'Total'], ['Ana', 1, 2, 3]])
  })
})

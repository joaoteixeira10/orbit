import { describe, it, expect } from 'vitest'
import { EditorSelection, EditorState, type StateCommand } from '@codemirror/state'
import { cursorsAtLineEnds, insertLineAbove, link, wrapWith } from './vscode'

// run applies a command to a text with selections ([from, to] pairs).
function run(cmd: StateCommand, doc: string, ranges: [number, number][]) {
  let state = EditorState.create({ doc, selection: EditorSelection.create(ranges.map(([a, b]) => EditorSelection.range(a, b))), extensions: EditorState.allowMultipleSelections.of(true) })
  const ok = cmd({ state, dispatch: (tr) => (state = tr.state) })
  return { ok, doc: state.doc.toString(), sel: state.selection.ranges.map((r) => [r.from, r.to]) }
}

describe('VS Code commands', () => {
  it('puts a cursor at the end of each selected line (Shift-Alt-I)', () => {
    const r = run(cursorsAtLineEnds, 'one\ntwo\nthree', [[1, 9]])
    expect(r.sel).toEqual([[3, 3], [7, 7], [13, 13]])
  })
  it('opens a line above, keeping the indent (Mod-Shift-Enter)', () => {
    const r = run(insertLineAbove, 'a\n  b', [[4, 4]])
    expect(r.doc).toBe('a\n  \n  b')
    expect(r.sel).toEqual([[4, 4]])
  })
  it('wraps every selection at once, and unwraps (Mod-B)', () => {
    const r = run(wrapWith('**'), 'red green', [[0, 3], [4, 9]])
    expect(r.doc).toBe('**red** **green**')
    expect(run(wrapWith('**'), '**red**', [[0, 7]]).doc).toBe('red')
    expect(run(wrapWith('`'), 'x', [[1, 1]]).sel).toEqual([[2, 2]])
  })
  it('links the selection with the address ready to type (Mod-K)', () => {
    const r = run(link, 'see docs', [[4, 8]])
    expect(r.doc).toBe('see [docs](https://)')
    expect(r.sel).toEqual([[11, 19]])
  })
})

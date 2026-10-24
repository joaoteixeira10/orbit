// What Visual Studio Code does in a text, for CodeMirror: the commands
// CodeMirror lacks, and the keymap that binds them (with CodeMirror's own)
// to VS Code's keys. Mod is ⌘ on a Mac, Ctrl elsewhere.
//
//   Alt-click / ⌥-click           another cursor
//   Shift-Alt-drag                a box (column) selection
//   Mod-D                         add the next occurrence of the selection
//   Mod-Shift-L                   select every occurrence
//   Mod-Alt-↑ / ↓                 a cursor on the line above / below
//   Shift-Alt-I                   a cursor at the end of each selected line
//   Alt-↑ / ↓                     move the line(s)
//   Shift-Alt-↑ / ↓               copy the line(s) up / down
//   Mod-Shift-K                   delete the line(s)
//   Mod-L                         select the line (again: the next)
//   Mod-Enter / Mod-Shift-Enter   a new line below / above
//   Mod-] / Mod-[                 indent / outdent
//   Mod-/                         comment (in Markdown <!-- -->)
//   Mod-U                         undo the last cursor change
//   Escape                        back to one cursor
//   Mod-F, Mod-Alt-F, F3          find (and replace), next; Ctrl-G go to line
//   Mod-C / Mod-X with nothing selected: copy / cut the whole line
import { EditorSelection, type EditorState, type SelectionRange, type StateCommand } from '@codemirror/state'
import { EditorView, type Command, type KeyBinding } from '@codemirror/view'
import { defaultKeymap, historyKeymap, indentWithTab, selectLine } from '@codemirror/commands'
import { gotoLine, openSearchPanel, searchKeymap } from '@codemirror/search'

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent)

// addCursor puts a cursor on the line above (-1) or below (1) each one, at
// the same column (or the line's end, when it's shorter).
function addCursor(dir: -1 | 1): Command {
  return (view) => {
    const { state } = view
    const ranges = [...state.selection.ranges]
    for (const r of state.selection.ranges) {
      const line = state.doc.lineAt(r.head)
      const n = line.number + dir
      if (n < 1 || n > state.doc.lines) continue
      const target = state.doc.line(n)
      const col = r.head - line.from
      const pos = target.from + Math.min(col, target.length)
      ranges.push(EditorSelection.cursor(pos))
    }
    if (ranges.length === state.selection.ranges.length) return false
    view.dispatch({ selection: EditorSelection.create(ranges, ranges.length - 1), scrollIntoView: true, userEvent: 'select' })
    return true
  }
}

// cursorsAtLineEnds: a cursor at the end of each line the selections cover.
export const cursorsAtLineEnds: StateCommand = ({ state, dispatch }) => {
  const out: SelectionRange[] = []
  for (const r of state.selection.ranges) {
    const first = state.doc.lineAt(r.from).number
    const last = state.doc.lineAt(r.to).number
    for (let n = first; n <= last; n++) out.push(EditorSelection.cursor(state.doc.line(n).to))
  }
  if (out.length <= 1 && state.selection.ranges.length === 1 && state.selection.main.empty) return false
  dispatch(state.update({ selection: EditorSelection.create(out), userEvent: 'select' }))
  return true
}

// insertLineAbove: a new, empty line above each cursor's line, the cursor
// on it (keeping the line's indentation).
export const insertLineAbove: StateCommand = ({ state, dispatch }) => {
  const changes = state.changeByRange((r) => {
    const line = state.doc.lineAt(r.head)
    const indent = /^\s*/.exec(line.text)![0]
    return { changes: { from: line.from, insert: indent + state.lineBreak }, range: EditorSelection.cursor(line.from + indent.length) }
  })
  dispatch(state.update(changes, { scrollIntoView: true, userEvent: 'input' }))
  return true
}

// wrapWith puts marks around each selection (bold, italic, code, a link);
// an empty one gets the marks with the cursor between.
export function wrapWith(open: string, close = open): StateCommand {
  return ({ state, dispatch }) => {
    if (state.readOnly) return false
    const changes = state.changeByRange((r) => {
      const text = state.sliceDoc(r.from, r.to)
      // Already wrapped: unwrap.
      if (text.startsWith(open) && text.endsWith(close) && text.length >= open.length + close.length) {
        const inner = text.slice(open.length, text.length - close.length)
        return { changes: { from: r.from, to: r.to, insert: inner }, range: EditorSelection.range(r.from, r.from + inner.length) }
      }
      return {
        changes: [
          { from: r.from, insert: open },
          { from: r.to, insert: close },
        ],
        range: r.empty ? EditorSelection.cursor(r.from + open.length) : EditorSelection.range(r.from + open.length, r.to + open.length),
      }
    })
    dispatch(state.update(changes, { userEvent: 'input' }))
    return true
  }
}

// link: [selection](https://) with "https://" selected to type over.
export const link: StateCommand = ({ state, dispatch }) => {
  if (state.readOnly) return false
  const url = 'https://'
  const changes = state.changeByRange((r) => {
    const text = state.sliceDoc(r.from, r.to)
    const insert = `[${text}](${url})`
    const at = r.from + text.length + 3
    return { changes: { from: r.from, to: r.to, insert }, range: r.empty ? EditorSelection.cursor(r.from + 1) : EditorSelection.range(at, at + url.length) }
  })
  dispatch(state.update(changes, { userEvent: 'input' }))
  return true
}

// lineRanges: with nothing selected, copy and cut take whole lines (as in
// VS Code). Answers the text, and the ranges to cut.
function lineRanges(state: EditorState): { text: string; ranges: { from: number; to: number }[] } | null {
  if (state.selection.ranges.some((r) => !r.empty)) return null
  const seen = new Set<number>()
  const lines: { from: number; to: number }[] = []
  for (const r of state.selection.ranges) {
    const line = state.doc.lineAt(r.head)
    if (seen.has(line.number)) continue
    seen.add(line.number)
    lines.push({ from: line.from, to: Math.min(line.to + 1, state.doc.length) })
  }
  lines.sort((a, b) => a.from - b.from)
  const text = lines.map((l) => state.doc.lineAt(l.from).text).join(state.lineBreak) + state.lineBreak
  return { text, ranges: lines }
}
export const copyCutLines = EditorView.domEventHandlers({
  copy(e, view) {
    const r = lineRanges(view.state)
    if (!r || !e.clipboardData) return false
    e.preventDefault()
    e.clipboardData.setData('text/plain', r.text)
    return true
  },
  cut(e, view) {
    const r = lineRanges(view.state)
    if (!r || !e.clipboardData || view.state.readOnly) return false
    e.preventDefault()
    e.clipboardData.setData('text/plain', r.text)
    view.dispatch({ changes: r.ranges, userEvent: 'delete.cut' })
    return true
  },
})

// vsKeymap: VS Code's keys first, then CodeMirror's (which already has many
// of the same). submit: what Mod-Enter does instead of a new line (a note
// saves with it).
export function vsKeymap(opts: { submit?: () => void; extra?: KeyBinding[] } = {}): KeyBinding[] {
  return [
    ...(opts.extra ?? []),
    { key: 'Mod-Alt-ArrowUp', run: addCursor(-1), preventDefault: true },
    { key: 'Mod-Alt-ArrowDown', run: addCursor(1), preventDefault: true },
    { key: 'Shift-Alt-i', run: cursorsAtLineEnds, preventDefault: true },
    { key: 'Mod-l', run: selectLine, preventDefault: true },
    { key: 'Mod-Shift-Enter', run: insertLineAbove, preventDefault: true },
    ...(opts.submit ? [{ key: 'Mod-Enter', run: () => (opts.submit!(), true), preventDefault: true }] : []),
    { key: 'Mod-b', run: wrapWith('**'), preventDefault: true },
    { key: 'Mod-i', run: wrapWith('*'), preventDefault: true },
    { key: 'Mod-e', run: wrapWith('`'), preventDefault: true },
    { key: 'Mod-k', run: link, preventDefault: true },
    { key: 'Mod-Alt-f', run: openSearchPanel, preventDefault: true },
    ...(isMac ? [] : [{ key: 'Ctrl-h', run: openSearchPanel, preventDefault: true }]),
    { key: 'Ctrl-g', run: gotoLine, preventDefault: true },
    ...searchKeymap,
    ...defaultKeymap,
    ...historyKeymap,
    indentWithTab,
  ]
}

// VS Code's mouse: Alt-click adds a cursor (Mod-click too, CodeMirror's
// way); Shift-Alt-drag selects a box.
export const addsCursor = (e: MouseEvent) => e.altKey || (isMac ? e.metaKey : e.ctrlKey)
export const boxSelects = (e: MouseEvent) => e.altKey && e.shiftKey

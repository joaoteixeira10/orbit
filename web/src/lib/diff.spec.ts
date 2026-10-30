import { describe, expect, it } from 'vitest'
import { blocks, changeCount, diff, diffMarkdown } from './diff'

describe('diff', () => {
  it('splits Markdown into blocks', () => {
    const md = '# Title\n\nA paragraph\nthat wraps.\n\n- one\n- two\n\n```js\nlet a = 1\n\nlet b = 2\n```\n\n| a | b |\n| - | - |'
    expect(blocks(md)).toEqual(['# Title', 'A paragraph\nthat wraps.', '- one', '- two', '```js\nlet a = 1\n\nlet b = 2\n```', '| a | b |', '| - | - |'])
    expect(blocks('')).toEqual([])
  })

  it('lines up what stayed, what came and what went', () => {
    expect(diff(['a', 'b', 'c'], ['a', 'x', 'c', 'd'])).toEqual([
      { op: 'same', text: 'a' },
      { op: 'del', text: 'b' },
      { op: 'add', text: 'x' },
      { op: 'same', text: 'c' },
      { op: 'add', text: 'd' },
    ])
    const ops = diffMarkdown('# Plan\n\n- milk\n- eggs', '# Plan\n\n- milk\n- bread\n- eggs\n\nDone.')
    expect(changeCount(ops)).toEqual({ added: 2, removed: 0 })
    expect(ops.filter((o) => o.op === 'add').map((o) => o.text)).toEqual(['- bread', 'Done.'])
  })
})

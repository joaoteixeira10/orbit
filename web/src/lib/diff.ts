// Changes between two versions of a page's Markdown, block by block (a
// paragraph, a heading, a code block, each line of a list or a table), for
// showing what changed in place: kept, added, removed.

export interface DiffOp {
  op: 'same' | 'add' | 'del'
  text: string
}

const ITEM = /^\s*(?:[-*+]\s|\d+[.)]\s|\||>)/

// blocks splits Markdown into the pieces compared: blank lines part them, a
// fenced code block is one, and a list, a table or a quote goes line by line.
export function blocks(md: string): string[] {
  const out: string[] = []
  let cur: string[] = []
  let fence = ''
  const flush = () => {
    if (!cur.length) return
    if (cur.every((l) => ITEM.test(l))) out.push(...cur)
    else out.push(cur.join('\n'))
    cur = []
  }
  for (const line of md.replace(/\r\n/g, '\n').split('\n')) {
    const f = /^\s*(```+|~~~+)/.exec(line)?.[1]
    if (fence) {
      cur.push(line)
      if (f && f[0] === fence[0] && f.length >= fence.length) {
        out.push(cur.join('\n'))
        cur = []
        fence = ''
      }
      continue
    }
    if (f) {
      flush()
      fence = f
      cur.push(line)
      continue
    }
    if (!line.trim()) flush()
    else cur.push(line)
  }
  flush()
  return out
}

// diff lines up two lists (longest common subsequence): what's in both,
// what only b has (added), what only a had (removed) — removals before the
// additions that replace them.
export function diff(a: string[], b: string[]): DiffOp[] {
  const n = a.length
  const m = b.length
  const L: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) L[i][j] = a[i] === b[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1])
  const out: DiffOp[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ op: 'same', text: a[i] })
      i++
      j++
    } else if (L[i + 1][j] >= L[i][j + 1]) out.push({ op: 'del', text: a[i++] })
    else out.push({ op: 'add', text: b[j++] })
  }
  while (i < n) out.push({ op: 'del', text: a[i++] })
  while (j < m) out.push({ op: 'add', text: b[j++] })
  return out
}

export const diffMarkdown = (before: string, after: string) => diff(blocks(before), blocks(after))

// changeCount: how many blocks were added and removed.
export function changeCount(ops: DiffOp[]): { added: number; removed: number } {
  return { added: ops.filter((o) => o.op === 'add').length, removed: ops.filter((o) => o.op === 'del').length }
}

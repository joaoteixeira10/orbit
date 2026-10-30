// Glossaries: a page marked as one lists terms and what they mean — a line
// "**Term** — what it means" (or "**Term**: …", in a list or not), or a
// heading "### Term" with its first paragraph under it. Where the
// workspace's other pages use a term, it's marked, and pointing at it
// explains it.

export interface Term {
  term: string
  definition: string
  pageId: string
  pageTitle: string
}

const BOLD_LINE = /^\s*(?:[-*+]\s+)?\*\*([^*]{1,60})\*\*\s*(?:[—–:-]|\s-\s)\s*(.+)$/
const HEADING = /^#{2,4}\s+(.{1,60}?)\s*#*\s*$/

// termsOf reads a glossary page's terms.
export function termsOf(md: string, pageId = '', pageTitle = ''): Term[] {
  const out: Term[] = []
  const lines = md.replace(/\r\n/g, '\n').split('\n')
  let fence = false
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    if (/^\s*(```|~~~)/.test(line)) {
      fence = !fence
      continue
    }
    if (fence) continue
    const b = BOLD_LINE.exec(line)
    if (b) {
      out.push({ term: b[1].trim(), definition: b[2].trim(), pageId, pageTitle })
      continue
    }
    const h = HEADING.exec(line)
    if (h) {
      let j = i + 1
      while (j < lines.length && !lines[j].trim()) j++
      const next = lines[j] ?? ''
      if (next.trim() && !/^\s*(#|```|~~~|>|\||[-*+]\s)/.test(next)) out.push({ term: h[1].trim(), definition: next.trim(), pageId, pageTitle })
    }
  }
  // Plain text: no Markdown marks in what the card says.
  for (const t of out) t.definition = t.definition.replace(/\*\*|__|`/g, '').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
  return out.filter((t) => t.term.length >= 2)
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// markTerms wraps the terms where they're used in root's text (not in
// links, code, headings, inputs or what's shown in place): a span .md-term
// with the term's number (data-term).
export function markTerms(root: HTMLElement, terms: Term[]): number {
  if (!terms.length) return 0
  const sorted = terms.map((t, i) => ({ t, i })).sort((a, b) => b.t.term.length - a.t.term.length)
  const re = new RegExp(`(?<![\\p{L}\\p{N}])(${sorted.map((x) => escape(x.t.term)).join('|')})(?![\\p{L}\\p{N}])`, 'giu')
  const index = new Map(sorted.map((x) => [x.t.term.toLowerCase(), x.i]))
  const skip = 'a, code, pre, h1, h2, h3, h4, h5, h6, .md-term, .page-embed, input, button, summary'
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) => (n.parentElement?.closest(skip) || !n.nodeValue?.trim() ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  })
  const nodes: Text[] = []
  for (let n = walker.nextNode(); n; n = walker.nextNode()) nodes.push(n as Text)
  let marked = 0
  for (const node of nodes) {
    const text = node.nodeValue ?? ''
    re.lastIndex = 0
    if (!re.test(text)) continue
    re.lastIndex = 0
    const frag = document.createDocumentFragment()
    let last = 0
    for (const m of text.matchAll(re)) {
      const at = m.index ?? 0
      if (at > last) frag.append(text.slice(last, at))
      const span = document.createElement('span')
      span.className = 'md-term'
      span.tabIndex = 0
      span.dataset.term = String(index.get(m[0].toLowerCase()) ?? 0)
      span.textContent = m[0]
      frag.append(span)
      last = at + m[0].length
      marked++
    }
    if (last < text.length) frag.append(text.slice(last))
    node.replaceWith(frag)
  }
  return marked
}

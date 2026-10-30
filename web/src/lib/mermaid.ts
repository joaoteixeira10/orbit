// Mermaid diagrams: text drawn as a diagram (mermaid.js, loaded the first
// time one is drawn), the templates a new one starts from, and the edits the
// drawing makes to the text — "+ Node" and "Connect" on a flowchart.

export type MermaidKind = 'flowchart' | 'sequence' | 'class' | 'state' | 'er' | 'gantt' | 'timeline' | 'pie' | 'mindmap'

export const MERMAID_TEMPLATES: { key: MermaidKind; code: string }[] = [
  {
    key: 'flowchart',
    code: `flowchart TD
  A[Start] --> B{Is it working?}
  B -- Yes --> C[Great]
  B -- No --> D[Fix it]
  D --> B`,
  },
  {
    key: 'sequence',
    code: `sequenceDiagram
  participant A as Alice
  participant B as Bob
  A->>B: Hello Bob
  B-->>A: Hi Alice`,
  },
  {
    key: 'class',
    code: `classDiagram
  class Animal {
    +String name
    +move()
  }
  class Dog {
    +bark()
  }
  Animal <|-- Dog`,
  },
  {
    key: 'state',
    code: `stateDiagram-v2
  [*] --> Draft
  Draft --> Review
  Review --> Published
  Review --> Draft
  Published --> [*]`,
  },
  {
    key: 'er',
    code: `erDiagram
  CUSTOMER ||--o{ ORDER : places
  ORDER ||--|{ LINE : contains
  PRODUCT ||--o{ LINE : "ordered in"`,
  },
  {
    key: 'gantt',
    code: `gantt
  dateFormat YYYY-MM-DD
  section Plan
  Research   :a1, 2026-01-05, 7d
  Design     :a2, after a1, 5d
  section Build
  Build      :b1, after a2, 10d`,
  },
  {
    key: 'timeline',
    code: `timeline
  2024 : First idea
  2025 : Prototype
       : First users
  2026 : Launch`,
  },
  {
    key: 'pie',
    code: `pie
  "Work" : 45
  "Sleep" : 35
  "Play" : 20`,
  },
  {
    key: 'mindmap',
    code: `mindmap
  root((Project))
    Goals
      Faster
      Simpler
    People
      Team
      Users`,
  },
]

// kindOf is the kind of diagram the text is (its first word), or "".
export function kindOf(code: string): MermaidKind | '' {
  for (const raw of code.split('\n')) {
    const line = raw.trim()
    if (!line || line.startsWith('%%') || line === '---') continue
    const w = line.split(/\s/)[0].toLowerCase()
    if (w === 'flowchart' || w === 'graph') return 'flowchart'
    if (w === 'sequencediagram') return 'sequence'
    if (w === 'classdiagram') return 'class'
    if (w.startsWith('statediagram')) return 'state'
    if (w === 'erdiagram') return 'er'
    if (w === 'gantt' || w === 'timeline' || w === 'pie' || w === 'mindmap') return w
    return ''
  }
  return ''
}

const RESERVED = new Set(['flowchart', 'graph', 'subgraph', 'end', 'direction', 'TD', 'TB', 'LR', 'RL', 'BT'])

// flowNodes are the ids of a flowchart's nodes, in order of appearance: each
// statement without its labels ([…], (…), {…}, "…", |…|, an arrow's text)
// and arrows, leaving the ids.
export function flowNodes(code: string): string[] {
  const out: string[] = []
  for (const raw of code.split('\n')) {
    let line = raw.replace(/%%.*$/, '').trim()
    if (!line || /^(flowchart|graph|classDef|class|style|linkStyle|click|direction|subgraph|end)\b/.test(line)) continue
    let prev = ''
    while (prev !== line) {
      prev = line
      line = line.replace(/"[^"]*"|\[[^[\]]*\]|\([^()]*\)|\{[^{}]*\}|\|[^|]*\|/g, ' ')
    }
    line = line
      .replace(/:::[\w-]+/g, ' ')
      .replace(/(--|==|-\.)\s+[^-=.][^>]*?\s+(-->|==>|\.->)/g, ' $2 ')
      .replace(/(?:<|\bo|\bx)?[-=.]{2,}(?:>|o\b|x\b)?/g, ' ')
    for (const m of line.matchAll(/[A-Za-z_][\w-]*/g)) {
      const id = m[0]
      if (!RESERVED.has(id) && !out.includes(id)) out.push(id)
    }
  }
  return out
}

// nextNodeId is a node id not used yet (N1, N2…).
export function nextNodeId(code: string): string {
  const used = new Set(flowNodes(code))
  let i = 1
  while (used.has(`N${i}`)) i++
  return `N${i}`
}

// A label as Mermaid reads it inside [ ]: quoted when it has characters
// Mermaid would take as syntax.
export function quoteLabel(label: string): string {
  const s = label.replace(/"/g, '#quot;').trim()
  return /[[\](){}<>|;:#&]/.test(s) ? `"${s}"` : s
}

// addNode adds a box (connected from `from`, when given) at the end.
export function addNode(code: string, label: string, from?: string): { code: string; id: string } {
  const id = nextNodeId(code)
  const box = `${id}[${quoteLabel(label || id)}]`
  const line = from ? `  ${from} --> ${box}` : `  ${box}`
  return { code: code.replace(/\s*$/, '') + '\n' + line, id }
}

// connect adds an arrow from one node to another (with a label, if given).
export function connect(code: string, a: string, b: string, label = ''): string {
  const arrow = label.trim() ? `-- ${label.replace(/[-|]/g, ' ').trim()} -->` : '-->'
  return code.replace(/\s*$/, '') + `\n  ${a} ${arrow} ${b}`
}

// labelsOf is the words in a diagram, for search: its text without the
// syntax (arrows, brackets, keywords).
export function labelsOf(code: string): string {
  return code
    .split('\n')
    .slice(1)
    .map((l) => l.replace(/%%.*$/, '').replace(/[-=.<>|o*x{}()[\]"#:;&+]+/g, ' ').replace(/\s+/g, ' ').trim())
    .filter(Boolean)
    .join(' ')
    .slice(0, 200_000)
}

type Mermaid = typeof import('mermaid').default
let lib: Promise<Mermaid> | null = null
function load(): Promise<Mermaid> {
  lib ??= import('mermaid').then((m) => {
    m.default.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      theme: 'default',
      // Labels as SVG text, not HTML, so the picture also draws as an <img>.
      htmlLabels: false,
      flowchart: { htmlLabels: false },
      fontFamily: '-apple-system, BlinkMacSystemFont, "SF Pro Text", "Helvetica Neue", Arial, sans-serif',
    })
    return m.default
  })
  return lib
}

// naturalSize gives Mermaid's SVG (as wide as its box) its own size, so a
// picture of it isn't blown up to fill a page.
export function naturalSize(svg: string): string {
  const vb = /<svg\b[^>]*\bviewBox="([\d.\s-]+)"/.exec(svg)?.[1].trim().split(/\s+/).map(Number)
  if (!vb || vb.length !== 4) return svg
  return svg.replace(/<svg\b[^>]*>/, (tag) =>
    tag
      .replace(/\swidth="[^"]*"/, '')
      .replace(/\sheight="[^"]*"/, '')
      .replace(/\sstyle="[^"]*"/, '')
      .replace(/^<svg/, `<svg width="${Math.ceil(vb[2])}" height="${Math.ceil(vb[3])}"`),
  )
}

let n = 0
// render draws the text as an SVG, or says what's wrong with it.
export async function renderMermaid(code: string): Promise<{ svg: string; error?: undefined } | { svg?: undefined; error: string }> {
  if (!code.trim()) return { error: '' }
  const m = await load()
  try {
    await m.parse(code)
    const { svg } = await m.render(`orbit-mmd-${++n}`, code)
    return { svg: naturalSize(svg) }
  } catch (e: any) {
    // Mermaid leaves its failed attempt in the page.
    document.getElementById(`dorbit-mmd-${n}`)?.remove()
    return { error: String(e?.message ?? e).split('\n').slice(0, 6).join('\n') }
  }
}

// pngOf draws an SVG as a PNG (scale times its size, on white).
export async function pngOf(svg: string, scale = 2): Promise<Blob> {
  const vb = /viewBox="([\d.\s-]+)"/.exec(svg)?.[1].trim().split(/\s+/).map(Number)
  const w = Math.ceil(vb?.[2] ?? 800)
  const h = Math.ceil(vb?.[3] ?? 600)
  const img = new Image()
  img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(naturalSize(svg))}`
  await img.decode()
  const canvas = document.createElement('canvas')
  canvas.width = w * scale
  canvas.height = h * scale
  const g = canvas.getContext('2d')!
  g.fillStyle = '#ffffff'
  g.fillRect(0, 0, canvas.width, canvas.height)
  g.drawImage(img, 0, 0, canvas.width, canvas.height)
  return new Promise((ok, fail) => canvas.toBlob((b) => (b ? ok(b) : fail(new Error('no picture'))), 'image/png'))
}

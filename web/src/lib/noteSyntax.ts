// How a quick note's text is coloured as it's written, as an editor (VS
// Code) would: as Markdown, as plain text, or as code in one language —
// chosen on the note, or guessed from its text. noteTokens are the coloured
// stretches, the same for the editor (CodeMirror marks) and the note's card
// (HTML), so both look alike. Colours: syntax.css (code) and .md-* below it.
import type { ElementContent, Root, RootContent } from 'hast'
import { LANGUAGES, detectLanguage, labelOf, languageOf, lowlight } from './syntax'

// The code languages a note can be (Markdown is a choice of its own).
export const CODE_LANGS = LANGUAGES.filter((l) => l.id !== 'markdown')

// Markdown: a line that is structure, or marks inside one.
const MD_LINE = /^(#{1,6}\s|\s*[-*+•]\s|\s*\d{1,9}[.)]\s|>\s?|\s*\[[ xX]\]\s)/
const MD_INLINE = /\*\*[^*\n]+\*\*|`[^`\n]+`|\[[^\]\n]+\]\([^)\n]+\)/
const FENCE = /^\s*(`{3,}|~{3,})\s*([\w+#.-]*)/
// Only the start of a long note is read to guess.
const SAMPLE = 4000

// The signs of a language people paste into notes. line: a line that looks
// like it (lines starting with "#" — comments — don't count either way);
// share: how many of the lines must; whole: a sign in the whole text it
// also needs (or, with no line rule, that settles it).
interface Rule {
  id: string
  line?: RegExp
  share?: number
  whole?: RegExp
}
const RULES: Rule[] = [
  { id: 'go', whole: /^package \w+\s*$/m },
  { id: 'php', whole: /^<\?php/ },
  { id: 'diff', whole: /^(diff --git |--- a\/|@@ -\d)/m },
  { id: 'dockerfile', line: /^(FROM|RUN|COPY|ADD|WORKDIR|ENV|EXPOSE|CMD|ENTRYPOINT|ARG|LABEL|USER|VOLUME|HEALTHCHECK|SHELL)\s/, share: 0.6, whole: /^FROM\s/m },
  {
    id: 'sql',
    line: /^\s*(SELECT|INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+(TABLE|INDEX|VIEW|DATABASE)|ALTER\s+TABLE|DROP\s+TABLE|WITH\s+\w+\s+AS|FROM|WHERE|AND|OR|ORDER\s+BY|GROUP\s+BY|LIMIT|JOIN|LEFT|VALUES|SET)\b/i,
    share: 0.6,
    whole: /^\s*(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|WITH)\b/i,
  },
  { id: 'nginx', line: /^\s*([a-z_]+\s+[^\n]*;|[a-z_]+[^\n]*\{|\})\s*$/, share: 0.8, whole: /^\s*(server|location|upstream|http|events)\b[^\n{]*\{/m },
  { id: 'rust', whole: /\bfn main\(\)|^use std::|\blet mut \w/m },
  { id: 'python', line: /^\s*(def \w+\(|class \w+[(:]|import \w|from [\w.]+ import |if __name__|elif\b|else:|try:|except\b|print\(|return\b|for \w+ in .+:$|while .+:$|with .+:$|@\w+|\w+\s*=\s*.+|\w+\(.*\)$)/, share: 0.7, whole: /^\s*(def |import |from [\w.]+ import |class \w+.*:$)/m },
  { id: 'typescript', line: /^\s*(const|let|var|function|export|import|class|interface|type|async|return|if \(|for \(|\}|\)|console\.|\w+\(.*\);?$|\w+\.\w+|\w+\??:\s*[\w<>[\]|' ]+[;,]?$)/, share: 0.7, whole: /(:\s*(string|number|boolean)\b|\binterface \w+|\btype \w+ =)/ },
  { id: 'javascript', line: /^\s*(const|let|var|function|export|import|class|async|return|if \(|for \(|\}|\)|console\.|\w+\(.*\);?$|\w+\.\w+)/, share: 0.7, whole: /^\s*(const|let|function|import|export)\b/m },
  { id: 'powershell', line: /^\s*(\$\w+\s*=|(Get|Set|New|Remove|Add|Invoke|Start|Stop|Import|Export|Write|Test)-[A-Z]\w+)/, share: 0.5 },
  { id: 'ini', line: /^\s*(\[[^\]\n]+\]|[\w.-]+\s*=.*)$/, share: 0.8, whole: /^\[[^\]\n]+\]\s*$/m },
  // YAML's keys are lower-case ("Room: 3" is a note, not YAML).
  { id: 'yaml', line: /^\s*(- |[a-z_][\w.-]*:(\s|$)|"[^"]+":\s)/, share: 0.8, whole: /^\s*[a-z_][\w.-]*:(\s|$)[\s\S]*^\s*[a-z_][\w.-]*:(\s|$)/m },
  {
    id: 'bash',
    line: /^\s*(\$ |sudo |cd |ls\b|echo |export \w+=|docker |docker-compose |kubectl |helm |git |curl |wget |apt(-get)? |yum |dnf |brew |npm |npx |yarn |pnpm |make\b|go |vault |systemctl |journalctl |ssh |scp |rsync |cp |mv |rm |chmod |chown |mkdir |cat |grep |sed |awk |tar |for \w+ in |if \[|then$|fi$|do$|done$|source |\.\/|terraform |ansible|openssl |psql |mysql |pip3? |python3? |node )/,
    share: 0.7,
  },
  { id: 'css', line: /^\s*([.#]?[\w-][^{\n]*\{|[\w-]+\s*:\s*[^;\n]+;|\})\s*$/, share: 0.8, whole: /\{[\s\S]*:[^;\n]+;/ },
]

// signsOf is the language whose signs most of the lines show (and at least
// `least` lines), or ''.
function signsOf(s: string, lines: string[], least: number): string {
  // Comments ("# …", "// …") say nothing either way.
  const body = lines.filter((l) => !/^\s*(#(?!!)|\/\/)/.test(l))
  for (const r of RULES) {
    if (r.whole && !r.whole.test(s)) continue
    if (!r.line) return r.id
    const n = body.filter((l) => r.line!.test(l)).length
    if (n >= least && n >= body.length * (r.share ?? 0.6)) return r.id
  }
  return ''
}

// blockLang is the language of a ``` block that doesn't say: its signs (one
// line is enough in a block), else highlight.js's guess when sure; '' = plain.
export function blockLang(code: string): string {
  const lines = code.split('\n').filter((l) => l.trim())
  if (!lines.length) return ''
  const signs = signsOf(code, lines, 1)
  if (signs) return signs
  const d = detectLanguage(code)
  return d.language && d.relevance >= 4 ? d.language : ''
}

// guessNoteLang reads a note's text: "markdown" with ``` blocks or
// Markdown's marks, a code language when the whole of it clearly is one (a
// shebang, JSON that parses, a language's signs on most lines, or
// highlight.js very sure), else "text". Short or unclear stays "text".
export function guessNoteLang(text: string): string {
  const s = text.slice(0, SAMPLE).trim()
  if (s.length < 12) return 'text'
  const lines = s.split('\n').filter((l) => l.trim())
  if (lines.some((l) => FENCE.test(l))) return 'markdown'
  const bang = /^#!\S*?(?:env\s+)?(\w+)/.exec(s)
  if (bang) return /sh$/.test(bang[1]) ? 'bash' : languageOf(bang[1].replace(/\d.*$/, '')) || 'bash'
  if (/^[[{]/.test(s) && /[\]}]$/.test(s)) {
    try {
      JSON.parse(s)
      return 'json'
    } catch {
      /* not JSON */
    }
  }
  if (/^(<\?xml|<!doctype html|<html|<svg|<[a-z][\w-]*[\s>])/i.test(s) && /<\/[a-z][\w-]*>\s*$|\/>\s*$/i.test(s)) return 'xml'
  const signs = signsOf(s, lines, 2)
  if (signs) return signs
  const md = lines.filter((l) => MD_LINE.test(l)).length + (MD_INLINE.test(s) ? 1 : 0)
  if (md) return 'markdown'
  const code = detectLanguage(s)
  if (code.language && code.relevance >= 12 && code.relevance / lines.length >= 2) return code.language
  return 'text'
}

// noteLangLabel names a note's language ("Markdown", "Plain text", "JSON").
export function noteLangLabel(lang: string, plain: string): string {
  if (lang === 'text' || !lang) return plain
  return labelOf(lang)
}

export const isCodeLang = (lang: string) => lang !== 'text' && lang !== 'markdown' && !!lang

// A coloured stretch of the text: [from, to) with its classes.
export interface Tok {
  from: number
  to: number
  cls: string
}

// noteTokens colours a note's text as lang — flat stretches in order, none
// overlapping (plain text has none).
export function noteTokens(text: string, lang: string): Tok[] {
  if (!lang || lang === 'text') return []
  if (lang !== 'markdown') return codeTokens(text, lang, 0, '')
  return markdownTokens(text)
}

// codeTokens: highlight.js's tokens of code at offset at, each with base too.
function codeTokens(code: string, lang: string, at: number, base: string): Tok[] {
  const out: Tok[] = []
  let tree: Root
  try {
    tree = lowlight.highlight(lang, code)
  } catch {
    return base ? [{ from: at, to: at + code.length, cls: base }] : []
  }
  let pos = at
  const walk = (nodes: (RootContent | ElementContent)[], cls: string[]) => {
    for (const n of nodes) {
      if (n.type === 'text') {
        const c = [base, ...cls].filter(Boolean).join(' ')
        if (c && n.value.length) out.push({ from: pos, to: pos + n.value.length, cls: c })
        pos += n.value.length
      } else if (n.type === 'element') {
        const own = (n.properties?.className as string[] | undefined) ?? []
        walk(n.children, [...cls, ...own])
      }
    }
  }
  walk(tree.children, [])
  return out
}

const INLINE = /(`[^`\n]+`)|(\*\*[^*\n]+\*\*|__[^_\n]+__)|(\*[^*\s][^*\n]*\*|_[^_\s][^_\n]*_)|(\[[^\]\n]+\]\([^)\s]+\))|(https?:\/\/[^\s<>`]+[^\s<>`.,;:!?)\]'"])/g

// markdownTokens: headings, quotes, list marks, ``` blocks (their code
// coloured by its language, or the one it clearly is) and inline marks.
function markdownTokens(text: string): Tok[] {
  const out: Tok[] = []
  const lines = text.split('\n')
  let pos = 0
  let fence: { mark: string; lang: string; start: number } | null = null
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const end = pos + line.length
    const f = FENCE.exec(line)
    if (fence) {
      if (f && f[1][0] === fence.mark[0] && f[1].length >= fence.mark.length && !line.trim().slice(f[1].length)) {
        // The block's code, then its closing fence.
        if (pos > fence.start) {
          const code = text.slice(fence.start, pos - 1)
          out.push(...codeTokens(code, fence.lang || blockLang(code) || 'plaintext', fence.start, 'md-codeblock'))
        }
        out.push({ from: pos, to: end, cls: 'md-fence' })
        fence = null
      }
    } else if (f) {
      out.push({ from: pos, to: end, cls: 'md-fence' })
      fence = { mark: f[1], lang: languageOf(f[2]), start: end + 1 }
    } else if (/^#{1,6}\s/.test(line)) {
      out.push({ from: pos, to: end, cls: 'md-heading' })
    } else if (/^>/.test(line)) {
      out.push({ from: pos, to: end, cls: 'md-quote' })
    } else {
      const m = /^(\s*)((?:[-*+•]|\d{1,9}[.)])\s+)?(\[[ xX]\]\s+)?/.exec(line)!
      let from = m[1].length
      const mark = (m[2] ?? '') + (m[3] ?? '')
      if (mark) {
        out.push({ from: pos + from, to: pos + from + mark.length, cls: 'md-bullet' })
        from += mark.length
      }
      INLINE.lastIndex = from
      let x: RegExpExecArray | null
      while ((x = INLINE.exec(line))) {
        const cls = x[1] ? 'md-code' : x[2] ? 'md-strong' : x[3] ? 'md-em' : 'md-link'
        out.push({ from: pos + x.index, to: pos + x.index + x[0].length, cls })
      }
    }
    pos = end + 1
  }
  // A block left open: its code to the end.
  if (fence && fence.start < text.length) {
    const code = text.slice(fence.start)
    out.push(...codeTokens(code, fence.lang || blockLang(code) || 'plaintext', fence.start, 'md-codeblock'))
  }
  return out
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

// tokensHtml writes text as HTML with its stretches in spans (escaped).
export function tokensHtml(text: string, toks: Tok[]): string {
  let html = ''
  let pos = 0
  for (const t of toks) {
    if (t.from < pos) continue
    html += esc(text.slice(pos, t.from)) + `<span class="${t.cls}">${esc(text.slice(t.from, t.to))}</span>`
    pos = t.to
  }
  return html + esc(text.slice(pos))
}

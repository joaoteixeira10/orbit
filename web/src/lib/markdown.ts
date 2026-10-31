// Pages are GitHub-flavoured Markdown. This is how the app reads them (the
// reading view: GitHub alerts, footnotes, task boxes that can be ticked,
// sanitised), keeps them tidy (one style, whichever editor saved), and moves
// them in and out as .md files.
import { Marked, type Tokens } from 'marked'
import markedFootnote from 'marked-footnote'
import DOMPurify from 'dompurify'
import { i18n } from '@/i18n'
import { highlightCode, labelOf } from './syntax'
import { remoteHost } from './media'
import { isUploadSrc } from './uploads'
import { MENTION, dateLabel, isDate } from './mentions'

// ---- Normalising ----------------------------------------------------------

// normalizeMarkdown is what every save stores: trailing spaces trimmed (but
// a line break's two kept), at most one blank line in a row outside code,
// no blank lines at either end. Both editors save through it, so switching
// editors doesn't reformat a page.
export function normalizeMarkdown(md: string): string {
  const lines = md.replace(/\r\n?/g, '\n').split('\n')
  const out: string[] = []
  let fence = ''
  let blank = 0
  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i]
    const m = /^\s*(`{3,}|~{3,})/.exec(raw)
    if (fence) {
      out.push(raw)
      if (m && m[1][0] === fence[0] && m[1].length >= fence.length) fence = ''
      continue
    }
    if (m) fence = m[1]
    // Two trailing spaces are a line break only inside a paragraph: kept when
    // the next line goes on (and this isn't a heading), dropped otherwise.
    const next = lines[i + 1] ?? ''
    const breaks = / {2}$/.test(raw) && raw.trim() !== '' && next.trim() !== '' && !/^\s*#/.test(raw)
    const line = breaks ? raw.replace(/\s+$/, '  ') : raw.replace(/\s+$/, '')
    if (!line) {
      if (++blank > 1) continue
    } else blank = 0
    out.push(line)
  }
  return out.join('\n').trim()
}

// ---- Reading --------------------------------------------------------------

type AlertKind = 'NOTE' | 'TIP' | 'IMPORTANT' | 'WARNING' | 'CAUTION'

function reader(): Marked {
  const md = new Marked({ gfm: true, breaks: false })
  md.use(markedFootnote())
  let task = 0
  // > [!TIP] … becomes a GitHub alert. A tokenizer on the same lexer (not a
  // second one) so footnotes inside keep working.
  md.use({
    extensions: [
      {
        name: 'alert',
        level: 'block',
        start: (src: string) => src.match(/^> ?\[!/m)?.index,
        tokenizer(src: string) {
          const m = /^> ?\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*\n((?:>[^\n]*(?:\n|$))*)/i.exec(src)
          if (!m) return undefined
          const inner = m[2].replace(/^> ?/gm, '')
          return { type: 'alert', raw: m[0], kind: m[1].toUpperCase(), tokens: this.lexer.blockTokens(inner, []) }
        },
        renderer(token: any) {
          const kind = token.kind as AlertKind
          return `<div class="md-alert md-alert-${kind.toLowerCase()}" data-kind="${kind}"><p class="md-alert-title">${i18n.global.t(`editor.alert.${kind}`)}</p>${this.parser.parse(token.tokens)}</div>`
        },
      },
    ],
  })
  md.use({
    hooks: { preprocess: (src) => ((task = 0), src) },
    renderer: {
      // Task boxes carry their index, so a click can tick the right line.
      checkbox({ checked }: Tokens.Checkbox) {
        return `<input type="checkbox" data-task="${task++}"${checked ? ' checked' : ''}> `
      },
      // Code blocks are highlighted: in their fence's language, else in the
      // one detected (lib/syntax.ts), which also labels the block.
      code({ text, lang }: Tokens.Code) {
        const { html, language } = highlightCode(text, (lang ?? '').split(/\s/)[0])
        const label = language ? ` data-lang="${labelOf(language)}"` : ''
        return `<pre${label}><code class="hljs${language ? ` language-${language}` : ''}">${html}</code></pre>\n`
      },
      // An image from another site waits behind a placeholder (lib/media.ts);
      // the reading view shows it on a click, or at once when the reader
      // chose to always show them.
      image({ href, title, text }: Tokens.Image) {
        const esc = (v: string) => v.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
        const host = remoteHost(href)
        const t = title ? ` title="${esc(title)}"` : ''
        // An upload is fetched with the sign-in token (lib/uploads.ts): the view
        // fills in its src.
        if (isUploadSrc(href)) return `<img data-upload="${esc(href)}" alt="${esc(text)}"${t} class="md-upload">`
        if (!host) return `<img src="${esc(href)}" alt="${esc(text)}"${t} loading="lazy">`
        const label = i18n.global.t('editor.remoteImage', { host })
        return `<span class="md-remote-img" role="button" tabindex="0" data-src="${esc(href)}" data-alt="${esc(text)}"${t}>${esc(label)}</span>`
      },
      // Links leave the app in a new tab.
      link({ href, title, tokens }: Tokens.Link) {
        const text = this.parser.parseInline(tokens)
        const t = title ? ` title="${title}"` : ''
        // A file uploaded to Orbit downloads through the app (it needs the
        // sign-in token); the view handles the click.
        if (isUploadSrc(href)) {
          const attr = (v: string) => v.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
          const name = attr(this.parser.parseInline(tokens, this.parser.textRenderer))
          return `<a href="${attr(href)}"${t} data-upload="${attr(href)}" data-name="${name}" class="md-file">${text}</a>`
        }
        // A link to this Orbit, written in full (pasted), reads as one within it.
      if (typeof location !== 'undefined' && href.startsWith(location.origin + '/')) href = href.slice(location.origin.length)
      // A link within Orbit (a page, the calendar) opens here.
        if (/^\/(?!\/)/.test(href)) return `<a href="${href}"${t} class="md-internal${/^\/w\/[^/]+\/p\//.test(href) ? ' md-page' : /^\/w\/[^/]+\/s\//.test(href) ? ' md-sheet' : /^\/w\/[^/]+\/d\//.test(href) ? ' md-diagram' : /^\/w\/[^/]+\/whiteboards\//.test(href) ? ' md-whiteboard' : ''}">${text}</a>`
        return `<a href="${href}"${t} target="_blank" rel="noopener noreferrer nofollow">${text}</a>`
      },
    },
  })
  return md
}

let cached: Marked | null = null

// renderMarkdown is a page's HTML for the reading view, sanitised (pages are
// shared, and Markdown may hold HTML).
export function renderMarkdown(md: string): string {
  cached ??= reader()
  const html = cached.parse(md, { async: false }) as string
  return mentionize(
    DOMPurify.sanitize(html, {
      ADD_ATTR: ['target', 'data-task', 'data-kind', 'data-lang', 'data-upload', 'data-name'],
      FORBID_TAGS: ['style', 'form', 'button', 'textarea', 'select', 'iframe'],
    }),
  )
}

// mentionize shows @username as a chip and @2026-10-23 as a date that opens
// the calendar on it — in text, not in links or code.
function mentionize(html: string): string {
  if (!html.includes('@')) return html
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  const walker = document.createTreeWalker(tpl.content, NodeFilter.SHOW_TEXT)
  const found: Text[] = []
  while (walker.nextNode()) {
    const n = walker.currentNode as Text
    if (n.data.includes('@') && !n.parentElement?.closest('a, code, pre')) found.push(n)
  }
  for (const n of found) {
    const frag = document.createDocumentFragment()
    let last = 0
    for (const m of n.data.matchAll(MENTION)) {
      const at = m.index! + m[1].length
      const word = m[2]
      let chip: HTMLElement
      if (/^\d{4}-/.test(word)) {
        if (!isDate(word)) continue
        const a = document.createElement('a')
        a.href = `/calendar?date=${word}`
        a.className = 'md-internal md-date'
        a.textContent = dateLabel(word)
        a.title = word
        chip = a
      } else {
        chip = document.createElement('span')
        chip.className = 'md-mention'
        chip.textContent = '@' + word
      }
      frag.append(n.data.slice(last, at), chip)
      last = at + 1 + word.length
    }
    if (!last) continue
    frag.append(n.data.slice(last))
    n.replaceWith(frag)
  }
  return tpl.innerHTML
}

// ---- Ticking a task in the reading view ------------------------------------

const TASK = /^(\s*(?:[-*+]|\d+[.)])\s+\[)([ xX])(\])/

// toggleTask ticks (or unticks) the index-th task box of a page, counting
// them as the reader does (outside code).
export function toggleTask(md: string, index: number): string {
  const lines = md.split('\n')
  let fence = ''
  let n = 0
  for (let i = 0; i < lines.length; i++) {
    const f = /^\s*(`{3,}|~{3,})/.exec(lines[i])
    if (fence) {
      if (f && f[1][0] === fence[0]) fence = ''
      continue
    }
    if (f) {
      fence = f[1]
      continue
    }
    const m = TASK.exec(lines[i])
    if (!m) continue
    if (n++ === index) {
      lines[i] = lines[i].replace(TASK, (_a, pre, box, post) => `${pre}${box === ' ' ? 'x' : ' '}${post}`)
      return lines.join('\n')
    }
  }
  return md
}

// ---- Files ----------------------------------------------------------------

// pageFile is a page as a .md file: its title as a heading, then its body.
export function pageFile(title: string, content: string, meta: { tags?: string[]; fields?: PageField[] } = {}): string {
  const head = `${frontmatter(meta.tags ?? [], meta.fields ?? [])}# ${title.trim() || 'Untitled'}`
  return content.trim() ? `${head}\n\n${content.trim()}\n` : `${head}\n`
}

// A page's own property (any page): a name, a kind, a value of that kind.
export interface PageField {
  key: string
  type: 'text' | 'number' | 'date' | 'checkbox' | 'url'
  value: string | number | boolean | null
}

// frontmatter: tags and properties as a Markdown file's YAML header ("" for
// none) — as Obsidian and the like keep them.
export function frontmatter(tags: string[], fields: PageField[]): string {
  if (!tags.length && !fields.length) return ''
  const q = (v: unknown) => JSON.stringify(v) // JSON is YAML
  const lines = ['---']
  if (tags.length) lines.push(`tags: ${q(tags)}`)
  for (const f of fields) {
    const key = /[:#"'{}[\],&*?|<>=!%@`]/.test(f.key) || f.key.toLowerCase() === 'tags' ? q(f.key) : f.key
    lines.push(f.value === null || f.value === '' ? `${key}:` : `${key}: ${q(f.value)}`)
  }
  return lines.join('\n') + '\n---\n'
}

// readFrontmatter takes a YAML header off a Markdown file: its tags and the
// rest as properties (simple "key: value" lines; lists only for tags).
export function readFrontmatter(md: string): { tags: string[]; fields: PageField[]; rest: string } {
  const m = /^---\n([\s\S]*?)\n---[ \t]*(?:\n|$)/.exec(md)
  if (!m) return { tags: [], fields: [], rest: md }
  const tags: string[] = []
  const fields: PageField[] = []
  const lines = m[1].split('\n')
  for (let i = 0; i < lines.length; i++) {
    // "a quoted: key": value, or key: value
    const quoted = /^("(?:[^"\\]|\\.)*")\s*:\s*(.*)$/.exec(lines[i])
    const plain = quoted ? null : /^([^:#\s][^:]*?)\s*:\s*(.*)$/.exec(lines[i])
    if (!quoted && !plain) continue
    let key: string
    try {
      key = quoted ? (JSON.parse(quoted[1]) as string) : plain![1].trim()
    } catch {
      continue
    }
    let raw = (quoted ? quoted[2] : plain![2]).trim()
    if (key.toLowerCase() === 'tags') {
      if (!raw) {
        while (i + 1 < lines.length && /^\s*-\s+/.test(lines[i + 1])) tags.push(lines[++i].replace(/^\s*-\s+/, '').replace(/^["']|["']$/g, ''))
      } else {
        try {
          const v = JSON.parse(raw.replace(/'/g, '"'))
          if (Array.isArray(v)) tags.push(...v.map(String))
          else tags.push(String(v))
        } catch {
          tags.push(...raw.replace(/^\[|\]$/g, '').split(',').map((x) => x.trim().replace(/^["'#]|["']$/g, '')).filter(Boolean))
        }
      }
      continue
    }
    if (/^["'].*["']$/.test(raw)) raw = raw.slice(1, -1)
    let field: PageField
    if (raw === 'true' || raw === 'false') field = { key, type: 'checkbox', value: raw === 'true' }
    else if (/^-?\d+(\.\d+)?$/.test(raw)) field = { key, type: 'number', value: Number(raw) }
    else if (/^\d{4}-\d{2}-\d{2}$/.test(raw)) field = { key, type: 'date', value: raw }
    else if (/^(https?:|mailto:)/.test(raw)) field = { key, type: 'url', value: raw }
    else field = { key, type: 'text', value: raw || null }
    if (!fields.some((f) => f.key.toLowerCase() === key.toLowerCase())) fields.push(field)
  }
  return { tags: tags.map((x) => x.trim()).filter(Boolean), fields: fields.slice(0, 40), rest: md.slice(m[0].length) }
}

// fromFile reads an imported .md: its YAML header becomes its tags and
// properties; a first-line "# Heading" becomes the title (else the file's
// name does), the rest the page.
export function fromFile(text: string, filename: string): { title: string; content: string; tags: string[]; fields: PageField[] } {
  const { tags, fields, rest } = readFrontmatter(text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n'))
  const m = /^\s*#\s+(.+?)\s*#*\s*(?:\n|$)/.exec(rest)
  if (m) return { title: m[1].trim().slice(0, 300), content: normalizeMarkdown(rest.slice(m[0].length)), tags, fields }
  return { title: filename.replace(/\.(md|markdown|txt)$/i, '').slice(0, 300), content: normalizeMarkdown(rest), tags, fields }
}

// download saves text as a file.
export function download(name: string, text: string, type = 'text/markdown') {
  const url = URL.createObjectURL(new Blob([text], { type: `${type};charset=utf-8` }))
  const a = Object.assign(document.createElement('a'), { href: url, download: name })
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// fileName makes a title safe as a file name.
export function fileName(title: string): string {
  const name = title.trim().replace(/[/\\:*?"<>|\u0000-\u001f]/g, '-').replace(/^[. ]+|[. ]+$/g, '').slice(0, 100)
  return name || 'Untitled'
}

// textToMarkdown writes plain text (a note) as Markdown that reads back as
// the same text: each line its own, nothing turned into formatting.
export function textToMarkdown(text: string): string {
  return text
    .split('\n')
    .map((l) => l.replace(/([\\`*_[\]~<#>|])/g, '\\$1').replace(/^(\s*)([-+]|\d+[.)])(\s)/, '$1\\$2$3'))
    .join('  \n')
}

// firstLine is a note's first line as plain text, for a one-line preview: no
// heading, list or quote marks, no emphasis, links as their text, images
// dropped, a fenced block skipped.
export function firstLine(md: string): string {
  let fence = false
  for (const raw of md.split('\n')) {
    const l = raw.trim()
    if (/^(`{3,}|~{3,})/.test(l)) {
      fence = !fence
      continue
    }
    if (fence || !l || /^([-*_]\s*){3,}$/.test(l) || /^\|?\s*:?-{3,}/.test(l)) continue
    const s = l
      .replace(/^(#{1,6}\s+|>\s*|[-*+]\s+(\[[ xX]\]\s+)?|\d+[.)]\s+(\[[ xX]\]\s+)?)+/, '')
      .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
      .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/(\*\*|__|~~|`)(.+?)\1/g, '$2')
      .replace(/(^|\W)[*_](\S(?:.*?\S)?)[*_](?=\W|$)/g, '$1$2')
      .replace(/\\([\\`*_[\]~<#>|+.!-])/g, '$1')
      .trim()
    if (s) return s
  }
  return ''
}

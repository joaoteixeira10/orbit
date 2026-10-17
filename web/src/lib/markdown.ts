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
        if (!host) return `<img src="${esc(href)}" alt="${esc(text)}"${t} loading="lazy">`
        const label = i18n.global.t('editor.remoteImage', { host })
        return `<span class="md-remote-img" role="button" tabindex="0" data-src="${esc(href)}" data-alt="${esc(text)}"${t}>${esc(label)}</span>`
      },
      // Links leave the app in a new tab.
      link({ href, title, tokens }: Tokens.Link) {
        const text = this.parser.parseInline(tokens)
        const t = title ? ` title="${title}"` : ''
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
  return DOMPurify.sanitize(html, {
    ADD_ATTR: ['target', 'data-task', 'data-kind', 'data-lang'],
    FORBID_TAGS: ['style', 'form', 'button', 'textarea', 'select', 'iframe'],
  })
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
export function pageFile(title: string, content: string): string {
  const head = `# ${title.trim() || 'Untitled'}`
  return content.trim() ? `${head}\n\n${content.trim()}\n` : `${head}\n`
}

// fromFile reads an imported .md: a first-line "# Heading" becomes the title
// (else the file's name does), the rest the page.
export function fromFile(text: string, filename: string): { title: string; content: string } {
  const md = text.replace(/^\uFEFF/, '').replace(/\r\n?/g, '\n')
  const m = /^\s*#\s+(.+?)\s*#*\s*(?:\n|$)/.exec(md)
  if (m) return { title: m[1].trim().slice(0, 300), content: normalizeMarkdown(md.slice(m[0].length)) }
  return { title: filename.replace(/\.(md|markdown|txt)$/i, '').slice(0, 300), content: normalizeMarkdown(md) }
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

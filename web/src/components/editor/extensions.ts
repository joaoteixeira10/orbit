// The blocks and marks of a page, and their Markdown. Pages are stored as
// GitHub-flavoured Markdown; the visual editor reads and writes it through
// @tiptap/markdown. What plain Markdown has no syntax for is written the way
// GitHub shows it: highlight and underline as <mark> and <u>, callouts as
// GitHub alerts (> [!TIP]). What the editor can't edit (HTML blocks,
// footnotes, <kbd>…) passes through untouched (raw.ts), and text is escaped
// only where a character would otherwise turn into formatting, so Markdown
// written by hand survives a visual edit. Used by the editor and the tests.
import type { AnyExtension } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { TaskList, TaskItem } from '@tiptap/extension-list'
import { TableKit } from '@tiptap/extension-table'
import { Highlight } from '@tiptap/extension-highlight'
import { Underline } from '@tiptap/extension-underline'
import { Markdown, MarkdownManager } from '@tiptap/markdown'
import { PageImage } from './image'
import { Paragraph } from '@tiptap/extension-paragraph'
import { Callout } from './callout'
import { CodeBlock } from './codeblock'
import { RawBlock, RawInline, FootnoteDef } from './raw'
import { ALERTS, alertOf, type AlertType } from './alerts'


// A paragraph holding only an image: TipTap unwraps it to a bare image, which
// only fits block images — pages' images are inline (in a line of text, or
// alone), so the image stays in its paragraph. Same Markdown either way.
const MarkdownParagraph = Paragraph.extend({
  parseMarkdown: (token: any, helpers: any) => {
    const tokens = token.tokens || []
    if (tokens.length === 1 && tokens[0].type === 'image') return helpers.createNode('paragraph', undefined, helpers.parseInline(tokens))
    return Paragraph.config.parseMarkdown!.call(Paragraph as any, token, helpers)
  },
})

const MarkdownHighlight = Highlight.extend({
  renderMarkdown: (node, h) => `<mark>${h.renderChildren(node)}</mark>`,
})
const MarkdownUnderline = Underline.extend({
  renderMarkdown: (node, h) => `<u>${h.renderChildren(node)}</u>`,
})

// > [!TIP]
// > text…
const ALERT = /^> ?\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*\n((?:>[^\n]*(?:\n|$))*)/i
const MarkdownCallout = Callout.extend({
  markdownTokenizer: {
    name: 'callout',
    level: 'block',
    start: (src: string) => src.search(/^> ?\[!/m),
    tokenize(src: string, _tokens: unknown, lexer: any): any {
      const m = ALERT.exec(src)
      if (!m) return undefined
      const inner = m[2].replace(/^> ?/gm, '')
      return { type: 'callout', raw: m[0], kind: m[1].toUpperCase(), tokens: lexer.blockTokens(inner) }
    },
  },
  parseMarkdown: (token: any, h: any) =>
    h.createNode('callout', { emoji: ALERTS[token.kind as AlertType] ?? ALERTS.NOTE }, (h.parseBlockChildren ?? h.parseChildren)(token.tokens || [])),
  renderMarkdown: (node: any, h: any) => {
    const body = (node.content ?? [])
      .map((child: any, i: number) => (h.renderChild?.(child, i) ?? h.renderChildren([child])) as string)
      .join('\n\n')
    const lines = body.split('\n').map((l: string) => (l.trim() ? `> ${l}` : '>'))
    return [`> [!${alertOf(node.attrs?.emoji)}]`, ...lines].join('\n')
  },
})

// minimalEscape backslash-escapes only what would otherwise read as Markdown:
// * or _ that could open or close emphasis (not "2 * 3", not snake_case),
// ~~, `, a [ with a ] after it, a backslash before punctuation, and < or &
// that would start a tag or an entity.
export function minimalEscape(text: string): string {
  const word = (c: string | undefined) => !!c && /[\p{L}\p{N}]/u.test(c)
  const space = (c: string | undefined) => c === undefined || /\s/.test(c)
  let out = ''
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    const prev = text[i - 1]
    const next = text[i + 1]
    switch (c) {
      case '\\':
        out += /[!-/:-@[-`{-~]/.test(next ?? '') ? '\\\\' : c
        break
      case '`':
        out += '\\`'
        break
      case '*':
        out += space(prev) && space(next) ? c : '\\*'
        break
      case '_':
        out += word(prev) && word(next) ? c : '\\_'
        break
      case '~':
        out += prev === '~' || next === '~' ? '\\~' : c
        break
      case '[':
        out += text.indexOf(']', i) > i ? '\\[' : c
        break
      case '<':
        out += /[A-Za-z/!?]/.test(next ?? '') ? '&lt;' : c
        break
      case '&':
        out += /^&(#\d+|#x[\da-f]+|\w+);/i.test(text.slice(i)) ? '&amp;' : c
        break
      default:
        out += c
    }
  }
  return out
}
;(MarkdownManager.prototype as any).encodeTextForMarkdown = function (this: any, text: string, node: any, parentNode: any) {
  const inCode =
    (parentNode?.type != null && this.codeTypes.has(parentNode.type)) ||
    (node?.marks || []).some((m: any) => this.codeTypes.has(typeof m === 'string' ? m : m.type))
  return inCode ? text : minimalEscape(text)
}

// pageExtensions are the page schema with Markdown; extra adds the editing
// helpers (placeholder, slash menu) that only the editor needs.
export function pageExtensions(extra: AnyExtension[] = [], link: Record<string, unknown> = {}): AnyExtension[] {
  return [
    StarterKit.configure({
      heading: { levels: [1, 2, 3] },
      dropcursor: { color: 'var(--ws)', width: 2 },
      underline: false,
      codeBlock: false, // highlighted: ./codeblock
      paragraph: false, // MarkdownParagraph
      link: { openOnClick: false, autolink: true, linkOnPaste: true, defaultProtocol: 'https', ...link },
    }),
    MarkdownParagraph,
    MarkdownUnderline,
    CodeBlock,
    TaskList,
    TaskItem.configure({ nested: true }),
    TableKit.configure({ table: { resizable: false, renderWrapper: true } }),
    MarkdownHighlight,
    MarkdownCallout,
    PageImage.configure({ inline: true }),
    RawBlock,
    FootnoteDef,
    RawInline,
    Markdown.configure({ indentation: { style: 'space', size: 2 } }),
    ...extra,
  ]
}

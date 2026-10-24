// A page as PDF: its title and rendered Markdown in a clean, light layout
// in a hidden frame, printed — the browser's print dialog saves it as PDF
// (real text, links that work). Its own pictures are fetched first (they need
// the sign-in); pictures from other sites stay a line naming the site.
import { renderMarkdown } from './markdown'
import { uploadBlobUrl } from './uploads'

const CSS = `
@page { margin: 18mm 16mm; }
* { box-sizing: border-box; }
html { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
body { margin: 0; font: 11pt/1.55 -apple-system, BlinkMacSystemFont, "SF Pro Text", "Segoe UI", system-ui, sans-serif; color: #1d1d1f; background: #fff; }
.meta { color: #6e6e73; font-size: 9pt; margin-bottom: 4pt; }
h1.title { font-size: 24pt; line-height: 1.2; margin: 0 0 14pt; }
h1, h2, h3, h4 { line-height: 1.25; margin: 18pt 0 6pt; break-after: avoid; }
h1 { font-size: 18pt; } h2 { font-size: 15pt; } h3 { font-size: 12.5pt; }
p, ul, ol, blockquote, table, pre { margin: 0 0 8pt; }
a { color: #0066cc; text-decoration: none; }
img { max-width: 100%; border-radius: 6pt; break-inside: avoid; }
code { font: 9.5pt ui-monospace, "SF Mono", Menlo, monospace; background: #f2f2f4; padding: 0 3pt; border-radius: 3pt; }
pre { background: #f6f6f8; padding: 8pt 10pt; border-radius: 6pt; white-space: pre-wrap; break-inside: avoid; }
pre code { background: none; padding: 0; }
blockquote { border-left: 3pt solid #d2d2d7; padding-left: 10pt; color: #3a3a3c; }
table { border-collapse: collapse; width: 100%; font-size: 10pt; }
th, td { border: 0.5pt solid #d2d2d7; padding: 4pt 6pt; text-align: left; vertical-align: top; }
th { background: #f6f6f8; }
hr { border: 0; border-top: 0.5pt solid #d2d2d7; margin: 14pt 0; }
li:has(> input[type=checkbox]) { list-style: none; margin-left: -16pt; }
input[type=checkbox] { margin: 0 6pt 0 0; vertical-align: -1pt; }
mark { background: #fff3a3; }
.md-alert { border-left: 3pt solid #0a84ff; background: #f2f7ff; padding: 6pt 10pt; border-radius: 4pt; margin-bottom: 8pt; }
.md-alert-tip { border-color: #34c759; background: #f0fbf3; } .md-alert-warning, .md-alert-caution { border-color: #ff9500; background: #fff7ec; }
.md-alert-title { font-weight: 600; margin-bottom: 2pt; }
.md-mention, .md-date { background: #eef4ff; color: #0058d0; border-radius: 3pt; padding: 0 2pt; }
.md-remote-img { color: #6e6e73; font-style: italic; }
.hljs-keyword, .hljs-selector-tag, .hljs-literal { color: #ad3da4; } .hljs-string, .hljs-attr { color: #c41a16; }
.hljs-comment { color: #707f8c; font-style: italic; } .hljs-number { color: #1c00cf; } .hljs-title, .hljs-function { color: #326d74; }
`

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

// printHtml is the document printed (exported for tests).
export function printHtml(title: string, html: string, meta: string): string {
  return `<!doctype html><html><head><meta charset="utf-8"><title>${esc(title)}</title><style>${CSS}</style></head><body>` +
    `<div class="meta">${esc(meta)}</div><h1 class="title">${esc(title)}</h1>${html}</body></html>`
}

export async function printPage(title: string, content: string, meta: string): Promise<void> {
  const tpl = document.createElement('template')
  tpl.innerHTML = renderMarkdown(content)
  // Its own pictures, fetched with the sign-in; inline so the frame shows them.
  await Promise.all(
    [...tpl.content.querySelectorAll<HTMLImageElement>('img[data-upload]')].map(async (img) => {
      try {
        img.src = await uploadBlobUrl(img.dataset.upload!)
      } catch {
        img.remove()
      }
    }),
  )
  const frame = document.createElement('iframe')
  Object.assign(frame.style, { position: 'fixed', right: '0', bottom: '0', width: '0', height: '0', border: '0', visibility: 'hidden' })
  frame.setAttribute('aria-hidden', 'true')
  frame.dataset.print = 'page'
  document.body.append(frame)
  const doc = frame.contentDocument!
  doc.open()
  doc.write(printHtml(title, tpl.innerHTML, meta))
  doc.close()
  // Pictures first, then the dialog; the frame goes once it's done with.
  await Promise.all([...doc.images].map((i) => (i.complete ? null : new Promise((ok) => ((i.onload = ok), (i.onerror = ok))))))
  const w = frame.contentWindow!
  const gone = () => setTimeout(() => frame.remove(), 500)
  w.addEventListener('afterprint', gone, { once: true })
  setTimeout(() => frame.isConnected && frame.remove(), 10 * 60 * 1000)
  w.focus()
  w.print()
}

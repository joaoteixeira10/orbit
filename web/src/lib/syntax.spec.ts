import { describe, it, expect } from 'vitest'
import { Editor } from '@tiptap/core'
import { highlightCode, languageOf } from './syntax'
import { renderMarkdown } from './markdown'
import { pageExtensions } from '@/components/editor/extensions'

const OPENSSL = `[ req ]
default_bits       = 2048
distinguished_name = req_distinguished_name
prompt             = yes

[ req_ext ]
basicConstraints = critical, CA:false
keyUsage = critical, digitalSignature, keyEncipherment`

describe('code highlighting', () => {
  it('uses the fence language, aliases included', () => {
    expect(highlightCode('fmt.Println("hi")', 'go').language).toBe('go')
    expect(highlightCode('echo $HOME', 'sh').language).toBe('bash')
    expect(languageOf('yml')).toBe('yaml')
    expect(languageOf('html')).toBe('xml')
  })

  it('detects an unlabelled block, and leaves prose plain', () => {
    expect(highlightCode(OPENSSL).language).toBe('ini')
    expect(highlightCode('{\n  "name": "orbit",\n  "version": "0.1.0",\n  "private": true\n}').language).toBe('json')
    const prose = highlightCode('Remember to call Ana about the budget.\nThen send the report on Friday.')
    expect(prose.language).toBe('')
    expect(prose.html).not.toContain('hljs-')
  })

  it('an unknown fence language stays plain (no guessing)', () => {
    expect(highlightCode(OPENSSL, 'openssl').language).toBe('')
  })

  it('the reading view colours and labels it, escaped and sanitised', () => {
    const html = renderMarkdown('```go\nfunc main() { x := "<script>alert(1)</script>" }\n```')
    expect(html).toContain('<pre data-lang="Go"><code class="hljs language-go">')
    expect(html).toContain('hljs-keyword')
    expect(html).not.toContain('<script>')
    expect(renderMarkdown('```\n' + OPENSSL + '\n```')).toContain('data-lang="INI / config"')
  })

  it('the visual editor keeps the fence as written', () => {
    for (const md of ['```go\nfunc main() {}\n```', '```\n' + OPENSSL + '\n```', '```openssl\nx = 1\n```']) {
      const ed = new Editor({ extensions: pageExtensions(), content: md, contentType: 'markdown' })
      expect(ed.getMarkdown().trim()).toBe(md)
      ed.destroy()
    }
  })
})

// Syntax highlighting for code blocks, the same in the reading view (marked)
// and the visual editor (TipTap's lowlight code block): highlight.js with a
// fixed set of languages, so both colour code alike and the bundle stays
// small. A block with a language (```go) uses it; one without is detected
// among these languages, and stays plain when no language is a clear match
// (prose, logs, lists of values).
import hljs from 'highlight.js/lib/core'
import { createLowlight } from 'lowlight'
import type { LanguageFn } from 'highlight.js'
import bash from 'highlight.js/lib/languages/bash'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import csharp from 'highlight.js/lib/languages/csharp'
import css from 'highlight.js/lib/languages/css'
import diff from 'highlight.js/lib/languages/diff'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import go from 'highlight.js/lib/languages/go'
import ini from 'highlight.js/lib/languages/ini'
import java from 'highlight.js/lib/languages/java'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import kotlin from 'highlight.js/lib/languages/kotlin'
import makefile from 'highlight.js/lib/languages/makefile'
import markdown from 'highlight.js/lib/languages/markdown'
import nginx from 'highlight.js/lib/languages/nginx'
import php from 'highlight.js/lib/languages/php'
import plaintext from 'highlight.js/lib/languages/plaintext'
import powershell from 'highlight.js/lib/languages/powershell'
import python from 'highlight.js/lib/languages/python'
import ruby from 'highlight.js/lib/languages/ruby'
import rust from 'highlight.js/lib/languages/rust'
import scss from 'highlight.js/lib/languages/scss'
import shell from 'highlight.js/lib/languages/shell'
import sql from 'highlight.js/lib/languages/sql'
import swift from 'highlight.js/lib/languages/swift'
import typescript from 'highlight.js/lib/languages/typescript'
import xml from 'highlight.js/lib/languages/xml'
import yaml from 'highlight.js/lib/languages/yaml'

// The languages, by highlight.js name, with the label shown on a block.
// Aliases come from the grammars (sh, zsh → bash; html, svg → xml; yml;
// js, ts; toml, conf → ini…).
export const LANGUAGES: { id: string; label: string; grammar: LanguageFn }[] = [
  { id: 'bash', label: 'Bash', grammar: bash },
  { id: 'c', label: 'C', grammar: c },
  { id: 'cpp', label: 'C++', grammar: cpp },
  { id: 'csharp', label: 'C#', grammar: csharp },
  { id: 'css', label: 'CSS', grammar: css },
  { id: 'diff', label: 'Diff', grammar: diff },
  { id: 'dockerfile', label: 'Dockerfile', grammar: dockerfile },
  { id: 'go', label: 'Go', grammar: go },
  { id: 'xml', label: 'HTML / XML', grammar: xml },
  { id: 'ini', label: 'INI / config', grammar: ini },
  { id: 'java', label: 'Java', grammar: java },
  { id: 'javascript', label: 'JavaScript', grammar: javascript },
  { id: 'json', label: 'JSON', grammar: json },
  { id: 'kotlin', label: 'Kotlin', grammar: kotlin },
  { id: 'makefile', label: 'Makefile', grammar: makefile },
  { id: 'markdown', label: 'Markdown', grammar: markdown },
  { id: 'nginx', label: 'nginx', grammar: nginx },
  { id: 'php', label: 'PHP', grammar: php },
  { id: 'powershell', label: 'PowerShell', grammar: powershell },
  { id: 'python', label: 'Python', grammar: python },
  { id: 'ruby', label: 'Ruby', grammar: ruby },
  { id: 'rust', label: 'Rust', grammar: rust },
  { id: 'scss', label: 'SCSS', grammar: scss },
  { id: 'shell', label: 'Shell session', grammar: shell },
  { id: 'sql', label: 'SQL', grammar: sql },
  { id: 'swift', label: 'Swift', grammar: swift },
  { id: 'typescript', label: 'TypeScript', grammar: typescript },
  { id: 'yaml', label: 'YAML', grammar: yaml },
]

for (const l of LANGUAGES) hljs.registerLanguage(l.id, l.grammar)
hljs.registerLanguage('plaintext', plaintext)

// Detection only picks a language this sure of itself (highlight.js
// relevance); below it a block stays plain text.
const MIN_RELEVANCE = 4
const SUBSET = LANGUAGES.map((l) => l.id)

// languageOf resolves a fence's language (an alias too) to a known one, or ''.
export function languageOf(name: string | null | undefined): string {
  if (!name) return ''
  const lang = hljs.getLanguage(name.trim().toLowerCase())
  if (!lang) return ''
  return LANGUAGES.find((l) => hljs.getLanguage(l.id) === lang)?.id ?? ''
}

export function labelOf(id: string): string {
  return LANGUAGES.find((l) => l.id === id)?.label ?? id
}

// detectLanguage is highlight.js's best guess among the given languages
// (all but Markdown by default) and how sure it is.
export function detectLanguage(code: string, subset = SUBSET.filter((l) => l !== 'markdown')): { language: string; relevance: number } {
  const auto = hljs.highlightAuto(code, subset)
  return { language: auto.language ?? '', relevance: auto.relevance }
}

// highlightCode is a code block's HTML (escaped, with hljs token spans) and
// the language it was read as ('' = plain).
export function highlightCode(code: string, lang?: string | null): { html: string; language: string } {
  const named = languageOf(lang)
  if (named) return { html: hljs.highlight(code, { language: named, ignoreIllegals: true }).value, language: named }
  if (!lang) {
    const auto = hljs.highlightAuto(code, SUBSET)
    if (auto.language && auto.relevance >= MIN_RELEVANCE) return { html: auto.value, language: auto.language }
  }
  return { html: hljs.highlight(code, { language: 'plaintext' }).value, language: '' }
}

// The editor's highlighter: the same grammars, and detection held to the
// same bar (lowlight on its own colours every block it can guess at).
const base = createLowlight(Object.fromEntries([...LANGUAGES.map((l) => [l.id, l.grammar]), ['plaintext', plaintext]]))
export const lowlight = {
  ...base,
  highlightAuto(code: string, options?: Parameters<typeof base.highlightAuto>[1]) {
    const auto = base.highlightAuto(code, { ...options, subset: SUBSET })
    return (auto.data?.relevance ?? 0) >= MIN_RELEVANCE ? auto : base.highlight('plaintext', code)
  },
}

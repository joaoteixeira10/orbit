// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { markTerms, termsOf } from './glossary'

describe('glossary', () => {
  it('reads terms', () => {
    const md = ['# Glossary', '', '- **SLA**: what we promise to answer within', '**On-call** — the person who answers at night', '', '### Runbook', '', 'Steps for an [incident](/x).', '', '```', '**Not** — a term', '```'].join('\n')
    expect(termsOf(md).map((t) => [t.term, t.definition])).toEqual([
      ['SLA', 'what we promise to answer within'],
      ['On-call', 'the person who answers at night'],
      ['Runbook', 'Steps for an incident.'],
    ])
  })

  it('marks them where used, whole words, not in links or code', () => {
    const el = document.createElement('div')
    el.innerHTML = '<p>Check the sla and the runbook; SLAs differ.</p><p><a href="#">SLA</a> <code>SLA</code></p><h2>SLA</h2>'
    const n = markTerms(el, termsOf('**SLA** — promise\n\n**Runbook** — steps'))
    expect(n).toBe(2)
    expect([...el.querySelectorAll('.md-term')].map((s) => s.textContent)).toEqual(['sla', 'runbook'])
  })
})

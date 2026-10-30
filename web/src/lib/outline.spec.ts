import { describe, expect, it } from 'vitest'
import { outlineOf, plain } from './outline'

describe('outline', () => {
  it('gathers headings, callouts and checkboxes, as the page counts them', () => {
    const md = [
      '# Trip',
      '',
      '> [!TIP]',
      '> Pack **light**.',
      '',
      '## Before',
      '- [ ] Book the [hotel](https://x.example)',
      '- [x] Passport',
      '',
      '```md',
      '# not a heading',
      '- [ ] not a task',
      '```',
      '',
      '### During ##',
      '> [!WARNING] Mind the gap',
      '1. [ ] Call home',
    ].join('\n')
    const o = outlineOf(md)
    expect(o.headings).toEqual([
      { level: 1, text: 'Trip', index: 0 },
      { level: 2, text: 'Before', index: 1 },
      { level: 3, text: 'During', index: 2 },
    ])
    expect(o.callouts).toEqual([
      { kind: 'TIP', text: 'Pack light.', index: 0 },
      { kind: 'WARNING', text: 'Mind the gap', index: 1 },
    ])
    expect(o.tasks).toEqual([
      { done: false, text: 'Book the hotel', index: 0 },
      { done: true, text: 'Passport', index: 1 },
      { done: false, text: 'Call home', index: 2 },
    ])
  })

  it('takes Markdown marks off', () => {
    expect(plain('A *b* `c` <kbd>d</kbd> ![e](f) ~~g~~')).toBe('A b c d e g')
  })
})

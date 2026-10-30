import { describe, expect, it } from 'vitest'
import { closeEmbed, embedEdits, embedEditor, embedHref, embedOf, openEmbed } from './embeds'

describe('embeds', () => {
  it('reads and writes links to sheets, diagrams and whiteboards', () => {
    expect(embedOf('/w/ws1/s/abc')).toEqual({ kind: 'sheet', ws: 'ws1', id: 'abc' })
    expect(embedOf('/w/ws1/d/abc')).toEqual({ kind: 'diagram', ws: 'ws1', id: 'abc' })
    expect(embedOf('/w/ws1/whiteboards/abc')).toEqual({ kind: 'board', ws: 'ws1', id: 'abc' })
    expect(embedOf('/w/ws1/p/abc')).toBeNull()
    expect(embedOf('/w/ws1/s/abc/x')).toBeNull()
    for (const href of ['/w/a/s/1', '/w/a/d/2', '/w/a/whiteboards/3']) expect(embedHref(embedOf(href)!)).toBe(href)
  })

  it('counts edits made in the big editor', () => {
    openEmbed({ kind: 'diagram', ws: 'a', id: 'x' })
    expect(embedEditor.open).toBe(true)
    closeEmbed()
    closeEmbed()
    expect(embedEditor.open).toBe(false)
    expect(embedEdits.x).toBe(2)
  })
})

import { describe, expect, it } from 'vitest'
import { shareNote } from './share'

describe('shareNote', () => {
  it('keeps the link once, under the text', () => {
    expect(shareNote({ title: 'Orbit', text: 'Look at this', url: 'https://example.com/a' })).toEqual({ title: 'Orbit', body: 'Look at this\n\nhttps://example.com/a' })
    // Android: the link inside text.
    expect(shareNote({ title: 'Orbit', text: 'Look https://example.com/a' })).toEqual({ title: 'Orbit', body: 'Look https://example.com/a' })
    expect(shareNote({ text: 'x', url: 'https://example.com/a', title: '' })).toEqual({ body: 'x\n\nhttps://example.com/a' })
  })
  it('drops the title repeated in the text; a lone title is the note', () => {
    expect(shareNote({ title: 'Recipe', text: 'Recipe\nhttps://r.example/1' })).toEqual({ title: 'Recipe', body: 'https://r.example/1' })
    expect(shareNote({ title: 'Just a title' })).toEqual({ body: 'Just a title' })
    expect(shareNote({ title: 'Recipe', text: 'Recipes for winter' })).toEqual({ title: 'Recipe', body: 'Recipes for winter' })
  })
  it('nothing shared: no note', () => {
    expect(shareNote({})).toBeNull()
    expect(shareNote({ title: '  ', text: '\n' })).toBeNull()
  })
})

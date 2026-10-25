import { describe, it, expect } from 'vitest'
import { FUNCTIONS, at, helpFor, suggest } from './formulaHelp'
import { parse, evaluate } from './formula'

describe('formula help', () => {
  it('suggests by what’s typed, the language’s own names first', () => {
    expect(suggest('su', false).map((f) => f.en).slice(0, 4)).toEqual(['SUM', 'SUMIF', 'SUMIFS', 'SUBSTITUTE'])
    expect(suggest('so', true).map((f) => f.pt).slice(0, 3)).toEqual(['SOMA', 'SOMA.SE', 'SOMA.SE.S'])
    expect(suggest('med', true)[0].pt).toBe('MÉDIA') // accents aside
    expect(suggest('', false)).toEqual([])
  })
  it('knows the word being typed and the call the caret is in', () => {
    expect(at('=SU', 3)).toEqual({ word: { from: 1, text: 'SU' }, call: null })
    expect(at('=IF(A1>2, ROU', 13)).toMatchObject({ word: { from: 10, text: 'ROU' }, call: { name: 'IF', arg: 1 } })
    expect(at('=ROUND(B2; 2', 12).call).toEqual({ name: 'ROUND', arg: 1 })
    expect(at('=SUM(B1', 7).word).toBeNull() // a cell, not a function
    expect(at('="a, b" & LEN(', 14).call).toEqual({ name: 'LEN', arg: 0 })
    expect(at('=SUM(1)+', 8).call).toBeNull()
  })
  it('has help for every function the engine knows, and only those', () => {
    for (const f of FUNCTIONS) {
      // Each name, English and Portuguese, is one the engine runs.
      for (const name of [f.en, f.pt]) {
        const v = evaluate(parse(`=${name}()`), { today: 0, row: () => undefined, column: () => undefined })
        expect(v, name).not.toEqual({ error: '#NAME?' })
      }
    }
    expect(helpFor('procv')?.en).toBe('VLOOKUP')
  })
})

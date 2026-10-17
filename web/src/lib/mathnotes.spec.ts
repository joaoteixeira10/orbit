import { describe, expect, it } from 'vitest'
import { acceptAnswer, evaluate } from './mathnotes'

const answers = (text: string) => Object.fromEntries(evaluate(text).map((r) => [r.line, r.result]))

describe('math in notes', () => {
  it('answers a line that ends with "="', () => {
    expect(answers('120 + 35 =')).toEqual({ 0: '155' })
    expect(answers('2 * (3 + 4) =')).toEqual({ 0: '14' })
    expect(answers('10 ÷ 4 =\n3 × 3 =\n2^10 =\n7 − 10 =')).toEqual({ 0: '2.5', 1: '9', 2: '1024', 3: '-3' })
    expect(answers('1/3 =')).toEqual({ 0: '0.33333333' })
  })
  it('leaves other lines alone', () => {
    expect(answers('Buy milk\n2026 =\n120 + 35 = 155\nx ==')).toEqual({})
    expect(answers('1/0 =')).toEqual({})
  })
  it('keeps the words before a sum', () => {
    expect(answers('March: 650 * 3 =\nGroceries 12 + 8 =')).toEqual({ 0: '1950', 1: '20' })
  })
  it('names values for the lines below', () => {
    const text = 'rent = 650\nbills = 120,5\ntotal = rent + bills\nTotal =\ntotal * 12 =\nunknown + 1 ='
    expect(answers(text)).toEqual({ 3: '770,5', 4: '9246' })
  })
  it('reads list items and Markdown', () => {
    expect(answers('- 12 + 3 =\n1. 2 * 4 =\n- [ ] 10 - 1 =')).toEqual({ 0: '15', 1: '8', 2: '9' })
    expect(Object.fromEntries(evaluate('**rent** = 650\n`rent` * 2 =', { markdown: true }).map((r) => [r.line, r.result]))).toEqual({ 1: '1300' })
  })
  it('writes an answer in with Tab', () => {
    const v = 'a = 2\na * 3 =\nmore'
    expect(acceptAnswer(v, 12, evaluate(v))).toEqual({ value: 'a = 2\na * 3 = 6\nmore', caret: 15 })
    expect(acceptAnswer(v, 2, evaluate(v))).toBeNull()
  })
  it('takes names of several words', () => {
    expect(answers('per person = 120 / 4\nPer  person * 2 =\nper person =')).toEqual({ 1: '60', 2: '30' })
  })
  it('does percentages like a calculator', () => {
    expect(answers('80 + 10% =\n80 - 25% =\n20% of 80 =\n15% de 200 =\n80 * 10% =')).toEqual({ 0: '88', 1: '60', 2: '16', 3: '30', 4: '8' })
  })
  it('keeps the decimal comma and the currency', () => {
    expect(answers('2,5 + 1,25 =')).toEqual({ 0: '3,75' })
    expect(answers('€12 + 8 =\n12€ * 3 =\n$9.99 * 3 =')).toEqual({ 0: '€20', 1: '36 €', 2: '$29.97' })
  })
})

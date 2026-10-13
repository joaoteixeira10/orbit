import { describe, it, expect } from 'vitest'
import { messages } from './messages'

// Flatten nested message objects to dotted key paths for set comparison.
function keys(obj: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) => {
    const path = prefix ? `${prefix}.${k}` : k
    return typeof v === 'object' && v !== null ? keys(v as Record<string, unknown>, path) : [path]
  })
}

describe('i18n message catalogs', () => {
  it('every locale defines exactly the same keys as English', () => {
    const en = keys(messages.en).sort()
    for (const [locale, msgs] of Object.entries(messages)) {
      if (locale === 'en') continue
      expect(keys(msgs as Record<string, unknown>).sort(), `locale ${locale} key set`).toEqual(en)
    }
  })

  it('no message value is empty', () => {
    for (const [locale, msgs] of Object.entries(messages)) {
      const flat = (o: Record<string, unknown>): string[] =>
        Object.values(o).flatMap((v) => (typeof v === 'object' && v !== null ? flat(v as Record<string, unknown>) : [String(v)]))
      for (const v of flat(msgs as Record<string, unknown>)) {
        expect(v.trim(), `empty value in ${locale}`).not.toBe('')
      }
    }
  })
})

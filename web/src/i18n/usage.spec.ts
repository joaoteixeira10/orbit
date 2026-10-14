import { describe, it, expect } from 'vitest'
import { messages } from './messages'

// Every message key the app asks for must exist (a missing one renders as
// its raw key, e.g. "bk.title"). Static keys — t('a.b') — are checked
// exactly; a template key — t(`modules.${m}`) — must at least lead into an
// existing group (modules). Keys built some other way aren't seen here.
const sources = import.meta.glob(['../**/*.vue', '../**/*.ts', '!../**/*.spec.ts', '!./*.ts'], { query: '?raw', import: 'default', eager: true }) as Record<string, string>

function lookup(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((o, k) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[k] : undefined), obj)
}

// <i18n-t keypath="a.b"> as well as t('a.b').
const KEYPATH = /\bkeypath="([^"]+)"/g
const CALL = /(?<![\w.])(?:\$?t|te|tm|i18n\.global\.t)\(\s*(?:(['"])([^'"\n]+)\1|`([^`\n]*)`)/g

function used(): { file: string; key: string; dynamic: boolean }[] {
  const out: { file: string; key: string; dynamic: boolean }[] = []
  for (const [file, src] of Object.entries(sources)) {
    for (const m of src.matchAll(KEYPATH)) out.push({ file, key: m[1], dynamic: false })
    for (const m of src.matchAll(CALL)) {
      const raw = m[2] ?? m[3]
      const dyn = raw.indexOf('${')
      if (dyn < 0) out.push({ file, key: raw, dynamic: false })
      // The group the key is in: `editor.block.h${n}.title` → editor.block.
      else if (dyn > 0) out.push({ file, key: raw.slice(0, raw.lastIndexOf('.', dyn - 1)), dynamic: true })
    }
  }
  return out
}

describe('i18n keys used by the app', () => {
  it('scans the sources', () => {
    expect(used().length).toBeGreaterThan(500)
  })
  for (const locale of Object.keys(messages) as (keyof typeof messages)[]) {
    it(`every key exists in ${locale}`, () => {
      const missing = used()
        .filter(({ key, dynamic }) => {
          const v = lookup(messages[locale], key)
          return dynamic ? !v || typeof v !== 'object' : typeof v !== 'string'
        })
        .map(({ file, key }) => `${file}: ${key}`)
      expect([...new Set(missing)]).toEqual([])
    })
  }
})

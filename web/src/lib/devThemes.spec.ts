// SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).
import { describe, it, expect } from 'vitest'
import { DEV_THEME_LIST, DEV_THEME_NAMES, resolveCrestIcons, resolveThemeAccent } from './devThemes'

function lum(hex: string): number {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((x) => (x <= 0.03928 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4))
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]
}
function contrast(a: string, b: string): number {
  const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p)
  return (x + 0.05) / (y + 0.05)
}

// The pages the accent is read on: white and light greys; the dark page and
// its cards (Apple's; the projects' dark grounds are these or darker).
const LIGHT = ['#ffffff', '#f2f2f7', '#f5f5f7']
const DARK = ['#1c1c1e', '#2c2c2e', '#232325']

describe('developer themes', () => {
  for (const t of DEV_THEME_LIST) {
    for (const dark of [false, true]) {
      it(`${t.id} (${dark ? 'dark' : 'light'}): text on the page and the button label read at 4.5:1`, () => {
        const r = resolveThemeAccent(t, dark)
        for (const bg of dark ? DARK : LIGHT) expect(contrast(r.text, bg)).toBeGreaterThanOrEqual(4.5)
        expect(contrast(r.onAccent, r.accent)).toBeGreaterThanOrEqual(4.5)
      })
    }
  }

  it('every theme has a name in every language and a full icon set', () => {
    for (const t of DEV_THEME_LIST) {
      for (const names of Object.values(DEV_THEME_NAMES)) expect(names[t.id]).toBeTruthy()
      const icons = resolveCrestIcons(t, false)
      expect(icons.touch).toMatch(/-touch-180\.png$/)
      expect(icons.maskable).toMatch(/-maskable-512\.png$/)
    }
    expect(resolveCrestIcons(DEV_THEME_LIST.find((t) => t.id === 'ink')!, false).svg).toContain('crest-bone')
  })
})

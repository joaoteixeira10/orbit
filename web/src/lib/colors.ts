// Colours for pickers: #rrggbb to hue / saturation / value and back.

export interface HSV {
  h: number // 0–360
  s: number // 0–1
  v: number // 0–1
}

export const isHex = (s: string) => /^#[0-9a-f]{6}$/i.test(s)

// hexOf reads what was typed ("#abc", "aabbcc"…) as #rrggbb ("" when it isn't one).
export function hexOf(s: string): string {
  let t = s.trim().replace(/^#/, '').toLowerCase()
  if (/^[0-9a-f]{3}$/.test(t)) t = t.replace(/./g, (c) => c + c)
  return /^[0-9a-f]{6}$/.test(t) ? `#${t}` : ''
}

export function hexToHsv(hex: string): HSV {
  const n = parseInt((hexOf(hex) || '#000000').slice(1), 16)
  const r = ((n >> 16) & 255) / 255
  const g = ((n >> 8) & 255) / 255
  const b = (n & 255) / 255
  const max = Math.max(r, g, b)
  const d = max - Math.min(r, g, b)
  let h = 0
  if (d) {
    if (max === r) h = ((g - b) / d) % 6
    else if (max === g) h = (b - r) / d + 2
    else h = (r - g) / d + 4
    h = (h * 60 + 360) % 360
  }
  return { h, s: max ? d / max : 0, v: max }
}

export function hsvToHex({ h, s, v }: HSV): string {
  const c = v * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - c
  const [r, g, b] = h < 60 ? [c, x, 0] : h < 120 ? [x, c, 0] : h < 180 ? [0, c, x] : h < 240 ? [0, x, c] : h < 300 ? [x, 0, c] : [c, 0, x]
  const to = (u: number) => Math.round((u + m) * 255).toString(16).padStart(2, '0')
  return `#${to(r)}${to(g)}${to(b)}`
}

// lightness 0–1 (to pick a readable mark on a swatch).
export function lightness(hex: string): number {
  const n = parseInt((hexOf(hex) || '#000000').slice(1), 16)
  return (0.299 * ((n >> 16) & 255) + 0.587 * ((n >> 8) & 255) + 0.114 * (n & 255)) / 255
}

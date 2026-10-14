// Page covers are stored as a short key in page.cover (≤ 64 chars): "g:<name>"
// for one of the gradient presets below, or "#RRGGBB" for a solid colour.
// Anything else (an unknown preset from a newer client) shows no cover.

export const GRADIENTS: Record<string, string> = {
  sunset: 'linear-gradient(135deg, #ff9a8b 0%, #ff6a88 55%, #ff99ac 100%)',
  peach: 'linear-gradient(135deg, #ffecd2 0%, #fcb69f 100%)',
  ocean: 'linear-gradient(135deg, #2e3192 0%, #1bffff 100%)',
  sky: 'linear-gradient(160deg, #a1c4fd 0%, #c2e9fb 100%)',
  forest: 'linear-gradient(135deg, #134e5e 0%, #71b280 100%)',
  aurora: 'linear-gradient(135deg, #00c9ff 0%, #92fe9d 100%)',
  dusk: 'linear-gradient(135deg, #2b5876 0%, #4e4376 100%)',
  lavender: 'linear-gradient(135deg, #e0c3fc 0%, #8ec5fc 100%)',
  graphite: 'linear-gradient(135deg, #434343 0%, #8e8e93 100%)',
}

// Apple system colours (light values; they read well in dark mode too).
export const SOLIDS = ['#FF3B30', '#FF9500', '#FFCC00', '#34C759', '#30B0C7', '#007AFF', '#5856D6', '#AF52DE', '#FF2D55', '#8E8E93']

export const COVER_KEYS = [...Object.keys(GRADIENTS).map((g) => `g:${g}`), ...SOLIDS]

// coverBackground resolves a stored key to a CSS background, or '' for none.
export function coverBackground(key: string | null | undefined): string {
  if (!key) return ''
  if (key.startsWith('g:')) return GRADIENTS[key.slice(2)] ?? ''
  if (/^#[0-9a-f]{6}$/i.test(key)) return key
  return ''
}

// randomCover picks a gradient, for "Add cover".
export function randomCover(): string {
  const names = Object.keys(GRADIENTS)
  return `g:${names[Math.floor(Math.random() * names.length)]}`
}

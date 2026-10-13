// The four curated "developer themes" (from PKI Management): a closed set, no
// free colour picker while the flag is on. Each locks the accent and brings a
// UBonito proxy crest (the blackletter "U" on the proxy mark — baked,
// font-independent SVGs under /dev-themes/, used for the in-app mark and the
// favicon) and the "UBonito" wordmark (self-hosted UnifrakturCook 700).

export type DevThemeId = 'blue' | 'gold' | 'red' | 'ink'

export interface DevTheme {
  id: DevThemeId
  name: string
  accent: string // dark-UI accent (Ink: bone)
  onAccent: string // foreground on the accent (button label etc.)
  accentStrong: string // 2nd gradient stop / hover
  // Ink is monochrome and flips with the light/dark UI; these are the light-UI
  // overrides. The other themes are the same in both.
  accentLight?: string
  onAccentLight?: string
  accentStrongLight?: string
  mark: string // wordmark glyph fill
  halo: string // wordmark glyph outline
  badge: string // mark background (theme card)
  ground: string // radial ground behind the theme card preview
  crest: string // crest SVG (mark + tab favicon), dark / colour variant
  crestLight?: string // light-UI crest (Ink flips to bone)
  // PNG crest for the iOS home-screen icon and the install manifest (iOS
  // doesn't take SVG there). Same variants as crest / crestLight.
  crestPng: string
  crestPngLight?: string
}

const crest = (v: string) => `/dev-themes/proxy-ubonito-${v}.svg`
const crestPng = (v: string) => `/dev-themes/proxy-ubonito-${v}-512.png`

export const DEV_THEMES: Record<DevThemeId, DevTheme> = {
  blue: {
    id: 'blue', name: 'Steel Blue',
    accent: '#2f6fe0', onAccent: '#ffffff', accentStrong: '#2257c4',
    mark: '#2f6fe0', halo: '#e6edfa', badge: '#0e1320',
    ground: 'radial-gradient(120% 120% at 50% 0%, #101a2e, #060a14)',
    crest: crest('blue'), crestPng: crestPng('blue'),
  },
  gold: {
    id: 'gold', name: 'Gold Leaf',
    accent: '#c79a3f', onAccent: '#1c1608', accentStrong: '#a87f2c',
    mark: '#cda44e', halo: '#f3e9cf', badge: '#13100b',
    ground: 'radial-gradient(120% 120% at 50% 0%, #1d1809, #0a0804)',
    crest: crest('gold'), crestPng: crestPng('gold'),
  },
  red: {
    id: 'red', name: 'Blood Red',
    accent: '#b4151c', onAccent: '#ffffff', accentStrong: '#8c0f14',
    mark: '#b01217', halo: '#f4f1ea', badge: '#13100f',
    ground: 'radial-gradient(120% 120% at 50% 0%, #1c0c0c, #0c0606)',
    crest: crest('red'), crestPng: crestPng('red'),
  },
  ink: {
    id: 'ink', name: 'Ink',
    // dark UI: bone button, ink text. light UI: ink button, bone text.
    accent: '#e7e2d8', onAccent: '#14130e', accentStrong: '#d3ccbd',
    accentLight: '#1b1a18', onAccentLight: '#f4f1ea', accentStrongLight: '#000000',
    mark: '#0c0c0c', halo: '#ffffff', badge: '#101010',
    ground: 'radial-gradient(120% 120% at 50% 0%, #242424, #0c0c0c)',
    crest: crest('dark'), crestLight: crest('bone'),
    crestPng: crestPng('dark'), crestPngLight: crestPng('bone'),
  },
}

export const DEV_THEME_LIST: DevTheme[] = [DEV_THEMES.blue, DEV_THEMES.gold, DEV_THEMES.red, DEV_THEMES.ink]

export interface ResolvedThemeAccent {
  accent: string
  onAccent: string
  accentStrong: string
}

// The accent of a theme, honouring Ink's light/dark flip.
export function resolveThemeAccent(t: DevTheme, isDark: boolean): ResolvedThemeAccent {
  const light = !isDark && t.accentLight
  return {
    accent: light ? t.accentLight! : t.accent,
    onAccent: light ? t.onAccentLight! : t.onAccent,
    accentStrong: light ? (t.accentStrongLight ?? t.accentStrong) : t.accentStrong,
  }
}

// The crest of a theme (Ink: bone in the light UI, dark in the dark UI).
export function resolveCrest(t: DevTheme, isDark: boolean): string {
  return !isDark && t.crestLight ? t.crestLight : t.crest
}

// The PNG crest, for the iOS home-screen icon and the install manifest.
export function resolveCrestPng(t: DevTheme, isDark: boolean): string {
  return !isDark && t.crestPngLight ? t.crestPngLight : t.crestPng
}

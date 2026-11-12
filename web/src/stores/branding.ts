import { defineStore } from 'pinia'
import { settingsApi } from '@/api'
import type { Branding } from '@/api/types'
import type { HolidaySetting } from '@/lib/holidayThemes'
import { DEV_THEMES, isDevThemeId, resolveCrest, resolveCrestIcons, resolveThemeAccent, type DevTheme, type DevThemeId } from '@/lib/devThemes'

// upsertLink returns the (created-if-missing) <link rel=...> in <head>.
function upsertLink(rel: string): HTMLLinkElement {
  let l = document.querySelector<HTMLLinkElement>(`link[rel="${rel}"]`)
  if (!l) {
    l = document.createElement('link')
    l.rel = rel
    document.head.appendChild(l)
  }
  return l
}

// iconType infers the MIME for an icon href (data URL or by extension), for the manifest.
function iconType(href: string): string {
  if (href.startsWith('data:')) return href.slice(5).match(/^[^;,]+/)?.[0] ?? ''
  if (href.endsWith('.svg')) return 'image/svg+xml'
  if (href.endsWith('.png')) return 'image/png'
  if (href.endsWith('.jpg') || href.endsWith('.jpeg')) return 'image/jpeg'
  if (href.endsWith('.webp')) return 'image/webp'
  if (href.endsWith('.ico')) return 'image/x-icon'
  return ''
}

// The default mark: Orbit's icon, also the favicon (web/public).
// Bound as a value, not a literal src, so the bundler serves the file as is.
export const DEFAULT_MARK = '/favicon.svg'

// The previous manifest blob URL, revoked when we rebuild it.
let manifestBlobURL: string | null = null

// Branding store: loads global branding and applies the resolved values to the
// document (accent CSS vars, favicon + home-screen icons + manifest, title). When
// developer themes are on, the accent is locked to a curated theme and the
// mark / wordmark / favicon come from the theme (each unless kept as the org's
// own). The rest of the app reads the resolved getters here, not raw fields.
export const useBrandingStore = defineStore('branding', {
  state: () => ({
    appName: 'Orbit',
    accentColor: '',
    // Per-user accent override (set by the prefs store); wins over everything.
    userAccentColor: '',
    logoDataURL: '',
    faviconDataURL: '',
    showAppName: true,
    showLogo: true,
    loginMessage: '',
    // Where Diagrams load the draw.io editor from.
    drawioUrl: 'https://embed.diagrams.net',
    oidcEnabled: false,
    // One "Sign in with …" button each (several identity providers).
    ssoProviders: [] as { id: string; name: string; kind: string }[],
    hideLocalLogin: false,
    passwordlessEnabled: true,
    autoSso: false,
    // Developer themes
    devThemesEnabled: false,
    devTheme: 'blue' as DevThemeId,
    assetSourceLogo: 'theme' as 'theme' | 'keep',
    assetSourceWordmark: 'theme' as 'theme' | 'keep',
    assetSourceFavicon: 'theme' as 'theme' | 'keep',
    // The owner lets each person pick their own theme (Settings → Appearance).
    devThemesPersonal: false,
    // Holiday themes (the owner's): off, auto (each in its season) or one now.
    holidayTheme: 'off' as HolidaySetting,
    // This person's choice ("" = as the instance, "none", or a theme id; set
    // by the prefs store, cached for the first paint, cleared at sign-out).
    userDevTheme: localStorage.getItem('orbit_dev_theme') || '',
    // Bumped when light/dark changes, so getters that read it (Ink's crest) recompute.
    dark: document.documentElement.classList.contains('dark'),
    loaded: false,
  }),
  getters: {
    // The instance's developer theme (the sign-in page's, everyone's default).
    instanceTheme(): DevTheme | null {
      return this.devThemesEnabled ? DEV_THEMES[this.devTheme] : null
    },
    // The active developer theme: this person's own when the owner allows it,
    // else the instance's; null for none.
    theme(): DevTheme | null {
      if (this.devThemesPersonal && this.userDevTheme) {
        if (this.userDevTheme === 'none') return null
        if (isDevThemeId(this.userDevTheme)) return DEV_THEMES[this.userDevTheme]
      }
      return this.instanceTheme
    },
    // What the sidebar/login mark should render.
    logoMode(): 'theme' | 'upload' | 'default' {
      if (this.theme && this.assetSourceLogo === 'theme') return 'theme'
      return this.logoDataURL ? 'upload' : 'default'
    },
    // The blackletter "UBonito" or the app name.
    wordmarkIsTheme(): boolean {
      return !!this.theme && this.assetSourceWordmark === 'theme'
    },
    // A developer theme is a block: its crest and wordmark always show,
    // overriding the per-element toggles (greyed out in Branding while on).
    showMark(): boolean {
      return this.logoMode === 'theme' || this.showLogo
    },
    showName(): boolean {
      return this.wordmarkIsTheme || this.showAppName
    },
    // The theme crest for the sidebar / sign-in mark (Ink flips with light/dark).
    crestUrl(): string {
      if (this.logoMode !== 'theme' || !this.theme) return ''
      return resolveCrest(this.theme, this.dark)
    },
  },
  actions: {
    async load() {
      try {
        const b = await settingsApi.branding()
        this.apply(b)
      } catch {
        /* keep defaults */
      } finally {
        this.loaded = true
      }
    },
    apply(b: Branding) {
      this.appName = b.app_name || 'Orbit'
      this.accentColor = b.accent_color
      this.logoDataURL = b.logo_data_url
      this.faviconDataURL = b.favicon_data_url
      this.showAppName = b.show_app_name
      this.showLogo = b.show_logo
      this.loginMessage = b.login_message
      this.oidcEnabled = b.oidc_enabled
      this.ssoProviders = b.sso_providers ?? []
      this.hideLocalLogin = b.hide_local_login
      this.passwordlessEnabled = b.passwordless_enabled
      this.autoSso = b.auto_sso
      this.devThemesEnabled = !!b.developer_themes_enabled
      this.devTheme = b.developer_theme || 'blue'
      this.assetSourceLogo = b.asset_source_logo || 'theme'
      this.assetSourceWordmark = b.asset_source_wordmark || 'theme'
      this.assetSourceFavicon = b.asset_source_favicon || 'theme'
      this.devThemesPersonal = !!b.developer_themes_personal
      this.holidayTheme = (b.holiday_theme || 'off') as HolidaySetting
      if (b.drawio_url) this.drawioUrl = b.drawio_url

      document.title = this.appName
      this.applyAccent()
      this.applyFavicon()
    },

    // applyAccent sets the accent CSS variables from the resolved accent: a
    // personal accent, else a developer theme's, else the org accent, else the
    // stylesheet defaults.
    applyAccent() {
      const root = document.documentElement
      this.dark = root.classList.contains('dark')
      // A developer theme also washes the top of the page in its colour.
      root.classList.toggle('dev-theme', !!this.theme)
      const setRamp = (accent: string, strong: string, fg: string, text = accent) => {
        root.style.setProperty('--accent', accent)
        root.style.setProperty('--accent-strong', strong)
        // Soft accent fills (active nav item, chips) track the accent too.
        root.style.setProperty('--accent-soft', `color-mix(in srgb, ${accent} 14%, transparent)`)
        root.style.setProperty('--accent-bg', `color-mix(in srgb, ${accent} 16%, transparent)`)
        root.style.setProperty('--accent-text', text)
        root.style.setProperty('--accent-fg', fg)
        root.style.setProperty('--ring', `color-mix(in srgb, ${accent} 38%, transparent)`)
      }
      if (this.userAccentColor) {
        // Personal accent wins over the org accent and any developer theme (the
        // theme still owns the mark / wordmark / favicon — only the colour changes).
        setRamp(this.userAccentColor, `color-mix(in srgb, ${this.userAccentColor} 78%, #000)`, '#fff')
      } else if (this.theme) {
        const r = resolveThemeAccent(this.theme, this.dark)
        setRamp(r.accent, r.accentStrong, r.onAccent, r.text)
      } else if (this.accentColor) {
        setRamp(this.accentColor, `color-mix(in srgb, ${this.accentColor} 78%, #000)`, '#fff')
      } else {
        // Default theme: clear overrides (CSS tokens take over; label = white).
        for (const v of ['--accent', '--accent-strong', '--accent-soft', '--accent-bg', '--accent-text', '--accent-fg', '--ring'])
          root.style.removeProperty(v)
      }
    },

    // applyFavicon points the browser-tab favicon, the iOS "Add to Home Screen"
    // apple-touch-icon, and the web-app-manifest icon (Android install) at the
    // *resolved* logo, so an installed/home-screen app uses the current branding
    // instead of a screenshot or default. Also sets theme-color to the accent.
    applyFavicon() {
      const isDark = document.documentElement.classList.contains('dark')
      // href: the tab favicon (SVG is fine for browsers). appleHref: a PNG for the
      // iOS home-screen / PWA icon — iOS does NOT accept SVG there (it would fall
      // back to a generated letter tile).
      // manifestHref: the install icon (512 px for the default icon).
      let href = ''
      let appleHref = ''
      let manifestHref = ''
      // A theme's crest comes as a full icon set (the owner's pack): a square,
      // opaque one for the iPhone / iPad home screen (iOS rounds it; the
      // rounded badge would get black corners) and 192 / 512 / maskable for
      // the install manifest.
      let themeIcons: ReturnType<typeof resolveCrestIcons> | null = null
      if (this.theme && this.assetSourceFavicon === 'theme') {
        themeIcons = resolveCrestIcons(this.theme, isDark)
        href = themeIcons.svg
        appleHref = themeIcons.touch
        manifestHref = themeIcons.png512
      } else if (this.faviconDataURL) {
        href = this.faviconDataURL
        appleHref = this.faviconDataURL // an uploaded favicon is used as-is
        manifestHref = this.faviconDataURL
      } else {
        href = '/favicon.svg' // product default
        appleHref = '/favicon-180.png'
        manifestHref = '/favicon-512.png'
      }

      upsertLink('icon').href = href
      // iOS reads apple-touch-icon from the live DOM at add-to-home time.
      upsertLink('apple-touch-icon').href = appleHref

      // Resolved accent → manifest theme + the address-bar theme-color (a
      // personal accent wins, matching applyAccent).
      let meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
      if (!meta) {
        meta = document.createElement('meta')
        meta.name = 'theme-color'
        document.head.appendChild(meta)
      }
      // The address bar matches the page, as Apple's apps do.
      meta.content = isDark ? '#1c1c1e' : '#ffffff'

      // Dynamic web manifest (Android install). Built as a blob so it reflects
      // the current name/icon/accent without a server round-trip; it replaces
      // the static /manifest.webmanifest. URLs must be absolute: inside a blob:
      // manifest, '/' doesn't resolve against the site. (CSP: manifest-src blob:.)
      const abs = (u: string) => new URL(u, location.origin).href
      // PNG icons (iOS/Android installers don't reliably take SVG manifest icons).
      const icons = themeIcons
        ? [
            { src: abs(themeIcons.png192), sizes: '192x192', type: 'image/png', purpose: 'any' },
            { src: abs(themeIcons.png512), sizes: '512x512', type: 'image/png', purpose: 'any' },
            { src: abs(themeIcons.maskable), sizes: '512x512', type: 'image/png', purpose: 'maskable' },
          ]
        : manifestHref === '/favicon-512.png'
        ? [
            { src: abs('/favicon-192.png'), sizes: '192x192', type: 'image/png', purpose: 'any' },
            { src: abs('/favicon-512.png'), sizes: '512x512', type: 'image/png', purpose: 'any' },
          ]
        : [{
            src: abs(manifestHref),
            sizes: manifestHref.endsWith('512.png') ? '512x512' : 'any',
            type: iconType(manifestHref) || undefined,
            purpose: 'any',
          }]
      const manifest = {
        id: abs('/'),
        name: this.appName,
        short_name: this.appName,
        start_url: abs('/'),
        scope: abs('/'),
        display: 'standalone',
        background_color: isDark ? '#1c1c1e' : '#ffffff',
        theme_color: isDark ? '#1c1c1e' : '#ffffff',
        icons,
        // Android's share sheet lists Orbit; what's shared becomes a Quick Note.
        share_target: { action: abs('/share'), method: 'GET', params: { title: 'title', text: 'text', url: 'url' } },
      }
      if (manifestBlobURL) URL.revokeObjectURL(manifestBlobURL)
      manifestBlobURL = URL.createObjectURL(new Blob([JSON.stringify(manifest)], { type: 'application/manifest+json' }))
      upsertLink('manifest').href = manifestBlobURL
    },
  },
})

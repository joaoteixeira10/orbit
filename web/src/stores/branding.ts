import { defineStore } from 'pinia'
import { settingsApi } from '@/api'
import type { Branding } from '@/api/types'

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
// document (accent CSS vars, favicon + home-screen icons + manifest, title).
// The rest of the app reads the resolved getters here, not raw fields.
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
    oidcEnabled: false,
    hideLocalLogin: false,
    passwordlessEnabled: true,
    autoSso: false,
    // Bumped when light/dark changes, so getters that read it recompute.
    dark: document.documentElement.classList.contains('dark'),
    loaded: false,
  }),
  getters: {
    // What the sidebar/login mark should render.
    logoMode(): 'upload' | 'default' {
      return this.logoDataURL ? 'upload' : 'default'
    },
    showMark(): boolean {
      return this.showLogo
    },
    showName(): boolean {
      return this.showAppName
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
      this.hideLocalLogin = b.hide_local_login
      this.passwordlessEnabled = b.passwordless_enabled
      this.autoSso = b.auto_sso

      document.title = this.appName
      this.applyAccent()
      this.applyFavicon()
    },

    // applyAccent sets the accent CSS variables from the resolved accent: a
    // personal accent, else the org accent, else the stylesheet defaults.
    applyAccent() {
      const root = document.documentElement
      this.dark = root.classList.contains('dark')
      const setRamp = (accent: string, strong: string, fg: string) => {
        root.style.setProperty('--accent', accent)
        root.style.setProperty('--accent-strong', strong)
        // Soft accent fills (active nav item, chips) track the accent too.
        root.style.setProperty('--accent-soft', `color-mix(in srgb, ${accent} 14%, transparent)`)
        root.style.setProperty('--accent-bg', `color-mix(in srgb, ${accent} 16%, transparent)`)
        root.style.setProperty('--accent-text', accent)
        root.style.setProperty('--accent-fg', fg)
        root.style.setProperty('--ring', `color-mix(in srgb, ${accent} 38%, transparent)`)
      }
      if (this.userAccentColor) {
        // Personal accent wins over the org accent.
        setRamp(this.userAccentColor, `color-mix(in srgb, ${this.userAccentColor} 78%, #000)`, '#fff')
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
      if (this.faviconDataURL) {
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
      const icons = manifestHref === '/favicon-512.png'
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
      }
      if (manifestBlobURL) URL.revokeObjectURL(manifestBlobURL)
      manifestBlobURL = URL.createObjectURL(new Blob([JSON.stringify(manifest)], { type: 'application/manifest+json' }))
      upsertLink('manifest').href = manifestBlobURL
    },
  },
})

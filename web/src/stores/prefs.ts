import { defineStore } from 'pinia'
import { api } from '@/lib/api'

export type Theme = 'light' | 'dark' | 'system'
// How a server change is confirmed: the per-proxy preview, or just the essentials.
export type ChangeDetails = 'detailed' | 'compact'

interface ServerPrefs {
  timezone: string
  theme: Theme
  accent_color: string
  change_details?: ChangeDetails
}

// Push the per-user accent into the branding store and repaint. Lazy import
// avoids a store cycle (branding never imports prefs).
function pushAccent(color: string) {
  void import('@/stores/branding').then((m) => {
    const b = m.useBrandingStore()
    b.userAccentColor = color
    b.applyAccent()
  })
}

// Per-user UI preferences. Backed by the server (UserPrefs) so they follow the
// user across devices; localStorage is a cache for instant first paint.
export const usePrefsStore = defineStore('prefs', {
  state: () => ({
    theme: (localStorage.getItem('orbit_theme') as Theme) || 'system',
    // "local" = use the device/browser timezone (the default).
    timezone: localStorage.getItem('orbit_timezone') || 'local',
    // Personal accent override ("" = follow the org accent).
    accentColor: localStorage.getItem('orbit_accent') || '',
    changeDetails: (localStorage.getItem('orbit_change_details') as ChangeDetails) || 'detailed',
  }),
  actions: {
    isDark(): boolean {
      if (this.theme === 'dark') return true
      if (this.theme === 'light') return false
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    },
    applyTheme() {
      document.documentElement.classList.toggle('dark', this.isDark())
      // Re-resolve accent + favicon (the manifest background follows light/dark).
      // Lazy import avoids a store cycle and tolerates branding not yet loaded.
      void import('@/stores/branding').then((m) => {
        const b = m.useBrandingStore()
        b.applyAccent()
        b.applyFavicon()
      })
    },

    // Apply server values without writing back (used on login/restore).
    hydrateFromServer(p: ServerPrefs) {
      this.theme = p.theme
      this.timezone = p.timezone
      this.accentColor = p.accent_color || ''
      this.changeDetails = p.change_details || 'detailed'
      localStorage.setItem('orbit_change_details', this.changeDetails)
      localStorage.setItem('orbit_theme', p.theme)
      localStorage.setItem('orbit_timezone', p.timezone)
      localStorage.setItem('orbit_accent', this.accentColor)
      pushAccent(this.accentColor)
      this.applyTheme()
    },

    async setTheme(t: Theme) {
      this.theme = t
      localStorage.setItem('orbit_theme', t)
      this.applyTheme()
      await this.persist({ theme: t })
    },
    async setTimezone(tz: string) {
      this.timezone = tz
      localStorage.setItem('orbit_timezone', tz)
      await this.persist({ timezone: tz })
    },
    async setChangeDetails(v: ChangeDetails) {
      this.changeDetails = v
      localStorage.setItem('orbit_change_details', v)
      await this.persist({ change_details: v })
    },
    // Set ("" clears) the personal accent; repaints immediately + persists.
    async setAccentColor(color: string) {
      this.accentColor = color
      localStorage.setItem('orbit_accent', color)
      pushAccent(color)
      await this.persist({ accent_color: color })
    },

    // Push a partial update to the server (best-effort; local state already set).
    async persist(patch: Partial<ServerPrefs>) {
      try {
        await api('/me/prefs', { method: 'PUT', body: JSON.stringify(patch) })
      } catch {
        /* offline / not yet authed — local cache still applies */
      }
    },
  },
})

import { defineStore } from 'pinia'
import { api } from '@/lib/api'

export type Theme = 'light' | 'dark' | 'system'
export type TimeFormat = '24h' | '12h'

interface ServerPrefs {
  timezone: string
  theme: Theme
  accent_color: string
  week_start?: 0 | 1
  time_format?: TimeFormat
  device_tz?: string
}

function browserTZ(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

// The default first day of the week and clock for this browser's language.
function localeDefaults(): { weekStart: 0 | 1; timeFormat: TimeFormat } {
  const lang = navigator.language || 'en'
  const us = /^en-(US|CA)|^pt-BR/.test(lang)
  return { weekStart: us ? 0 : 1, timeFormat: /^en-(US|CA|AU)/.test(lang) ? '12h' : '24h' }
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
    weekStart: (Number(localStorage.getItem('orbit_week_start') ?? localeDefaults().weekStart) === 0 ? 0 : 1) as 0 | 1,
    timeFormat: ((localStorage.getItem('orbit_time_format') as TimeFormat) || localeDefaults().timeFormat) as TimeFormat,
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
      this.weekStart = p.week_start === 0 ? 0 : 1
      this.timeFormat = p.time_format === '12h' ? '12h' : '24h'
      localStorage.setItem('orbit_week_start', String(this.weekStart))
      localStorage.setItem('orbit_time_format', this.timeFormat)
      localStorage.setItem('orbit_theme', p.theme)
      localStorage.setItem('orbit_timezone', p.timezone)
      localStorage.setItem('orbit_accent', this.accentColor)
      pushAccent(this.accentColor)
      this.applyTheme()
      // Reminders need the zone this browser is in (while the setting is "local").
      const tz = browserTZ()
      if (tz && tz !== p.device_tz) void this.persist({ device_tz: tz })
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
    async setWeekStart(v: 0 | 1) {
      this.weekStart = v
      localStorage.setItem('orbit_week_start', String(v))
      await this.persist({ week_start: v })
    },
    async setTimeFormat(v: TimeFormat) {
      this.timeFormat = v
      localStorage.setItem('orbit_time_format', v)
      await this.persist({ time_format: v })
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

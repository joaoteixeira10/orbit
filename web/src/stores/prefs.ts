import { defineStore } from 'pinia'
import { currentLocale } from '@/i18n'
import { api } from '@/lib/api'
import { mediaPrefs } from '@/lib/media'

export type Theme = 'light' | 'dark' | 'system'
export type TimeFormat = '24h' | '12h'
// How pages are edited: visually (blocks) or as Markdown source.
export type PageEditorMode = 'visual' | 'markdown'
// Editors that may keep their own look whatever Orbit's theme.
export type EditorKind = 'sheets' | 'diagrams' | 'whiteboards'
export type EditorTheme = 'follow' | 'light' | 'dark'
type EditorThemes = Partial<Record<EditorKind, 'light' | 'dark'>>

function readEditorThemes(): EditorThemes {
  try {
    return JSON.parse(localStorage.getItem('orbit_editor_themes') || '{}') as EditorThemes
  } catch {
    return {}
  }
}

interface ServerPrefs {
  timezone: string
  theme: Theme
  accent_color: string
  week_start?: 0 | 1
  time_format?: TimeFormat
  device_tz?: string
  locale?: string
  page_editor?: PageEditorMode
  dev_theme?: string // "" = as the instance | none | a developer theme id
  remote_images?: 'ask' | 'always'
  sketch_colors?: string[] // their own palette in Quick Draw
  editor_themes?: EditorThemes
  calendar_colors?: Record<string, string>
  focus_lengths?: number[] // own focus timer lengths, minutes
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

// Push this person's developer theme into the branding store and repaint
// (cached for the first paint; cleared at sign-out).
function pushDevTheme(choice: string) {
  if (choice) localStorage.setItem('orbit_dev_theme', choice)
  else localStorage.removeItem('orbit_dev_theme')
  void import('@/stores/branding').then((m) => {
    const b = m.useBrandingStore()
    b.userDevTheme = choice
    b.applyAccent()
    b.applyFavicon()
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
    sketchColors: [] as string[],
    editorThemes: readEditorThemes(),
    // Own colours for calendars (a workspace id, list:<id>, sub:<id>, holidays).
    calendarColors: {} as Record<string, string>,
    // Own lengths for the focus timer (minutes), besides 15, 25 and 50.
    focusLengths: [] as number[],
    weekStart: (Number(localStorage.getItem('orbit_week_start') ?? localeDefaults().weekStart) === 0 ? 0 : 1) as 0 | 1,
    timeFormat: ((localStorage.getItem('orbit_time_format') as TimeFormat) || localeDefaults().timeFormat) as TimeFormat,
    pageEditor: (localStorage.getItem('orbit_page_editor') === 'markdown' ? 'markdown' : 'visual') as PageEditorMode,
    // Own developer theme ("" = as the instance), when the owner allows it.
    devTheme: localStorage.getItem('orbit_dev_theme') || '',
    // Images from other sites in pages: after a click (ask) or at once.
    remoteImages: (localStorage.getItem('orbit_remote_images') === 'always' ? 'always' : 'ask') as 'ask' | 'always',
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
      this.pageEditor = p.page_editor === 'markdown' ? 'markdown' : 'visual'
      localStorage.setItem('orbit_page_editor', this.pageEditor)
      localStorage.setItem('orbit_week_start', String(this.weekStart))
      localStorage.setItem('orbit_time_format', this.timeFormat)
      localStorage.setItem('orbit_theme', p.theme)
      localStorage.setItem('orbit_timezone', p.timezone)
      localStorage.setItem('orbit_accent', this.accentColor)
      pushAccent(this.accentColor)
      this.devTheme = p.dev_theme || ''
      pushDevTheme(this.devTheme)
      this.remoteImages = p.remote_images === 'always' ? 'always' : 'ask'
      this.sketchColors = p.sketch_colors ?? []
      this.editorThemes = p.editor_themes ?? {}
      this.calendarColors = p.calendar_colors ?? {}
      this.focusLengths = p.focus_lengths ?? []
      localStorage.setItem('orbit_editor_themes', JSON.stringify(this.editorThemes))
      mediaPrefs.remoteImages = this.remoteImages
      localStorage.setItem('orbit_remote_images', this.remoteImages)
      this.applyTheme()
      // Reminders need the zone this browser is in (while the setting is "local").
      const tz = browserTZ()
      if (tz && tz !== p.device_tz) void this.persist({ device_tz: tz })
      // The server words push notifications in the language last used here.
      const lang = currentLocale()
      if (lang !== p.locale) void this.persist({ locale: lang })
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
    async setPageEditor(v: PageEditorMode) {
      this.pageEditor = v
      localStorage.setItem('orbit_page_editor', v)
      await this.persist({ page_editor: v })
    },
    async setTimeFormat(v: TimeFormat) {
      this.timeFormat = v
      localStorage.setItem('orbit_time_format', v)
      await this.persist({ time_format: v })
    },
    async setRemoteImages(v: 'ask' | 'always') {
      this.remoteImages = v
      mediaPrefs.remoteImages = v
      localStorage.setItem('orbit_remote_images', v)
      await this.persist({ remote_images: v })
    },
    // Pick this person's developer theme ("" = as the instance, "none").
    async setDevTheme(choice: string) {
      this.devTheme = choice
      pushDevTheme(choice)
      await this.persist({ dev_theme: choice })
    },
    // An editor's look: as Orbit's, or always light / dark.
    editorTheme(kind: EditorKind): EditorTheme {
      return this.editorThemes[kind] ?? 'follow'
    },
    // editorDark: is this editor shown dark now?
    editorDark(kind: EditorKind): boolean {
      const own = this.editorThemes[kind]
      return own ? own === 'dark' : document.documentElement.classList.contains('dark')
    },
    // editorClass: the class that keeps an editor in its own look ('' = Orbit's).
    editorClass(kind: EditorKind): string {
      const own = this.editorThemes[kind]
      return own ? `theme-${own}` : ''
    },
    async setEditorTheme(kind: EditorKind, v: EditorTheme) {
      const next = { ...this.editorThemes }
      if (v === 'follow') delete next[kind]
      else next[kind] = v
      this.editorThemes = next
      localStorage.setItem('orbit_editor_themes', JSON.stringify(next))
      await this.persist({ editor_themes: { [kind]: v } as any })
    },
    // A calendar's colour for this person ("" = the calendar's own again).
    async setCalendarColor(key: string, color: string) {
      const next = { ...this.calendarColors }
      if (color) next[key] = color
      else delete next[key]
      this.calendarColors = next
      await this.persist({ calendar_colors: { [key]: color } })
    },
    // Their own lengths for the focus timer.
    async setFocusLengths(lengths: number[]) {
      this.focusLengths = lengths
      await this.persist({ focus_lengths: lengths })
    },
    // Their own palette in Quick Draw.
    async setSketchColors(colors: string[]) {
      this.sketchColors = colors
      await this.persist({ sketch_colors: colors })
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

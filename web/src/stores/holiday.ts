import { defineStore } from 'pinia'
import { useBrandingStore } from '@/stores/branding'
import { cssVars, resolveHoliday, type Holiday, type Look, type Mode } from '@/lib/holidayThemes'
import { resolveSeason, type Season } from '@/lib/seasonThemes'

// The custom properties apply() set last, removed before the next.
let applied: string[] = []

// Holiday and season themes: which one shows — the owner's settings (or this
// person's own, when the owner lets each choose): off, auto (the one whose
// days today is in) or one of them now — and its colours
// as --h-* on <html>, in light or dark (styles/holiday.css: the sidebar, the
// main column and every window). A holiday in its days wins over the season;
// only a holiday has its mark, countdown and capture line; both hang
// things on Home and dress a card's corner. `now`
// ticks every minute (the shell), so a theme starts and ends, and New Year's
// hours count down.
export const useHolidayStore = defineStore('holiday', {
  state: () => ({ now: new Date() }),
  getters: {
    active(): Holiday | null {
      const b = useBrandingStore()
      if (b.impersonating) return null // the impersonation theme instead
      const set = b.effectiveHolidayTheme
      return set === 'off' ? null : resolveHoliday(set, this.now)
    },
    // The season, while no holiday shows.
    season(): Season | null {
      if (this.active) return null
      const b = useBrandingStore()
      if (b.impersonating) return null
      const set = b.effectiveSeasonTheme
      return set === 'off' ? null : resolveSeason(set, this.now)
    },
    // What colours the app now: the holiday, else the season.
    look(): Look | null {
      return this.active ?? this.season
    },
    mode(): Mode {
      return useBrandingStore().dark ? 'dark' : 'light'
    },
  },
  actions: {
    tick() {
      this.now = new Date()
    },
    apply() {
      const root = document.documentElement
      for (const k of applied) root.style.removeProperty(k)
      applied = []
      const h = this.look
      root.classList.toggle('holiday', !!h)
      if (!h) {
        delete root.dataset.holiday
        return
      }
      root.dataset.holiday = h.id
      const a = h.art
      const vars: Record<string, string> = {
        ...cssVars(h, this.mode),
        '--h-tile-bg': a.tile.background,
        '--h-tile-overlay': a.tile.overlay,
        '--h-tile-icon': a.tileIcon,
        '--h-box-calendar': a.iconBoxes.calendar,
        '--h-box-quickNotes': a.iconBoxes.quickNotes,
        '--h-box-quickDraw': a.iconBoxes.quickDraw,
        '--h-box-myFiles': a.iconBoxes.myFiles,
        '--h-box-workspace': a.iconBoxes.workspace,
        '--h-box-avatar': a.iconBoxes.avatar,
        '--h-done-in': a.done[0],
        '--h-done-out': a.done[1],
        '--h-done-glow': a.done[2],
        '--h-festive': a.festiveBar,
      }
      for (const [k, v] of Object.entries(vars)) {
        root.style.setProperty(k, v)
        applied.push(k)
      }
    },
    // clear takes the theme off (signing out: the sign-in page stays plain).
    clear() {
      const root = document.documentElement
      for (const k of applied) root.style.removeProperty(k)
      applied = []
      root.classList.remove('holiday')
      delete root.dataset.holiday
    },
  },
})

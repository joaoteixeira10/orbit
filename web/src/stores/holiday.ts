import { defineStore } from 'pinia'
import { useBrandingStore } from '@/stores/branding'
import { cssVars, resolveHoliday, type Holiday, type Mode } from '@/lib/holidayThemes'

// The custom properties apply() set last, removed before the next.
let applied: string[] = []

// Holiday themes: which one shows — the owner's setting: off, auto (the one
// whose season today is in) or one of them now — and its colours and art as
// --h-* on <html>, in light or dark. The sidebar and Home read them
// (styles/holiday.css); nothing else changes. `now` ticks every minute (the
// shell), so a season starts and ends, and New Year's hours count down.
export const useHolidayStore = defineStore('holiday', {
  state: () => ({ now: new Date() }),
  getters: {
    active(): Holiday | null {
      const b = useBrandingStore()
      return b.holidayTheme === 'off' ? null : resolveHoliday(b.holidayTheme, this.now)
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
      const h = this.active
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

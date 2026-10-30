// The side panel (components/SideRail.vue): open or not (remembered; open
// on a wide window until closed), and what the page being read puts in it —
// its text (for the outline and notes), where it's shown (to jump to), and
// how to tick a box.
import { reactive } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'

const saved = (() => {
  try {
    return localStorage.getItem('orbit_outline')
  } catch {
    return null
  }
})()

export const rail = reactive<{
  open: boolean
  page: { content: string; root: HTMLElement | null; canTick: boolean; toggle: (i: number) => void } | null
}>({
  // Open by itself only where it leaves the screen room (1440px+).
  open: saved === '1' || (saved === null && typeof window !== 'undefined' && window.matchMedia('(min-width: 1440px)').matches),
  page: null,
})

export function setRailOpen(on: boolean) {
  rail.open = on
  try {
    localStorage.setItem('orbit_outline', on ? '1' : '0')
  } catch {
    /* private mode */
  }
}

// railFocus is what the panel is about on this screen — a page, a sheet, a
// diagram, a whiteboard, a task list or a folder ("": nothing; then there's
// no panel).
export function railFocus(route: RouteLocationNormalizedLoaded): string {
  const p = route.params
  if (typeof p.ws !== 'string') return ''
  if (typeof p.page === 'string') return p.page
  if (['ws-sheet', 'ws-diagram', 'ws-whiteboard', 'ws-form'].includes(String(route.name)) && typeof p.id === 'string') return p.id
  if ((route.name === 'ws-tasks' || route.name === 'ws-board') && typeof p.list === 'string' && p.list !== 'all') return p.list
  if (route.name !== 'ws-graph' && typeof route.query.folder === 'string' && route.query.folder) return route.query.folder
  return ''
}

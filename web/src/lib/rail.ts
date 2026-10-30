// The side panel (components/SideRail.vue): open or not (remembered; open
// on a wide window until closed), and what the page being read puts in it —
// its text (for the outline and notes), where it's shown (to jump to), and
// how to tick a box.
import { reactive } from 'vue'

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

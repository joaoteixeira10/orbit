import { ref, watch, onMounted, onUnmounted, type Ref } from 'vue'
import { usePrefsStore } from '@/stores/prefs'

// SVG presentation attributes (stroke/fill) don't resolve CSS var(), so charts
// need concrete resolved colors. usePalette() reads them from the computed
// styles and re-reads on a rAF whenever the theme flips, so charts re-theme too.
export interface Palette {
  accent: string
  accentStrong: string
  ok: string
  warn: string
  danger: string
  violet: string
  text: string
  text2: string
  text3: string
  textFaint: string
  surface: string
  surface3: string
  grid: string
  borderStrong: string
}

function read(): Palette {
  const s = getComputedStyle(document.documentElement)
  const v = (n: string) => s.getPropertyValue(n).trim()
  return {
    accent: v('--accent'),
    accentStrong: v('--accent-strong'),
    ok: v('--ok'),
    warn: v('--warn'),
    danger: v('--danger'),
    violet: v('--violet'),
    text: v('--text'),
    text2: v('--text-2'),
    text3: v('--text-3'),
    textFaint: v('--text-faint'),
    surface: v('--surface'),
    surface3: v('--surface-3'),
    grid: v('--grid-line'),
    borderStrong: v('--border-strong'),
  }
}

export function usePalette(): Ref<Palette> {
  const prefs = usePrefsStore()
  const pal = ref<Palette>(read())
  let raf = 0
  const refresh = () => {
    cancelAnimationFrame(raf)
    // Re-read after the class toggle + paint so the new values are resolved.
    raf = requestAnimationFrame(() => {
      pal.value = read()
    })
  }
  const stop = watch(() => prefs.theme, refresh)
  const mq = window.matchMedia('(prefers-color-scheme: dark)')
  const onMq = () => {
    if (prefs.theme === 'system') refresh()
  }
  onMounted(() => {
    mq.addEventListener('change', onMq)
    refresh()
  })
  onUnmounted(() => {
    stop()
    mq.removeEventListener('change', onMq)
    cancelAnimationFrame(raf)
  })
  return pal
}

import { shallowRef } from 'vue'

// useFlash tracks keys that should briefly highlight after a live update, so the
// user notices what just changed or appeared. Call flash(keys) when a live event
// brings in new/changed items; bind isFlashing(key) to a highlight class. Each
// key clears itself after `durationMs` (re-flashing a key restarts its timer).
// Keys are numbers (table-row ids) by default, or strings (e.g. nav item names).
// durationMs must match the .row-flash / .nav-flash animation in styles/main.css.
export function useFlash<K = string>(durationMs = 600) {
  const flashing = shallowRef<Set<K>>(new Set<K>())
  const timers = new Map<K, ReturnType<typeof setTimeout>>()

  function flash(keys: K[]) {
    if (!keys.length) return
    const next = new Set(flashing.value)
    for (const key of keys) {
      next.add(key)
      clearTimeout(timers.get(key))
      timers.set(
        key,
        setTimeout(() => {
          const s = new Set(flashing.value)
          s.delete(key)
          flashing.value = s
          timers.delete(key)
        }, durationMs),
      )
    }
    flashing.value = next
  }

  function isFlashing(key: K): boolean {
    return flashing.value.has(key)
  }

  return { flash, isFlashing }
}

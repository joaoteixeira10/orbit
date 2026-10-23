import { defineStore } from 'pinia'
import { api } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'

// The focus timer: the running session (whichever workspace), what was
// focused on today, and a clock that ticks while one runs. The server keeps
// the session, so it carries on across tabs and devices; when its time is
// up this tab ends it at once (the scheduler would within a minute).
export interface FocusSession {
  id: string
  workspace_id: string
  task_id?: string
  task_title?: string
  minutes: number
  started_at: string
  ended_at?: string | null
  spent: number
}
interface State {
  running: FocusSession | null
  today: FocusSession[]
  minutes: number
}

const j = (b: unknown) => JSON.stringify(b)
export const focusApi = {
  get: (ws?: string) => api<State>(`/me/focus${ws ? `?ws=${ws}` : ''}`),
  start: (b: { workspace_id: string; task_id?: string; minutes: number }) => api<State>('/me/focus', { method: 'POST', body: j(b) }),
  stop: () => api<State>('/me/focus/stop', { method: 'POST' }),
}

let tick: ReturnType<typeof setInterval> | null = null

export const useFocusStore = defineStore('focus', {
  state: () => ({ running: null as FocusSession | null, today: [] as FocusSession[], minutes: 0, now: Date.now(), ws: '' }),
  getters: {
    // Seconds left in the running session (0 when none).
    left(s): number {
      if (!s.running) return 0
      const end = new Date(s.running.started_at).getTime() + s.running.minutes * 60000
      return Math.max(0, Math.round((end - s.now) / 1000))
    },
    progress(s): number {
      if (!s.running) return 0
      const total = s.running.minutes * 60000
      const gone = s.now - new Date(s.running.started_at).getTime()
      return Math.min(1, Math.max(0, gone / total))
    },
  },
  actions: {
    take(st: State) {
      this.running = st.running
      this.today = st.today
      this.minutes = st.minutes
      this.clock()
    },
    async load(ws?: string) {
      if (ws !== undefined) this.ws = ws
      this.take(await focusApi.get(this.ws || undefined))
    },
    async start(ws: string, taskId: string, minutes: number) {
      this.ws = ws
      this.take(await focusApi.start({ workspace_id: ws, task_id: taskId || undefined, minutes }))
    },
    async stop() {
      this.take(await focusApi.stop())
    },
    // clock ticks every second while a session runs; at zero it ends it.
    clock() {
      if (tick) clearInterval(tick)
      tick = null
      this.now = Date.now()
      if (!this.running) return
      tick = setInterval(() => {
        this.now = Date.now()
        if (this.running && this.left === 0) {
          clearInterval(tick!)
          tick = null
          window.dispatchEvent(new CustomEvent('orbit:focus-done', { detail: this.running }))
          void this.stop().catch(() => {})
        }
      }, 1000)
    },
  },
})

let wired = false
// wireFocusLive keeps the timer in step with other tabs and devices.
export function wireFocusLive() {
  if (wired) return
  wired = true
  const s = useFocusStore()
  void s.load().catch(() => {})
  onLiveEvent('focus', () => void s.load().catch(() => {}))
}

// clock formats seconds as m:ss.
export function clockOf(sec: number): string {
  const m = Math.floor(sec / 60)
  return `${m}:${String(sec % 60).padStart(2, '0')}`
}

// durationOf writes minutes as "25 min" or "1 h 25 min".
export function durationOf(min: number): string {
  if (min < 60) return `${min} min`
  const h = Math.floor(min / 60)
  const m = min % 60
  return m ? `${h} h ${m} min` : `${h} h`
}

// Who's on a page, a sheet, a diagram or a whiteboard right now: this tab
// says it's here every 15 seconds (editing, or looking) and that it left
// when it goes; the server tells everyone there when someone comes, goes, or
// starts or stops editing. A tab counts as gone 40 seconds after its last word.
import { onUnmounted, ref, watch, type Ref } from 'vue'
import { api, CLIENT_ID } from './api'
import { onLiveEvent } from './live'

export type PresenceKind = 'page' | 'board' | 'sheet' | 'diagram'

export interface Present {
  client: string
  user_id: string
  name: string
  editing: boolean
  since: string
}

const BEAT_MS = 15_000

// usePresence keeps this tab on a document while it's open; others are the
// other tabs there (anyone's, mine on another device too).
export function usePresence(kind: PresenceKind, id: Ref<string | undefined>, editing: Ref<boolean> = ref(false)) {
  const people = ref<Present[]>([])
  const others = ref<Present[]>([])
  let timer: ReturnType<typeof setInterval> | undefined
  let current = ''
  const set = (list: Present[]) => {
    people.value = list
    others.value = list.filter((p) => p.client !== CLIENT_ID)
  }
  async function beat(leave = false) {
    const doc = current
    if (!doc) return
    try {
      const list = await api<Present[]>(`/presence/${kind}/${doc}`, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID, editing: editing.value, leave }), keepalive: leave })
      if (!leave && doc === current) set(list)
    } catch {
      /* offline a moment: the next beat says it again */
    }
  }
  function leave() {
    if (current) void beat(true)
    clearInterval(timer)
    current = ''
    set([])
  }
  watch(
    id,
    (v) => {
      leave()
      if (!v) return
      current = v
      void beat()
      timer = setInterval(() => void beat(), BEAT_MS)
    },
    { immediate: true },
  )
  watch(editing, () => void beat())
  const off = onLiveEvent('presence', (e) => {
    if (e.doc === `${kind}:${current}`) set((e.people as Present[]) ?? [])
  })
  const onHide = () => leave()
  window.addEventListener('pagehide', onHide)
  onUnmounted(() => {
    off()
    window.removeEventListener('pagehide', onHide)
    leave()
  })
  return { people, others }
}

// A person's colour: the same for them everywhere (from their id).
const COLORS = ['#007aff', '#34c759', '#ff9500', '#af52de', '#ff2d55', '#5ac8fa', '#ffcc00', '#ff3b30', '#5856d6', '#30b0c7']
export function personColor(userId: string): string {
  let h = 0
  for (const ch of userId) h = (h * 31 + ch.charCodeAt(0)) >>> 0
  return COLORS[h % COLORS.length]
}

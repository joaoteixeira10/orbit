// The journal: the API, and its moods (1–5, as faces).
import { api } from '@/lib/api'

export interface JournalEntry {
  date: string
  content: string
  mood: number
}
export interface JournalDay {
  date: string
  mood: number
  excerpt: string
}
const j = (b: unknown) => JSON.stringify(b)
export const journalApi = {
  month: (ws: string, month: string) => api<JournalDay[]>(`/workspaces/${ws}/journal?month=${month}`),
  day: (ws: string, date: string) => api<{ entry: JournalEntry | null; on_this_day: JournalDay[] }>(`/workspaces/${ws}/journal/${date}`),
  put: (ws: string, date: string, b: { content?: string; mood?: number }) =>
    api<{ entry: JournalEntry | null }>(`/workspaces/${ws}/journal/${date}`, { method: 'PUT', body: j(b) }),
}

export const MOODS = ['😞', '😕', '😐', '🙂', '😄']
export const moodFace = (m: number) => (m >= 1 && m <= 5 ? MOODS[m - 1] : '')

// Dragging a task onto the calendar (from its side pane, or a task already
// on a day) gives it that date — and, dropped on an hour, that time.
import type { Task } from '@/api/planner'

const MIME = 'application/x-orbit-task'

export interface DraggedTask {
  id: string
  ws: string
}

export function startTaskDrag(ev: DragEvent, t: Pick<Task, 'id' | 'workspace_id'>) {
  if (!ev.dataTransfer) return
  ev.dataTransfer.effectAllowed = 'move'
  ev.dataTransfer.setData(MIME, JSON.stringify({ id: t.id, ws: t.workspace_id }))
  ev.dataTransfer.setData('text/plain', t.id)
}

// isTaskDrag: a task is being dragged (the data itself is only readable on drop).
export const isTaskDrag = (ev: DragEvent) => !!ev.dataTransfer?.types.includes(MIME)

export function readTaskDrag(ev: DragEvent): DraggedTask | null {
  try {
    const v = JSON.parse(ev.dataTransfer?.getData(MIME) || 'null')
    return v && typeof v.id === 'string' && typeof v.ws === 'string' ? v : null
  } catch {
    return null
  }
}

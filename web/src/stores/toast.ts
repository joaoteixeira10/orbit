import { defineStore } from 'pinia'

export type ToastKind = 'success' | 'error' | 'info'
export interface Toast {
  id: number
  kind: ToastKind
  message: string
  detail?: string
}

let nextId = 1

// As in v0: success toasts go away on their own; an error with a detail (e.g.
// what a proxy answered) stays until dismissed.
export const useToastStore = defineStore('toast', {
  state: () => ({ toasts: [] as Toast[] }),
  actions: {
    push(kind: ToastKind, message: string, detail?: string) {
      const id = nextId++
      this.toasts.push({ id, kind, message, detail })
      if (!(kind === 'error' && detail)) setTimeout(() => this.dismiss(id), kind === 'error' ? 8000 : 5000)
    },
    success(m: string) {
      this.push('success', m)
    },
    error(m: string, detail?: string) {
      this.push('error', m, detail)
    },
    info(m: string) {
      this.push('info', m)
    },
    dismiss(id: number) {
      this.toasts = this.toasts.filter((t) => t.id !== id)
    },
  },
})

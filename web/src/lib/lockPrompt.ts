// Asking for the lock PIN from anywhere: askUnlock() resolves true once the
// lock key is open (asking to set a PIN first when there's none), false if
// cancelled. One dialog (LockPinDialog, in the app shell) answers it.
import { reactive } from 'vue'
import { unlocked } from './lock'

export type PinMode = 'set' | 'unlock' | 'change'
export const pinPrompt = reactive<{ mode: PinMode | ''; resolve: ((ok: boolean) => void) | null }>({ mode: '', resolve: null })

export function askPin(mode: PinMode): Promise<boolean> {
  pinPrompt.resolve?.(false)
  return new Promise((resolve) => {
    pinPrompt.mode = mode
    pinPrompt.resolve = resolve
  })
}

// askUnlock: the key open, or why not (cancelled).
export function askUnlock(): Promise<boolean> {
  if (unlocked.value) return Promise.resolve(true)
  return askPin('unlock')
}

export function answerPin(ok: boolean) {
  const r = pinPrompt.resolve
  pinPrompt.mode = ''
  pinPrompt.resolve = null
  r?.(ok)
}

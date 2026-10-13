import { reactive } from 'vue'
import { setStepUpHandler } from '@/lib/api'

// Bridge between the api()-layer step-up interceptor and the StepUpModal.
// When a request hits `step_up_required`, requestStepUp() opens the modal and
// resolves once the user re-authenticates (true) or cancels (false).
export const stepUpState = reactive({ open: false })

let resolver: ((ok: boolean) => void) | null = null

function requestStepUp(): Promise<boolean> {
  // If a prompt is already open, chain onto the same resolution.
  if (stepUpState.open && resolver) {
    return new Promise<boolean>((res) => {
      const prev = resolver!
      resolver = (ok) => {
        prev(ok)
        res(ok)
      }
    })
  }
  stepUpState.open = true
  return new Promise<boolean>((res) => {
    resolver = res
  })
}

// resolveStepUp is called by the modal when re-auth succeeds or is cancelled.
export function resolveStepUp(ok: boolean) {
  stepUpState.open = false
  const r = resolver
  resolver = null
  r?.(ok)
}

// Wire the handler once at app start.
export function installStepUp() {
  setStepUpHandler(requestStepUp)
}

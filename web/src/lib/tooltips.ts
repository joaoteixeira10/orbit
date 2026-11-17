// Tooltips as shadcn-vue's (a small dark label over the control), for every
// element that has a `title` — the whole app, without wrapping each control.
// After a short pause on an element its title is lifted (so the browser's
// own tooltip doesn't show too) and put back the moment the pointer leaves,
// so it stays the element's title for everything else.
import { computePosition, flip, offset, shift } from '@floating-ui/dom'

const DELAY = 500

export function installTooltips(): void {
  if (typeof window === 'undefined' || !window.matchMedia('(hover: hover)').matches) return
  let el: HTMLElement | null = null // the element whose title is lifted
  let tip: HTMLDivElement | null = null
  let timer = 0

  function hide() {
    window.clearTimeout(timer)
    if (el) {
      const t = el.dataset.tipTitle
      if (t !== undefined && !el.hasAttribute('title')) el.setAttribute('title', t)
      delete el.dataset.tipTitle
      el = null
    }
    tip?.remove()
    tip = null
  }

  async function show(target: HTMLElement) {
    const text = target.getAttribute('title')?.trim()
    if (!text || !target.isConnected) return
    el = target
    target.dataset.tipTitle = target.getAttribute('title') ?? ''
    target.removeAttribute('title')
    tip = document.createElement('div')
    tip.className = 'orbit-tip'
    tip.setAttribute('role', 'tooltip')
    tip.textContent = text
    ;((document.fullscreenElement as HTMLElement | null) ?? document.body).appendChild(tip)
    const { x, y } = await computePosition(target, tip, {
      placement: 'top',
      strategy: 'fixed',
      middleware: [offset(6), flip({ padding: 8 }), shift({ padding: 8 })],
    })
    if (tip) Object.assign(tip.style, { left: `${x}px`, top: `${y}px` })
  }

  document.addEventListener('pointerover', (e) => {
    if (e.pointerType !== 'mouse') return
    const t = (e.target as Element | null)?.closest?.('[title]') as HTMLElement | null
    if (!t || t === el || t instanceof SVGElement || t.closest('iframe, .cm-editor, .ProseMirror')) return
    hide()
    timer = window.setTimeout(() => void show(t), DELAY)
  })
  document.addEventListener('pointerout', (e) => {
    const to = e.relatedTarget as Node | null
    const from = el ?? ((e.target as Element | null)?.closest?.('[title]') as HTMLElement | null)
    if (from && to && from.contains(to)) return
    hide()
  })
  for (const ev of ['pointerdown', 'keydown', 'scroll', 'blur'] as const) window.addEventListener(ev, hide, true)
}

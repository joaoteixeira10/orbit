import { describe, expect, it, vi } from 'vitest'
import { applyMotion, moving, spotlight, themeSweep, vCountUp } from './motion'

describe('motion', () => {
  it('turns the app still and moving again', () => {
    applyMotion(false)
    expect(document.documentElement.classList.contains('no-motion')).toBe(true)
    expect(moving()).toBe(false)
    applyMotion(true)
    expect(moving()).toBe(true)
  })

  it('switches the theme at once when still or without view transitions', () => {
    const apply = vi.fn()
    applyMotion(false)
    themeSweep(apply)
    expect(apply).toHaveBeenCalledTimes(1)
    applyMotion(true)
    themeSweep(apply) // no startViewTransition here
    expect(apply).toHaveBeenCalledTimes(2)
    expect(document.documentElement.classList.contains('theme-sweep')).toBe(false)
  })

  it('sweeps with a view transition when moving', async () => {
    const finished = Promise.resolve()
    const start = vi.fn((cb: () => void) => (cb(), { finished }))
    ;(document as any).startViewTransition = start
    const apply = vi.fn()
    applyMotion(true)
    themeSweep(apply)
    expect(start).toHaveBeenCalled()
    expect(apply).toHaveBeenCalled()
    expect(document.documentElement.classList.contains('theme-sweep')).toBe(true)
    await finished
    await Promise.resolve()
    expect(document.documentElement.classList.contains('theme-sweep')).toBe(false)
    delete (document as any).startViewTransition
  })

  it('shows a count at once when still, and blanks as they are', () => {
    applyMotion(false)
    const el = document.createElement('span')
    vCountUp.mounted(el, { value: 12 })
    expect(el.textContent).toBe('12')
    vCountUp.updated(el, { value: '', oldValue: 12 })
    expect(el.textContent).toBe('')
    applyMotion(true)
  })

  it('lights a date up even when still (it answers a click)', () => {
    applyMotion(false)
    const el = document.createElement('span')
    const animate = vi.fn()
    ;(el as any).animate = animate
    spotlight(el)
    spotlight(el, 'area')
    expect(animate).toHaveBeenCalledTimes(2)
    applyMotion(true)
  })
})

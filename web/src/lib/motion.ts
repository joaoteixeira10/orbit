// The app's motion: on unless the person turns Animations off (Settings →
// Appearance) or the system asks for reduced motion. html.no-motion stills
// everything (styles/motion.css); what animates on purpose asks moving().

// The system's setting (absent where there's no screen, as in the tests).
const still = typeof window.matchMedia === 'function' ? window.matchMedia('(prefers-reduced-motion: reduce)') : null
let wanted = true

function paint() {
  document.documentElement.classList.toggle('no-motion', !wanted || !!still?.matches)
}
still?.addEventListener('change', paint)

/** Animations on or off, as the person chose (the system can still hold them). */
export function applyMotion(on: boolean) {
  wanted = on
  paint()
}

/** Is the app animating now? */
export function moving(): boolean {
  return !document.documentElement.classList.contains('no-motion')
}

type VTDocument = Document & { startViewTransition?: (cb: () => Promise<void> | void) => { finished: Promise<void> } }

/** Light ↔ dark: the new look rises from the bottom of the window with a soft
 *  edge (styles/motion.css). Still, or in a browser without view transitions,
 *  it simply changes. */
export function themeSweep(apply: () => Promise<void> | void) {
  const d = document as VTDocument
  if (!d.startViewTransition || !moving()) return void apply()
  const root = document.documentElement
  root.classList.add('theme-sweep')
  void d.startViewTransition(apply).finished.finally(() => root.classList.remove('theme-sweep'))
}

/** A page that has just opened: it fades in and rises into place. Without
 *  `rise`, it only fades: a page with a canvas or an editor (a drawing, a
 *  sheet) measures where it is as it opens, and must not be moving then. */
export function pageIn(el: Element | null | undefined, rise = true) {
  if (!el || !moving() || !('animate' in el)) return
  el.animate(
    rise
      ? [
          { opacity: 0, translate: '0 18px' },
          { opacity: 1, translate: '0 0' },
        ]
      : [{ opacity: 0 }, { opacity: 1 }],
    { duration: rise ? 420 : 260, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' },
  )
}

// Ticking a check (one not yet done) anywhere in the app, or a habit's day:
// it pops and a ring of its colour spreads from it. Animated here, not by a class: Vue rewrites
// the check's classes as it turns done.
document.addEventListener(
  'click',
  (e) => {
    // A task's check, or a habit's day (Habits).
    const el = (e.target as Element | null)?.closest?.('.check, .habit .tick') as HTMLElement | null
    if (!el || el.classList.contains('on') || el.classList.contains('done') || (el as HTMLButtonElement).disabled || !moving() || !el.animate) return
    const css = getComputedStyle(el)
    const c = css.getPropertyValue('--check').trim() || css.getPropertyValue('--c').trim() || 'var(--accent)'
    el.animate(
      [
        { scale: 1, boxShadow: `0 0 0 0 color-mix(in srgb, ${c} 55%, transparent)` },
        { scale: 0.78, offset: 0.3 },
        { scale: 1.18, offset: 0.65 },
        { scale: 1, boxShadow: '0 0 0 10px transparent' },
      ],
      { duration: 480, easing: 'cubic-bezier(0.3, 0.7, 0.4, 1)' },
    )
  },
  true,
)

/** Point at something on purpose (Calendar's Today): a ring of the accent
 *  that pulses twice around a date, or a wash over a day's area. It answers a
 *  click, so it shows whatever the Animations switch says; with the system's
 *  reduced motion it holds still for a moment instead of pulsing. */
export function spotlight(el: HTMLElement | null | undefined, kind: 'dot' | 'area' = 'dot') {
  if (!el?.animate) return
  const c = getComputedStyle(el).getPropertyValue('--accent').trim() || '#0a84ff'
  const tint = (pct: number) => `color-mix(in srgb, ${c} ${pct}%, transparent)`
  const steady = !!still?.matches
  if (kind === 'area') {
    const on = `inset 0 0 0 2px ${tint(70)}, inset 0 0 0 400px ${tint(12)}`
    const off = `inset 0 0 0 2px ${tint(0)}, inset 0 0 0 400px ${tint(0)}`
    el.animate(steady ? [{ boxShadow: on }, { boxShadow: on, offset: 0.8 }, { boxShadow: off }] : [{ boxShadow: off }, { boxShadow: on, offset: 0.15 }, { boxShadow: on, offset: 0.6 }, { boxShadow: off }], { duration: 1600, easing: 'ease-out' })
    return
  }
  const ring = (pct: number, r: number) => `0 0 0 ${r}px ${tint(pct)}`
  el.animate(
    steady
      ? [{ boxShadow: ring(60, 3) }, { boxShadow: ring(60, 3), offset: 0.8 }, { boxShadow: ring(0, 3) }]
      : [
          { boxShadow: ring(0, 0), scale: 1 },
          { boxShadow: ring(60, 5), scale: 1.15, offset: 0.15 },
          { boxShadow: ring(0, 14), scale: 1, offset: 0.45 },
          { boxShadow: ring(60, 5), scale: 1.1, offset: 0.6 },
          { boxShadow: ring(0, 14), scale: 1 },
        ],
    { duration: 1400, easing: 'ease-out' },
  )
}

// ---- v-count-up: a count that ticks up to its number (and on to the next
// one when it changes). The element is the directive's: leave it empty in
// the template. A blank or a word shows as it is. ----
const counting = new WeakMap<HTMLElement, number>()
function countTo(el: HTMLElement, from: number, to: unknown) {
  cancelAnimationFrame(counting.get(el) ?? 0)
  if (typeof to !== 'number' || !Number.isFinite(to) || !moving() || from === to || typeof requestAnimationFrame !== 'function') {
    el.textContent = to == null ? '' : String(to)
    return
  }
  const t0 = performance.now()
  const dur = Math.min(700, 300 + Math.abs(to - from) * 25)
  const step = (now: number) => {
    const k = Math.min(1, (now - t0) / dur)
    el.textContent = String(Math.round(from + (to - from) * (1 - (1 - k) ** 3)))
    if (k < 1) counting.set(el, requestAnimationFrame(step))
  }
  counting.set(el, requestAnimationFrame(step))
}
export const vCountUp = {
  mounted(el: HTMLElement, b: { value: unknown }) {
    countTo(el, 0, b.value)
  },
  updated(el: HTMLElement, b: { value: unknown; oldValue: unknown }) {
    if (b.value !== b.oldValue) countTo(el, typeof b.oldValue === 'number' ? b.oldValue : 0, b.value)
  },
  beforeUnmount(el: HTMLElement) {
    cancelAnimationFrame(counting.get(el) ?? 0)
  },
}

// ---- Glide: the highlight of what's chosen slides from the old choice to
// the new one (a view switch, tabs, the sidebar). A stand-in of the new
// choice's look travels from where the old one was; the new one shows its
// own look again when it lands. ----
const unstyle = (el: HTMLElement, on: boolean) => {
  for (const p of ['background', 'box-shadow', 'border-color', 'transition']) {
    if (on) el.style.setProperty(p, p === 'transition' ? 'none' : p === 'background' ? 'transparent' : p === 'box-shadow' ? 'none' : 'transparent', 'important')
    else el.style.removeProperty(p)
  }
}
export function glide(host: HTMLElement, from: DOMRect, to: HTMLElement) {
  if (!moving() || !to.animate) return
  const tr = to.getBoundingClientRect()
  if (!tr.width || !from.width || (from.left === tr.left && from.top === tr.top)) return
  const hr = host.getBoundingClientRect()
  // Its look as it ends up (a fade it may have just started is cut short).
  to.style.setProperty('transition', 'none', 'important')
  const cs = getComputedStyle(to)
  const g = document.createElement('span')
  g.setAttribute('aria-hidden', 'true')
  const x = tr.left - hr.left - host.clientLeft + host.scrollLeft
  const y = tr.top - hr.top - host.clientTop + host.scrollTop
  Object.assign(g.style, {
    position: 'absolute', left: `${x}px`, top: `${y}px`, width: `${tr.width}px`, height: `${tr.height}px`,
    background: cs.background, borderRadius: cs.borderRadius, boxShadow: cs.boxShadow, border: `${cs.borderTopWidth} solid ${cs.borderTopColor}`,
    boxSizing: 'border-box', zIndex: '-1', pointerEvents: 'none', transformOrigin: '0 0',
  })
  if (getComputedStyle(host).position === 'static') host.style.position = 'relative'
  host.style.isolation = 'isolate'
  host.appendChild(g)
  unstyle(to, true)
  g.animate(
    [
      { transform: `translate(${from.left - tr.left}px, ${from.top - tr.top}px)`, width: `${from.width}px`, height: `${from.height}px` },
      { transform: 'none', width: `${tr.width}px`, height: `${tr.height}px` },
    ],
    { duration: 300, easing: 'cubic-bezier(0.3, 0.8, 0.3, 1)' },
  ).finished.finally(() => {
    g.remove()
    unstyle(to, false)
  })
}
// Every view switch (.seg) and every tab list: on a click, the highlight
// glides to the new choice (once the app has drawn it).
const GLIDERS: [string, string][] = [
  ['.seg', ':scope > button.on'],
  ['[data-slot="tabs-list"]', ':scope > [data-state="active"]'],
]
document.addEventListener(
  'click',
  (e) => {
    if (!moving()) return
    const t = e.target as Element | null
    for (const [box, on] of GLIDERS) {
      const host = t?.closest?.(box) as HTMLElement | null
      if (!host) continue
      const from = host.querySelector(on) as HTMLElement | null
      if (!from || from.contains(t)) return
      const rect = from.getBoundingClientRect()
      unstyle(from, true)
      setTimeout(() => {
        unstyle(from, false)
        const to = host.querySelector(on) as HTMLElement | null
        if (to && to !== from) glide(host, rect, to)
      })
      return
    }
  },
  true,
)

/** A page turning (Calendar's next and previous): the new one slides in from
 *  the side it comes from. */
export function slideIn(el: Element | null | undefined, dir: 1 | -1) {
  if (!el || !moving() || !('animate' in el)) return
  el.animate(
    [
      { opacity: 0.2, translate: `${dir * 36}px 0` },
      { opacity: 1, translate: '0 0' },
    ],
    { duration: 320, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' },
  )
}

/** Something new to see (the bell when a notification comes): a short shake. */
export function shake(el: Element | null | undefined) {
  if (!el || !moving() || !('animate' in el)) return
  el.animate([{ rotate: '0deg' }, { rotate: '14deg' }, { rotate: '-12deg' }, { rotate: '8deg' }, { rotate: '-5deg' }, { rotate: '0deg' }], { duration: 650, easing: 'ease-in-out' })
}

/** A small thing that changed (a badge, a check-in): it pops. */
export function pop(el: Element | null | undefined, to = 1.25) {
  if (!el || !moving() || !('animate' in el)) return
  el.animate([{ scale: 1 }, { scale: to, offset: 0.4 }, { scale: 1 }], { duration: 380, easing: 'cubic-bezier(0.3, 0.7, 0.4, 1.4)' })
}

/** Something dropped in place (a board card): it settles with a little bounce. */
export function settle(el: Element | null | undefined) {
  if (!el || !moving() || !('animate' in el)) return
  el.animate([{ scale: 1.04 }, { scale: 0.98, offset: 0.5 }, { scale: 1 }], { duration: 360, easing: 'cubic-bezier(0.3, 0.7, 0.4, 1)' })
}

/** A celebration (the last task of the day done): confetti in the theme's
 *  colours bursts from a point on the screen. */
export function confetti(x: number, y: number) {
  if (!moving() || typeof document.body.animate !== 'function') return
  const css = getComputedStyle(document.documentElement)
  const theme = ['--accent', '--h-a1', '--h-a2', '--h-a3'].map((v) => css.getPropertyValue(v).trim()).filter(Boolean)
  const colors = [...theme, '#ff9f0a', '#34c759', '#ff375f', '#5e5ce6', '#ffd60a']
  const box = document.createElement('div')
  Object.assign(box.style, { position: 'fixed', left: `${x}px`, top: `${y}px`, width: '0', height: '0', zIndex: '800', pointerEvents: 'none' })
  box.setAttribute('aria-hidden', 'true')
  document.body.appendChild(box)
  let left = 0
  for (let i = 0; i < 70; i++) {
    const p = document.createElement('i')
    const w = 5 + Math.random() * 6
    Object.assign(p.style, { position: 'absolute', display: 'block', width: `${w}px`, height: `${w * (Math.random() < 0.5 ? 0.45 : 1)}px`, background: colors[i % colors.length], borderRadius: Math.random() < 0.3 ? '50%' : '1px' })
    box.appendChild(p)
    const a = -Math.PI / 2 + (Math.random() - 0.5) * Math.PI * 1.2
    const v = 160 + Math.random() * 260
    const dx = Math.cos(a) * v
    const dy = Math.sin(a) * v
    const spin = (Math.random() - 0.5) * 1000
    left++
    p.animate(
      [
        { transform: 'translate(0, 0) rotate(0deg)', opacity: 1 },
        { transform: `translate(${dx * 0.8}px, ${dy * 0.8}px) rotate(${spin * 0.4}deg)`, opacity: 1, offset: 0.35 },
        { transform: `translate(${dx}px, ${dy + 320}px) rotate(${spin}deg)`, opacity: 0 },
      ],
      { duration: 1600 + Math.random() * 800, easing: 'cubic-bezier(0.2, 0.7, 0.4, 1)', delay: Math.random() * 150 },
    ).onfinish = () => {
      if (--left === 0) box.remove()
    }
  }
}

// Where the last click was (a celebration bursts from there).
let lastX = -1
let lastY = -1
document.addEventListener(
  'pointerdown',
  (e) => {
    lastX = e.clientX
    lastY = e.clientY
  },
  true,
)
/** The last task of the day done: confetti from where it was ticked. */
export function celebrate() {
  confetti(lastX >= 0 ? lastX : innerWidth / 2, lastY >= 0 ? lastY : innerHeight / 3)
}

/** For <Transition :css="false" @enter="openUp" @leave="foldAway">: a list
 *  that opens down and folds back up (the sidebar's folders). */
export function openUp(el: Element, done: () => void) {
  const h = (el as HTMLElement).scrollHeight
  if (!moving() || !h) return done()
  ;(el as HTMLElement).style.overflow = 'hidden'
  el.animate([{ height: '0px', opacity: 0 }, { height: `${h}px`, opacity: 1 }], { duration: 240, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' }).finished.finally(() => {
    ;(el as HTMLElement).style.overflow = ''
    done()
  })
}
export function foldAway(el: Element, done: () => void) {
  const h = (el as HTMLElement).offsetHeight
  if (!moving() || !h) return done()
  ;(el as HTMLElement).style.overflow = 'hidden'
  el.animate([{ height: `${h}px`, opacity: 1 }, { height: '0px', opacity: 0 }], { duration: 200, easing: 'cubic-bezier(0.4, 0, 0.6, 1)' }).finished.finally(done)
}

// ---- v-flip: a list whose items move (sorted, dragged, a task un-ticked)
// glides them to their new places instead of jumping. Items are the
// container's children with a data-id; none while a drag is on (the drag
// moves them itself). ----
const places = new WeakMap<HTMLElement, Map<string, DOMRect>>()
let dropAt = -1e9
/** A drag just ended (lib/sortable.ts): what moved was put there by hand, so
 *  the lists don't glide it; it settles instead. */
export function dropped() {
  dropAt = performance.now()
}
const dragging = () => performance.now() - dropAt < 600 || !!document.querySelector('.sortable-chosen, .sortable-drag, .sortable-ghost')
function measure(el: HTMLElement) {
  const m = new Map<string, DOMRect>()
  for (const c of el.querySelectorAll<HTMLElement>(':scope > [data-id]')) m.set(c.dataset.id!, c.getBoundingClientRect())
  return m
}
export const vFlip = {
  beforeUpdate(el: HTMLElement) {
    if (moving() && !dragging()) places.set(el, measure(el))
    else places.delete(el)
  },
  updated(el: HTMLElement) {
    const before = places.get(el)
    places.delete(el)
    if (!before || dragging()) return
    for (const c of el.querySelectorAll<HTMLElement>(':scope > [data-id]')) {
      const was = before.get(c.dataset.id!)
      if (!was || !c.animate) continue
      const now = c.getBoundingClientRect()
      const dx = was.left - now.left
      const dy = was.top - now.top
      if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue
      c.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: 'none' }], { duration: 320, easing: 'cubic-bezier(0.2, 0.8, 0.2, 1)' })
    }
  },
}

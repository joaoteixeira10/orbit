<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useHolidayStore } from '@/stores/holiday'

// The landscape behind the liquid glass while a season or holiday theme is
// on: a sky, the sun's glow, five ridges fading into the distance with mist
// between them, the nearest with a tree line. Shapes only, no picture: the
// ridges are waves (each theme its own), the colours the theme's --h-sc*
// tokens (styles/glass.css). Each season adds its own (below). Behind
// everything, never a click; html.scene tells the glass and the cards to
// let it through.
const holiday = useHolidayStore()

const W = 1600
const H = 900
// A ridge's skyline: three waves for the land, and with `trees`, rounded
// crowns along the top.
function ridge(base: number, amp: number, seed: number, trees = 0, step = 8): string {
  let d = `M0 ${H} `
  for (let x = 0; x <= W; x += step) {
    const u = (x / W) * Math.PI * 2
    let y = base - amp * (0.55 * Math.sin(u * 1.3 + seed) + 0.3 * Math.sin(u * 2.7 + seed * 2.3) + 0.15 * Math.sin(u * 5.1 + seed * 4.1))
    if (trees) y -= trees * Math.sqrt(Math.abs(Math.sin(x / 15 + seed * 7))) * (0.6 + 0.4 * Math.sin(x / 61 + seed))
    d += `L${x} ${y.toFixed(1)} `
  }
  return d + `L${W} ${H}Z`
}
const seedOf = (id: string) => [...id].reduce((a, c) => (a * 31 + c.charCodeAt(0)) % 997, 7) / 97

const paths = computed(() => {
  const s = seedOf(holiday.look?.id ?? 'orbit')
  return [
    ridge(400, 90, s),
    ridge(480, 74, s + 1.7, 2, 4),
    ridge(575, 60, s + 3.1, 5, 3),
    ridge(680, 50, s + 4.6, 8, 3),
    ridge(800, 40, s + 6.2, 12, 3),
  ]
})

// Each season's own (the colours its --h-sc* tokens, seasonThemes.ts), the
// same every time (fixed seeds):
// - spring: cherry trees in bloom; autumn: maples in their reds and golds.
//   A trunk that forks and forks again, each twig ending in a cloud of
//   blossom or leaves (--h-scCrown1…3).
// - winter: pines in tiers, snow on each tier.
// - summer: a lake in front of the hills, the sun on it, a sailboat.
type Twig = { d: string; w: number }
type Dab = { x: number; y: number; r: number; c: 1 | 2 | 3; o: number }
// fill: from this depth down, every fork carries leaves too (a full crown);
// trunk: the trunk's length (of the size); fork: how wide the forks open.
type Crown = { mass: number; dabs: number; dab: [number, number]; spread: number; depth: number; fill: number; trunk: number; fork: number }
const BLOSSOM: Crown = { mass: 0.045, dabs: 14, dab: [0.008, 0.016], spread: 0.07, depth: 6, fill: 0, trunk: 0.27, fork: 1 }
const MAPLE: Crown = { mass: 0.07, dabs: 20, dab: [0.011, 0.016], spread: 0.1, depth: 5, fill: 2, trunk: 0.2, fork: 0.75 }
function tree(x0: number, y0: number, size: number, seed: number, c: Crown, far = false) {
  let t = seed
  const rnd = () => ((t = (t * 16807) % 2147483647) - 1) / 2147483646
  const twigs: Twig[] = []
  const dabs: Dab[] = []
  const grow = (x: number, y: number, ang: number, len: number, w: number, depth: number) => {
    const x1 = x + Math.sin(ang) * len
    const y1 = y - Math.cos(ang) * len
    const bend = (rnd() - 0.5) * len * 0.35
    const mx = (x + x1) / 2 + Math.cos(ang) * bend
    const my = (y + y1) / 2 + Math.sin(ang) * bend
    twigs.push({ d: `M${x.toFixed(1)} ${y.toFixed(1)}Q${mx.toFixed(1)} ${my.toFixed(1)} ${x1.toFixed(1)} ${y1.toFixed(1)}`, w })
    if (depth <= c.fill) {
      // A faint mass, then small flowers or leaves over it, with sky
      // between the clouds.
      dabs.push({ x: x1, y: y1, r: size * (c.mass + rnd() * 0.02), c: 1, o: 0.45 })
      for (let i = 0; i < c.dabs; i++) {
        const a = rnd() * Math.PI * 2
        const d = Math.sqrt(rnd()) * size * c.spread
        dabs.push({ x: x1 + Math.cos(a) * d, y: y1 + Math.sin(a) * d * 0.75, r: size * (c.dab[0] + rnd() * c.dab[1]), c: (1 + Math.floor(rnd() * 3)) as 1 | 2 | 3, o: 0.7 + rnd() * 0.3 })
      }
    }
    if (depth === 0) return
    const n = depth > 3 ? 2 : 2 + (rnd() < 0.4 ? 1 : 0)
    for (let i = 0; i < n; i++) {
      const spread = (n === 2 ? (i ? 1 : -1) : i - 1) * (0.32 + rnd() * 0.3) * c.fork
      grow(x1, y1, ang * 0.6 + spread, len * (0.68 + rnd() * 0.16), w * 0.68, depth - 1)
    }
  }
  grow(x0, y0, (rnd() - 0.5) * 0.15, size * c.trunk, size * 0.05, c.depth)
  return { twigs, dabs, far, origin: `${x0}px ${y0}px` }
}
// A pine: a trunk and five tiers, each with snow on its top part.
function pine(x: number, base: number, h: number, w: number) {
  const tiers = 5
  let body = `M${x - w * 0.04} ${base}h${w * 0.08}v${-h * 0.14}h${-w * 0.08}z`
  let snow = ''
  for (let i = 0; i < tiers; i++) {
    const top = base - h + i * h * 0.16
    const bot = top + h * 0.3
    const hw = (w / 2) * (0.35 + (0.65 * (i + 1)) / tiers)
    body += `M${x} ${top}L${x + hw} ${bot}Q${x + hw * 0.5} ${bot - h * 0.03} ${x} ${bot + h * 0.01}Q${x - hw * 0.5} ${bot - h * 0.03} ${x - hw} ${bot}z`
    const sh = (bot - top) * 0.42
    const sw = hw * 0.42
    snow += `M${x} ${top}L${x + sw} ${top + sh}Q${x + sw * 0.5} ${top + sh * 0.8} ${x + sw * 0.15} ${top + sh * 1.05}Q${x - sw * 0.3} ${top + sh * 0.75} ${x - sw} ${top + sh}z`
  }
  return { body, snow }
}
const look = computed(() => holiday.look?.id)
const trees = computed(() => {
  if (look.value === 'spring') return [tree(1340, 905, 560, 4242, BLOSSOM), tree(250, 700, 240, 777, BLOSSOM, true)]
  if (look.value === 'autumn') return [tree(1350, 905, 520, 9001, MAPLE), tree(1530, 905, 300, 31337, MAPLE), tree(300, 690, 220, 2024, MAPLE, true)]
  return []
})
const pines = computed(() => {
  if (look.value !== 'winter') return []
  const far = [130, 205, 260, 340, 420, 1500, 1560].map((x, i) => ({ ...pine(x, 640 + (i % 3) * 8, 90 + (i % 4) * 18, 50 + (i % 3) * 10), far: true }))
  const near = [
    { ...pine(1300, 905, 440, 210), far: false },
    { ...pine(1440, 905, 340, 160), far: false },
    { ...pine(1180, 905, 250, 120), far: false },
    { ...pine(1560, 905, 260, 125), far: false },
    { ...pine(140, 905, 380, 180), far: false },
  ]
  return [...far, ...near]
})
const lake = computed(() => look.value === 'summer')
// The lake's ripples: short light strokes, more of them near the shore.
const ripples = Array.from({ length: 26 }, (_, i) => {
  const y = 772 + ((i * 37) % 120)
  const x = (i * 263) % W
  return { x, y, w: 30 + ((i * 17) % 50) }
})

onMounted(() => document.documentElement.classList.add('scene'))
onUnmounted(() => document.documentElement.classList.remove('scene'))
</script>

<template>
  <div class="glass-scene" aria-hidden="true" data-testid="glass-scene">
    <svg :viewBox="`0 0 ${W} ${H}`" preserveAspectRatio="xMidYMax slice">
      <defs>
        <linearGradient id="gs-sky" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style="stop-color: var(--sc-sky)" />
          <stop offset="0.62" style="stop-color: var(--sc-low)" />
        </linearGradient>
        <radialGradient id="gs-sun" cx="0.5" cy="0.5" r="0.5">
          <stop offset="0" style="stop-color: var(--sc-sun); stop-opacity: 1" />
          <stop offset="1" style="stop-color: var(--sc-sun); stop-opacity: 0" />
        </radialGradient>
        <linearGradient id="gs-mist" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style="stop-color: var(--sc-mist); stop-opacity: 0" />
          <stop offset="0.55" style="stop-color: var(--sc-mist); stop-opacity: 0.85" />
          <stop offset="1" style="stop-color: var(--sc-mist); stop-opacity: 0" />
        </linearGradient>
        <linearGradient id="gs-haze" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" style="stop-color: var(--sc-mist); stop-opacity: 0" />
          <stop offset="1" style="stop-color: var(--sc-mist); stop-opacity: 0.45" />
        </linearGradient>
      </defs>
      <rect :width="W" :height="H" fill="url(#gs-sky)" />
      <circle cx="1130" cy="330" r="340" fill="url(#gs-sun)" />
      <path :d="paths[0]" style="fill: var(--sc-r1)" />
      <rect class="gs-drift" x="-200" y="360" :width="W + 400" height="170" fill="url(#gs-mist)" />
      <path :d="paths[1]" style="fill: var(--sc-r2)" />
      <rect class="gs-drift gs-slow" x="-200" y="450" :width="W + 400" height="150" fill="url(#gs-mist)" />
      <path :d="paths[2]" style="fill: var(--sc-r3)" />
      <rect class="gs-drift" x="-200" y="550" :width="W + 400" height="140" fill="url(#gs-mist)" opacity="0.85" />
      <path :d="paths[3]" style="fill: var(--sc-r4)" />
      <rect class="gs-drift gs-slow" x="-200" y="660" :width="W + 400" height="120" fill="url(#gs-mist)" opacity="0.6" />
      <!-- The far ones, behind the nearest hills and hazed by the air. -->
      <g v-for="(tr, i) in trees.filter((x) => x.far)" :key="'ft' + i" opacity="0.75">
        <path v-for="(b, j) in tr.twigs" :key="j" :d="b.d" :stroke-width="b.w" style="stroke: var(--h-scBark)" fill="none" stroke-linecap="round" />
        <circle v-for="(b, j) in tr.dabs" :key="'d' + j" :cx="b.x" :cy="b.y" :r="b.r" :opacity="b.o" :style="{ fill: `var(--h-scCrown${b.c})` }" />
      </g>
      <g v-for="(p, i) in pines.filter((x) => x.far)" :key="'fp' + i" opacity="0.6">
        <path :d="p.body" style="fill: var(--h-scPine)" />
        <path :d="p.snow" style="fill: var(--h-scSnow)" />
      </g>
      <path :d="paths[4]" style="fill: var(--sc-r5)" />
      <!-- Summer: the lake, the sun on the water, ripples, a sailboat. -->
      <template v-if="lake">
        <defs>
          <linearGradient id="gs-water" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" style="stop-color: var(--h-scWater1)" />
            <stop offset="1" style="stop-color: var(--h-scWater2)" />
          </linearGradient>
          <radialGradient id="gs-glint" cx="0.5" cy="0.5" r="0.5">
            <stop offset="0" style="stop-color: var(--sc-sun); stop-opacity: 0.9" />
            <stop offset="1" style="stop-color: var(--sc-sun); stop-opacity: 0" />
          </radialGradient>
        </defs>
        <path :d="`M0 772Q400 762 800 770T${W} 768V${H}H0Z`" fill="url(#gs-water)" />
        <ellipse cx="1130" cy="830" rx="60" ry="70" fill="url(#gs-glint)" />
        <g class="gs-ripple" style="stroke: var(--sc-sun)" stroke-width="2" stroke-linecap="round" opacity="0.55">
          <path v-for="(r, i) in ripples" :key="i" :d="`M${r.x} ${r.y}h${r.w}`" />
        </g>
        <g class="gs-boat">
          <path d="M1288 818h104l-14 18h-78z" style="fill: var(--h-scHull)" />
          <path d="M1338 736v80" style="stroke: var(--h-scHull)" stroke-width="3" />
          <path d="M1341 740q34 34 40 72h-40z" style="fill: var(--h-scSail)" />
          <path d="M1335 750q-24 30-38 62h38z" style="fill: var(--h-scSail)" opacity="0.85" />
          <path d="M1338 736l14 6-14 6z" style="fill: var(--h-scHull)" />
        </g>
        <path d="M1300 842h82" style="stroke: var(--h-scWater1)" stroke-width="2" opacity="0.7" />
      </template>
      <g v-for="(tr, i) in trees.filter((x) => !x.far)" :key="'nt' + i" class="gs-sway" :style="{ transformOrigin: tr.origin, animationDelay: `${-i * 2.3}s` }">
        <path v-for="(b, j) in tr.twigs" :key="j" :d="b.d" :stroke-width="b.w" style="stroke: var(--h-scBark)" fill="none" stroke-linecap="round" />
        <circle v-for="(b, j) in tr.dabs" :key="'d' + j" :cx="b.x" :cy="b.y" :r="b.r" :opacity="b.o" :style="{ fill: `var(--h-scCrown${b.c})` }" />
      </g>
      <g v-for="(p, i) in pines.filter((x) => !x.far)" :key="'np' + i">
        <path :d="p.body" style="fill: var(--h-scPine)" />
        <path :d="p.snow" style="fill: var(--h-scSnow)" />
      </g>
      <rect y="760" :width="W" height="140" fill="url(#gs-haze)" />
    </svg>
  </div>
</template>

<style scoped>
.glass-scene {
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
}
.glass-scene svg {
  width: 100%;
  height: 100%;
  display: block;
}
/* The mist drifts, slowly (none when motion is reduced). */
.gs-drift {
  animation: gs-drift 70s ease-in-out infinite alternate;
}
.gs-slow {
  animation-duration: 95s;
  animation-direction: alternate-reverse;
}
@keyframes gs-drift {
  to {
    transform: translateX(160px);
  }
}
/* The near trees sway a little in the wind; the boat rocks on the water,
   the ripples drift. */
.gs-sway {
  transform-box: view-box;
  animation: gs-sway 9s ease-in-out infinite alternate;
}
.gs-boat {
  transform-box: view-box;
  transform-origin: 1340px 830px;
  animation: gs-rock 5s ease-in-out infinite alternate;
}
@keyframes gs-rock {
  from {
    rotate: -2deg;
    translate: 0 1px;
  }
  to {
    rotate: 2deg;
    translate: 0 -2px;
  }
}
.gs-ripple {
  animation: gs-ripple 14s ease-in-out infinite alternate;
}
@keyframes gs-ripple {
  to {
    transform: translateX(40px);
    opacity: 0.25;
  }
}
@keyframes gs-sway {
  from {
    rotate: -0.6deg;
  }
  to {
    rotate: 0.6deg;
  }
}
@media (prefers-reduced-motion: reduce) {
  .gs-drift,
  .gs-sway,
  .gs-boat,
  .gs-ripple {
    animation: none;
  }
}
</style>

<script setup lang="ts">
import { computed } from 'vue'
import { useHolidayStore } from '@/stores/holiday'
import { buntingLayout, lightsLayout, rnd, type Decor } from '@/lib/holidayThemes'

// What hangs along the top of Home's column: Christmas lights, things on
// threads (spiders, stars, eggs, lanterns — in the right part only, clear
// of the greeting and the capture box), Santos' bunting, Halloween's moon.
// Never a click; a narrow column keeps only the lights or bunting.
const holiday = useHolidayStore()
const decors = computed(() => {
  const h = holiday.active
  return h ? ([h.decor, h.extraDecor].filter(Boolean) as Decor[]) : []
})

const lights = computed(() => {
  const d = decors.value.find((x) => x.kind === 'lights')
  return d?.kind === 'lights' ? lightsLayout(d.count) : null
})
const bunting = computed(() => {
  const d = decors.value.find((x) => x.kind === 'bunting')
  return d?.kind === 'bunting' ? buntingLayout(d.scallops, d.depth, d.perScallop) : null
})

function drops(d: Extract<Decor, { kind: 'drop' }>) {
  return d.items.map(([x, len, scale], i) => ({
    x,
    len,
    w: d.viewBox[0] * scale * 1.2,
    h: d.viewBox[1] * scale * 1.2,
    svg: d.svg(i),
    dur: 2.4 + rnd(i, 9) * 1.6,
    delay: -rnd(i, 10) * 3,
  }))
}
</script>

<template>
  <div v-if="holiday.active" class="holiday-hang" aria-hidden="true" data-testid="holiday-hang">
    <template v-for="(d, n) in decors" :key="n">
      <template v-if="d.kind === 'lights' && lights">
        <svg class="h-wire" :viewBox="lights.viewBox" preserveAspectRatio="none">
          <path :d="lights.wire" fill="none" stroke="var(--h-wire)" stroke-width="1.5" vector-effect="non-scaling-stroke" />
        </svg>
        <span
          v-for="(b, i) in lights.bulbs"
          :key="i"
          class="h-bulb"
          :style="{ left: `${b.leftPct}%`, transform: `translateX(-50%) rotate(${b.rotate}deg)` }"
        >
          <span class="h-bulb-cap" />
          <span
            class="h-bulb-glass"
            :style="{ background: b.color, boxShadow: `0 0 10px 2px ${b.color}88`, animation: `h-twinkle ${b.twinkleDur}s ${b.twinkleDelay}s ease-in-out infinite alternate` }"
          />
        </span>
      </template>
      <template v-else-if="d.kind === 'bunting' && bunting">
        <svg class="h-wire" :viewBox="bunting.viewBox" preserveAspectRatio="none">
          <path :d="bunting.wire" fill="none" stroke="var(--h-wire)" stroke-width="1.5" vector-effect="non-scaling-stroke" />
        </svg>
        <span
          v-for="(f, i) in bunting.flags"
          :key="i"
          class="h-flag"
          :style="{ left: `calc(${f.leftPct}% - ${bunting.flagW / 2}px)`, top: `${f.topPx}px`, animation: `h-sway ${f.swayDur}s ${f.swayDelay}s ease-in-out infinite alternate` }"
          v-html="bunting.flagSvg(f.color)"
        />
      </template>
      <template v-else-if="d.kind === 'drop'">
        <span
          v-for="(it, i) in drops(d)"
          :key="i"
          class="h-drop"
          :style="{ left: `${it.x}%`, animation: `h-bob ${it.dur}s ${it.delay}s ease-in-out infinite alternate` }"
        >
          <span class="h-thread" :style="{ height: `${it.len + 12}px` }" />
          <span class="h-item" :style="{ width: `${it.w}px`, height: `${it.h}px` }" v-html="it.svg" />
        </span>
      </template>
    </template>
    <span v-if="holiday.active.art.moon" class="h-moon">
      <span style="left: 38px; top: 16px; width: 12px; height: 12px; background: rgba(160, 140, 90, 0.25)" />
      <span style="left: 18px; top: 38px; width: 8px; height: 8px; background: rgba(160, 140, 90, 0.22)" />
      <span style="left: 44px; top: 44px; width: 6px; height: 6px; background: rgba(160, 140, 90, 0.2)" />
    </span>
  </div>
</template>

<style scoped>
.holiday-hang {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 0;
  z-index: 2;
  pointer-events: none;
}
.h-wire {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 40px;
}
.h-bulb {
  position: absolute;
  top: 12px;
  display: flex;
  width: 8px;
  flex-direction: column;
  align-items: center;
}
.h-bulb-cap {
  width: 5px;
  height: 4px;
  border-radius: 1px;
  background: var(--h-bulbCap);
}
.h-bulb-glass {
  width: 8px;
  height: 12px;
  border-radius: 50% 50% 50% 50% / 40% 40% 60% 60%;
}
.h-flag {
  position: absolute;
  display: block;
  transform-origin: top center;
}
.h-drop {
  position: absolute;
  top: -12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  translate: -50% 0;
}
.h-thread {
  width: 1px;
  background: var(--h-thread);
}
.h-item {
  display: block;
}
.h-item :deep(svg) {
  display: block;
  width: 100%;
  height: 100%;
}
.h-moon {
  position: absolute;
  top: 34px;
  right: 120px;
  width: 70px;
  height: 70px;
  border-radius: 50%;
  background: radial-gradient(circle at 38% 36%, #fffaf0, var(--h-moon) 60%);
  box-shadow: 0 0 50px 18px var(--h-moonGlow);
}
.h-moon > span {
  position: absolute;
  border-radius: 50%;
}
/* A narrow column: no threads or moon over the greeting; lights and bunting stay. */
@container home (max-width: 899px) {
  .h-drop,
  .h-moon {
    display: none;
  }
}
</style>

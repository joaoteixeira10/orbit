<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useHolidayStore } from '@/stores/holiday'
import { particleLayout } from '@/lib/holidayThemes'

// The holiday's snow, bats, confetti or petals over the window: never a
// click, as many as fit the window's size (the counts are for 1440 × 900),
// still while the window is hidden. Mounted only with Effects on and motion
// allowed (Home decides).
const holiday = useHolidayStore()
const area = ref(1)
function measure() {
  area.value = Math.min(1, (window.innerWidth * window.innerHeight) / (1440 * 900))
}
let timer = 0
function onResize() {
  window.clearTimeout(timer)
  timer = window.setTimeout(measure, 200)
}
function onVisibility() {
  document.documentElement.classList.toggle('holiday-paused', document.visibilityState === 'hidden')
}
onMounted(() => {
  measure()
  window.addEventListener('resize', onResize)
  document.addEventListener('visibilitychange', onVisibility)
  onVisibility()
})
onUnmounted(() => {
  window.clearTimeout(timer)
  window.removeEventListener('resize', onResize)
  document.removeEventListener('visibilitychange', onVisibility)
  document.documentElement.classList.remove('holiday-paused')
})

const p = computed(() => holiday.active?.particles ?? null)
const items = computed(() => (p.value ? particleLayout(p.value, 0, area.value) : []))
</script>

<template>
  <div v-if="p" class="holiday-fx" aria-hidden="true" data-testid="holiday-fx">
    <template v-if="p.kind === 'fly'">
      <span
        v-for="(b, i) in items"
        :key="i"
        class="h-bat"
        :style="{ top: `${'topPx' in b ? b.topPx : 0}px`, opacity: b.opacity, animation: `h-${b.anim} ${b.dur}s ${b.delay}s linear infinite` }"
      >
        <svg
          viewBox="0 0 24 12"
          :width="b.size"
          :height="b.size / 2"
          :style="{ animation: `h-flap ${'flapDur' in b ? b.flapDur : 0.2}s ease-in-out infinite alternate` }"
        >
          <path :d="p.path" fill="var(--h-bat)" />
        </svg>
      </span>
    </template>
    <template v-else>
      <span
        v-for="(f, i) in items"
        :key="i"
        class="h-flake"
        :class="{ rise: p.kind === 'rise' }"
        :style="{ left: `${'leftPct' in f ? f.leftPct : 0}%`, opacity: f.opacity, animation: `h-${f.anim} ${f.dur}s ${f.delay}s linear infinite` }"
      >
        <svg
          :viewBox="p.viewBox"
          :width="f.size"
          :height="f.size"
          :style="{
            filter: p.kind === 'fall' && !p.spin && f.size > 3 ? 'blur(0.6px)' : undefined,
            animation:
              'spinDur' in f && f.spinDur
                ? `h-spin3d ${f.spinDur}s linear infinite`
                : 'swayDur' in f && f.swayDur
                  ? `h-sway ${f.swayDur}s ease-in-out infinite alternate`
                  : undefined,
          }"
        >
          <path :d="'shape' in f ? f.shape : ''" :fill="'color' in f ? f.color : ''" />
        </svg>
      </span>
    </template>
  </div>
</template>

<style scoped>
.holiday-fx {
  position: fixed;
  inset: 0;
  z-index: 40;
  overflow: hidden;
  pointer-events: none;
}
.h-flake,
.h-bat {
  position: absolute;
  display: block;
  will-change: transform;
}
.h-flake {
  top: -20px;
}
.h-flake.rise {
  top: 0;
}
.h-bat {
  left: 0;
}
.h-bat svg {
  transform-origin: center 35%;
}
.h-flake svg,
.h-bat svg {
  display: block;
}
</style>

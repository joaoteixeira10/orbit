<script setup lang="ts">
import { computed } from 'vue'
import { useHolidayStore } from '@/stores/holiday'
import { cobwebPath } from '@/lib/holidayThemes'

// A card's corner art: snow along the top edge (Christmas, winter: every
// card, the two drifts in turn), Halloween's cobweb in the top-right corner
// (the first two cards), or the theme's 40×40 picture (the first card only).
// `card` is the card's place on the page. Inside a card that is
// position: relative; it never takes a click.
const props = defineProps<{ card: number }>()
const holiday = useHolidayStore()
const corner = computed(() => {
  const c = holiday.look?.art.cardCorner
  if (!c || (c.kind === 'svg' && props.card !== 0) || (c.kind === 'cobweb' && props.card > 1)) return null
  return c
})
const web = cobwebPath()
</script>

<template>
  <template v-if="corner">
    <svg v-if="corner.kind === 'snowcap'" class="h-cap" viewBox="0 0 400 18" preserveAspectRatio="none" aria-hidden="true" data-testid="holiday-corner">
      <path :d="corner.paths[props.card % 2]" fill="var(--h-cap)" stroke="var(--h-capLine)" />
    </svg>
    <svg v-else-if="corner.kind === 'cobweb'" class="h-web" viewBox="0 0 64 64" aria-hidden="true" data-testid="holiday-corner">
      <path :d="web" fill="none" stroke="var(--h-web)" stroke-width="0.8" />
    </svg>
    <span v-else class="h-art" aria-hidden="true" data-testid="holiday-corner" v-html="corner.svg" />
  </template>
</template>

<style scoped>
.h-cap,
.h-web,
.h-art {
  position: absolute;
  pointer-events: none;
}
.h-cap {
  /* 7px above the card's edge — moved, not placed, so the card's column
     on Home doesn't change (an overflow above it upsets the balancing). */
  top: 0;
  transform: translateY(-7px);
  left: 16px;
  right: 16px;
  width: calc(100% - 32px);
  height: 14px;
  filter: drop-shadow(0 1px 1px rgba(0, 0, 0, 0.3));
}
.h-web {
  top: 0;
  right: 0;
  width: 76px;
  height: 76px;
}
.h-art {
  top: 12px;
  right: 14px;
  width: 40px;
  height: 40px;
}
</style>

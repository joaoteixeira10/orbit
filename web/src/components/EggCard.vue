<script setup lang="ts">
// Orbit only: the card behind the Konami codes (KonamiEgg's ↑↑↓↓←→←→ B A,
// TomasEgg's … T N). The app's own window (the sheet's ground, ring, radius
// and backdrop: every window looks the same), and the fun in it: the code
// lights up key by key, the flag waves, the name shimmers, confetti bursts
// from the card. Escape, a click outside or the close button put it away.
import { ref, watch, nextTick } from 'vue'
import { Github, X } from 'lucide-vue-next'
import { Button } from '@/components/shadcn/button'
import { moving } from '@/lib/motion'

const props = defineProps<{ open: boolean; name: string; quote: string; github: string; last: [string, string]; testid?: string }>()
const emit = defineEmits<{ close: [] }>()

const keys = ['↑', '↑', '↓', '↓', '←', '→', '←', '→']
const burstEl = ref<HTMLElement | null>(null)

// Confetti: little pieces thrown up and out from the card's top, then
// falling and turning as they go. Only when the app is moving.
const COLORS = ['var(--accent)', '#ff9f0a', '#34c759', '#ff375f', '#5e5ce6', '#ffd60a', '#64d2ff']
function burst() {
  const box = burstEl.value
  if (!box || !moving() || !box.animate) return
  for (let i = 0; i < 48; i++) {
    const p = document.createElement('i')
    const w = 5 + Math.random() * 5
    p.style.cssText = `width:${w}px;height:${w * (Math.random() < 0.5 ? 0.45 : 1)}px;background:${COLORS[i % COLORS.length]};border-radius:${Math.random() < 0.3 ? '50%' : '1px'}`
    box.appendChild(p)
    const a = -Math.PI / 2 + (Math.random() - 0.5) * Math.PI * 1.1
    const v = 140 + Math.random() * 190
    const dx = Math.cos(a) * v
    const dy = Math.sin(a) * v
    const spin = (Math.random() - 0.5) * 900
    const anim = p.animate(
      [
        { transform: 'translate(0, 0) rotate(0deg)', opacity: 1 },
        { transform: `translate(${dx * 0.8}px, ${dy * 0.8}px) rotate(${spin * 0.4}deg)`, opacity: 1, offset: 0.35 },
        { transform: `translate(${dx}px, ${dy + 260}px) rotate(${spin}deg)`, opacity: 0 },
      ],
      { duration: 1500 + Math.random() * 700, easing: 'cubic-bezier(0.2, 0.7, 0.4, 1)', delay: Math.random() * 120 },
    )
    anim.onfinish = () => p.remove()
  }
}
watch(
  () => props.open,
  (o) => o && void nextTick(burst),
)
</script>

<template>
  <Teleport to="body">
    <Transition name="sheet">
      <div
        v-if="open"
        class="sheet-backdrop fixed inset-0 z-700 flex items-center justify-center bg-black/10 supports-backdrop-filter:backdrop-blur-xs dark:bg-black/40"
        :data-testid="testid"
        @mousedown.self="emit('close')"
      >
        <div role="dialog" aria-modal="true" :aria-label="name" class="sheet-panel egg relative w-[calc(100%-32px)] max-w-[340px] px-8 pb-6 pt-7 text-center">
          <span ref="burstEl" class="burst" aria-hidden="true" />
          <Button variant="ghost" size="icon-sm" class="absolute right-2.5 top-2.5 text-text-2" aria-label="Close" @click="emit('close')"><X /></Button>

          <div class="flag mb-3 text-[46px] leading-none" aria-hidden="true">🇵🇹</div>
          <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-widest text-text-3">Engineered by</div>
          <div class="name mb-1 text-[26px] font-bold">{{ name }}</div>
          <div class="mb-4 text-[13px] text-text-2">in Portugal</div>

          <!-- The code, lighting up as if just typed. -->
          <div class="mb-5 flex flex-wrap justify-center gap-1" aria-hidden="true">
            <kbd v-for="(k, i) in [...keys, ...last]" :key="i" class="key" :style="{ animationDelay: `${120 + i * 55}ms` }">{{ k }}</kbd>
          </div>

          <div class="mb-5 text-[15px] italic text-accent">“{{ quote }}”</div>
          <Button variant="outline" size="sm" as="a" class="rounded-full" :href="`https://github.com/${github}`" target="_blank" rel="noopener">
            <Github />
            github.com/{{ github }}
          </Button>
          <span class="mt-4 block text-[11px] text-text-3">Esc or click outside to close</span>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* The app's window (components/ui/Overlay.vue's .sheet): same ground,
   ring, radius and shadow. */
.egg {
  background: var(--sheet-bg);
  border-radius: 12px;
  box-shadow: 0 0 0 1px var(--sheet-ring), var(--shadow-lg);
}
.burst {
  position: absolute;
  left: 50%;
  top: 56px;
  width: 0;
  height: 0;
  pointer-events: none;
}
.burst :deep(i) {
  position: absolute;
  left: 0;
  top: 0;
  display: block;
}
.flag {
  display: inline-block;
  transform-origin: 15% 90%;
  animation: egg-wave 2.4s ease-in-out infinite;
}
@keyframes egg-wave {
  0%,
  100% {
    rotate: -4deg;
  }
  50% {
    rotate: 5deg;
  }
}
.name {
  background: linear-gradient(100deg, var(--text) 0 38%, var(--accent) 50%, var(--text) 62% 100%);
  background-size: 260% 100%;
  background-position: 100% 0;
  -webkit-background-clip: text;
  background-clip: text;
  color: transparent;
  animation: egg-shine 2.8s 0.5s ease-in-out infinite;
}
@keyframes egg-shine {
  0% {
    background-position: 100% 0;
  }
  45%,
  100% {
    background-position: 0 0;
  }
}
.key {
  display: inline-grid;
  place-items: center;
  min-width: 22px;
  height: 22px;
  padding: 0 4px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-2);
  font: 600 11px/1 var(--font-mono, ui-monospace, monospace);
  box-shadow: 0 1px 0 var(--border);
  animation: egg-key 0.5s ease-out both;
}
@keyframes egg-key {
  0% {
    translate: 0 1px;
    box-shadow: none;
  }
  40% {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-fg);
  }
  100% {
    color: var(--text-2);
  }
}
</style>

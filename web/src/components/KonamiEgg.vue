<script setup lang="ts">
// SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).
// Change it there, not here: the same file is in every project.
import { ref, onMounted, onUnmounted } from 'vue'
import { Github } from 'lucide-vue-next'

// Developer easter egg: the Konami code (↑↑↓↓←→←→ B A) reveals a little card.
const SEQ = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'b', 'a']
const open = ref(false)
let pos = 0

function onKey(e: KeyboardEvent) {
  if (open.value && e.key === 'Escape') {
    open.value = false
    return
  }
  const key = e.key.length === 1 ? e.key.toLowerCase() : e.key
  if (key === SEQ[pos]) {
    pos++
    if (pos === SEQ.length) {
      pos = 0
      open.value = true
    }
  } else {
    pos = key === SEQ[0] ? 1 : 0
  }
}

onMounted(() => document.addEventListener('keydown', onKey))
onUnmounted(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-[700] flex items-center justify-center bg-black/55"
      @click="open = false"
    >
      <div class="egg-card w-[90%] max-w-[320px] rounded-2xl border border-border-2 bg-surface px-10 pb-6 pt-8 text-center shadow-2xl">
        <div class="mb-3 text-[46px] leading-none">🇵🇹</div>
        <div class="mb-1.5 text-[10px] font-semibold uppercase tracking-[0.1em] text-text-3">Engineered by</div>
        <div class="mb-1 text-[26px] font-bold text-text">João Teixeira</div>
        <div class="mb-4 text-[13px] text-text-2">in Portugal</div>
        <div class="mx-auto mb-4 h-[3px] w-9 rounded bg-accent"></div>
        <div class="mb-5 text-[14px] italic text-accent">"Build your own"</div>
        <a
          class="inline-flex items-center gap-2 rounded-full border border-border-2 bg-surface-2 px-[18px] py-1.5 text-[12px] text-text transition hover:bg-surface-3"
          href="https://github.com/joaoteixeira10"
          target="_blank"
          rel="noopener"
          @click.stop
        >
          <Github class="h-4 w-4" />
          github.com/joaoteixeira10
        </a>
        <span class="mt-3.5 block text-[11px] text-text-3">Click anywhere to close</span>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.egg-card {
  animation: egg-pop 0.22s cubic-bezier(0.34, 1.56, 0.64, 1);
}
@keyframes egg-pop {
  from {
    transform: scale(0.82);
    opacity: 0;
  }
  to {
    transform: scale(1);
    opacity: 1;
  }
}
</style>

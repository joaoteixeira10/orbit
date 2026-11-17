<script setup lang="ts">
// Orbit only (not the shared KonamiEgg): the Konami arrows ending in T N
// (↑↑↓↓←→←→ T N) reveal a second little card.
import { ref, onMounted, onUnmounted } from 'vue'
import EggCard from '@/components/EggCard.vue'

const SEQ = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 't', 'n']
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
  <EggCard :open="open" name="Tomás Neto" quote="Não tentes. Faz!" github="Tomasneto404" :last="['T', 'N']" testid="egg-tn" @close="open = false" />
</template>

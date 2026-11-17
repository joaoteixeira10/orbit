<script setup lang="ts">
// SHARED — from ../blueprint/shared/dev-themes (copied by blueprint/sync-themes.sh).
// Orbit's copy now shows its card through EggCard.vue (Orbit's window look,
// keys, confetti): see PORTING.md before syncing over it.
import { ref, onMounted, onUnmounted } from 'vue'
import EggCard from '@/components/EggCard.vue'

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
  <EggCard :open="open" name="João Teixeira" quote="Build your own" github="joaoteixeira10" :last="['B', 'A']" @close="open = false" />
</template>

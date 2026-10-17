<script setup lang="ts">
import { computed } from 'vue'
import type { MathLine } from '@/lib/mathnotes'

// The answers of a textarea's sums (lib/mathnotes), laid over it: the same
// text, invisible and wrapped the same way, with each answer showing after
// its "=". Give it the textarea's classes, so the type and padding match,
// and put both in a .math-wrap.
const props = defineProps<{ text: string; answers: MathLine[] }>()
const lines = computed(() => props.text.split('\n'))
const at = computed(() => new Map(props.answers.map((a) => [a.line, a.result])))
</script>

<template>
  <div v-if="answers.length" class="math-ghost" aria-hidden="true"><template v-for="(l, i) in lines" :key="i"><span class="mg-t">{{ l }}</span><span v-if="at.get(i)" class="mg-r">{{ /\s$/.test(l) ? '' : ' ' }}{{ at.get(i) }}</span>{{ i < lines.length - 1 ? '\n' : '' }}</template></div>
</template>

<style scoped>
.math-ghost {
  position: absolute !important;
  inset: 0;
  z-index: 1;
  margin: 0 !important;
  overflow: hidden;
  white-space: pre-wrap;
  overflow-wrap: break-word;
  background: transparent !important;
  box-shadow: none !important;
  pointer-events: none;
}
.mg-t {
  color: transparent;
}
.mg-r {
  color: var(--accent-text);
  opacity: 0.75;
  font-weight: 500;
}
</style>

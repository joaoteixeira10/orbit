<script setup lang="ts">
import { computed } from 'vue'

// A round avatar: the photo, else initials on a colour picked from the name
// (the same name always gets the same colour), like Contacts.
const props = withDefaults(defineProps<{ name: string; src?: string; size?: number }>(), { src: '', size: 28 })
const palette = ['#FF9500', '#34C759', '#007AFF', '#5856D6', '#AF52DE', '#FF2D55', '#30B0C7', '#A2845E']
const initials = computed(() =>
  props.name
    .split(/[\s._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]!.toUpperCase())
    .join('') || '?',
)
const bg = computed(() => {
  let h = 0
  for (const c of props.name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return palette[h % palette.length]
})
</script>

<template>
  <img v-if="src" :src="src" :alt="name" class="shrink-0 rounded-full object-cover" :style="{ width: size + 'px', height: size + 'px' }" />
  <span
    v-else
    class="grid shrink-0 select-none place-items-center rounded-full font-semibold text-white"
    :style="{ width: size + 'px', height: size + 'px', fontSize: size * 0.38 + 'px', background: `linear-gradient(180deg, color-mix(in srgb, ${bg} 82%, #fff), ${bg})` }"
    :title="name"
    >{{ initials }}</span
  >
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ICONS } from '@/lib/icons'

// A coloured icon tile (iOS Settings style) for a workspace or a list: a
// lucide icon by name, or an emoji / letter when the name isn't one.
const props = withDefaults(defineProps<{ icon?: string; color?: string; size?: number; name?: string }>(), {
  icon: '',
  color: '#007AFF',
  size: 22,
  name: '',
})
const comp = computed(() => ICONS[props.icon])
const glyph = computed(() => props.icon || props.name.slice(0, 1).toUpperCase() || '•')
</script>

<template>
  <span class="tile" :style="{ '--tile': color, width: size + 'px', height: size + 'px', borderRadius: Math.round(size * 0.28) + 'px' }">
    <component :is="comp" v-if="comp" :style="{ width: size * 0.6 + 'px', height: size * 0.6 + 'px' }" :stroke-width="2.2" />
    <span v-else :style="{ fontSize: size * 0.55 + 'px', lineHeight: 1 }">{{ glyph }}</span>
  </span>
</template>

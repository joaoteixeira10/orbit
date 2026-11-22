<script setup lang="ts">
import { ICONS, ICON_CHOICES, COLORS } from '@/lib/icons'
import { Check } from '@lucide/vue'

// Pick a colour (Apple's system palette) and an icon, as Reminders does for lists.
const icon = defineModel<string>('icon', { default: '' })
const color = defineModel<string>('color', { default: '#007AFF' })
defineProps<{ compact?: boolean }>()
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap gap-2.5">
      <button
        v-for="c in COLORS"
        :key="c"
        type="button"
        class="grid size-7 place-items-center rounded-full ring-1 ring-black/10 transition-transform hover:scale-110 dark:ring-white/15"
        :style="{ background: c, boxShadow: color.toLowerCase() === c.toLowerCase() ? `0 0 0 2px var(--surface), 0 0 0 4px ${c}` : 'none' }"
        :aria-label="c"
        @click="color = c"
      >
        <Check v-if="color.toLowerCase() === c.toLowerCase()" class="h-3.5 w-3.5 text-white" :stroke-width="3" />
      </button>
    </div>
    <div class="grid gap-1.5" :class="compact ? 'grid-cols-8' : 'grid-cols-8 sm:grid-cols-10'">
      <button
        v-for="k in ICON_CHOICES"
        :key="k"
        type="button"
        class="grid aspect-square place-items-center rounded-md transition-colors"
        :class="icon === k ? 'text-white' : 'text-text-2 hover:bg-fill'"
        :style="icon === k ? { background: color } : {}"
        :aria-label="k"
        @click="icon = k"
      >
        <component :is="ICONS[k]" class="h-[17px] w-[17px]" :stroke-width="2" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ status: string }>()

// Map domain statuses to the semantic color ramp.
const tone = computed(() => {
  switch (props.status) {
    case 'valid':
    case 'active':
      return 'ok'
    case 'expiring':
    case 'pending_chain':
      return 'warn'
    case 'revoked':
    case 'expired':
      return 'crit'
    case 'offline_sealed':
      return 'neutral'
    default:
      return 'neutral'
  }
})

const label = computed(() => props.status.replace(/_/g, ' '))
</script>

<template>
  <span
    class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium capitalize"
    :class="{
      'bg-ok-bg text-ok-text': tone === 'ok',
      'bg-warn-bg text-warn-text': tone === 'warn',
      'bg-crit-bg text-crit-text': tone === 'crit',
      'bg-surface-3 text-text-2': tone === 'neutral',
    }"
  >
    {{ label }}
  </span>
</template>

<script setup lang="ts">
const props = defineProps<{ modelValue: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="props.modelValue"
    :disabled="props.disabled"
    class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50"
    :class="props.modelValue ? 'bg-accent' : 'bg-border-strong'"
    @click="props.disabled || emit('update:modelValue', !props.modelValue)"
  >
    <span
      class="inline-block h-4 w-4 transform rounded-full bg-surface shadow transition-transform"
      :class="props.modelValue ? 'translate-x-[18px]' : 'translate-x-0.5'"
    />
  </button>
</template>

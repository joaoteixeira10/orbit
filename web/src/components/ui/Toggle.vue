<script setup lang="ts">
// An iOS switch: green when on.
const props = defineProps<{ modelValue: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="props.modelValue"
    :disabled="props.disabled"
    class="relative inline-flex h-[24px] w-[40px] shrink-0 items-center rounded-full transition-colors duration-200 disabled:cursor-not-allowed disabled:opacity-50"
    :style="{ background: props.modelValue ? 'var(--ok)' : 'var(--fill-strong)' }"
    @click="props.disabled || emit('update:modelValue', !props.modelValue)"
  >
    <span
      class="inline-block h-[20px] w-[20px] rounded-full bg-white transition-transform duration-200"
      style="box-shadow: 0 2px 4px rgba(0, 0, 0, 0.18), 0 0 0 0.5px rgba(0, 0, 0, 0.04); transition-timing-function: var(--spring)"
      :class="props.modelValue ? 'translate-x-[18px]' : 'translate-x-[2px]'"
    />
  </button>
</template>

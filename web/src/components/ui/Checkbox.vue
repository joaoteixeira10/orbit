<script setup lang="ts">
import { Check } from 'lucide-vue-next'

// A macOS-style square checkbox with an optional label.
const props = withDefaults(defineProps<{ modelValue?: boolean; disabled?: boolean }>(), {
  modelValue: false,
  disabled: false,
})
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()

function toggle() {
  if (!props.disabled) emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    class="flex items-center gap-2 text-left text-[13.5px] transition"
    :class="disabled ? 'cursor-not-allowed opacity-50' : ''"
    @click="toggle"
  >
    <span
      class="flex h-[17px] w-[17px] shrink-0 items-center justify-center rounded-[5px] transition-colors"
      :class="modelValue ? 'bg-accent text-accent-fg' : 'bg-surface shadow-[inset_0_0_0_1px_var(--border-2)]'"
    >
      <Check v-if="modelValue" class="h-3 w-3" :stroke-width="3.2" />
    </span>
    <span v-if="$slots.default"><slot /></span>
  </button>
</template>

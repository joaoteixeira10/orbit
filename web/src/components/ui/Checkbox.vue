<script setup lang="ts">
import { Check } from 'lucide-vue-next'

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
    class="flex items-center gap-2 text-left text-[13px] transition"
    :class="disabled ? 'cursor-not-allowed opacity-50' : ''"
    @click="toggle"
  >
    <span
      class="flex h-4 w-4 shrink-0 items-center justify-center rounded border"
      :class="modelValue ? 'border-accent bg-accent text-white' : 'border-border-2'"
    >
      <Check v-if="modelValue" class="h-3 w-3" />
    </span>
    <span v-if="$slots.default"><slot /></span>
  </button>
</template>

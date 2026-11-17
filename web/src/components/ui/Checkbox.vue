<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { Checkbox } from '@/components/shadcn/checkbox'

// A checkbox (shadcn-vue's, reka-ui) with an optional label beside it: a
// class styles the label; the rest (aria-label, a test id) goes to the box.
defineOptions({ inheritAttrs: false })
const props = withDefaults(defineProps<{ modelValue?: boolean; disabled?: boolean }>(), {
  modelValue: false,
  disabled: false,
})
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
const attrs = useAttrs()
const boxAttrs = computed(() => {
  const { class: _c, style: _s, ...rest } = attrs
  return rest
})
</script>

<template>
  <label class="flex items-center gap-2 text-left text-[13.5px]" :class="[attrs.class, disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer']" :style="attrs.style as any">
    <Checkbox v-bind="boxAttrs" :model-value="props.modelValue" :disabled="props.disabled" @update:model-value="(v) => emit('update:modelValue', v === true)" />
    <span v-if="$slots.default"><slot /></span>
  </label>
</template>

<script setup lang="ts">
import { Switch } from '@/components/shadcn/switch'

// A switch: shadcn-vue's (reka-ui), in the accent when on.
const props = defineProps<{ modelValue: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
</script>

<template>
  <Switch :model-value="props.modelValue" :disabled="props.disabled" @update:model-value="(v: boolean) => emit('update:modelValue', v)" />
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import Overlay from './Overlay.vue'

// typeToConfirm (optional): the user must type this exact text before the
// confirm button enables, as v0 did for risky changes.
const props = withDefaults(
  defineProps<{ open: boolean; title: string; message: string; confirmLabel?: string; danger?: boolean; typeToConfirm?: string }>(),
  { confirmLabel: 'Confirm', danger: false, typeToConfirm: '' },
)
const emit = defineEmits<{ confirm: []; close: [] }>()

const typed = ref('')
watch(
  () => props.open,
  (o) => {
    if (o) typed.value = ''
  },
)
const ready = computed(() => !props.typeToConfirm || typed.value.trim() === props.typeToConfirm)
</script>

<template>
  <Overlay :open="open" :title="title" mode="centered" @close="emit('close')">
    <p class="whitespace-pre-line text-[13px] leading-relaxed text-text-2">{{ message }}</p>
    <label v-if="typeToConfirm" class="mt-4 block text-[12px] text-text-2">
      Type <code class="font-semibold text-text">{{ typeToConfirm }}</code> to confirm
      <input v-model="typed" class="input mt-1.5 font-mono" autocomplete="off" spellcheck="false" @keydown.enter="ready && emit('confirm')" />
    </label>
    <div class="mt-5 flex justify-end gap-2">
      <button class="btn-ghost" @click="emit('close')">Cancel</button>
      <button :class="danger ? 'btn-danger' : 'btn-primary'" :disabled="!ready" @click="emit('confirm')">
        {{ confirmLabel }}
      </button>
    </div>
  </Overlay>
</template>

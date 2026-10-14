<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Overlay from './Overlay.vue'

// An alert-style confirmation. typeToConfirm (optional): the user must type
// this exact text before the confirm button enables (deleting a workspace).
const props = withDefaults(
  defineProps<{ open: boolean; title: string; message: string; confirmLabel?: string; danger?: boolean; typeToConfirm?: string }>(),
  { confirmLabel: '', danger: false, typeToConfirm: '' },
)
const emit = defineEmits<{ confirm: []; close: [] }>()
const { t } = useI18n()

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
  <Overlay :open="open" size="sm" @close="emit('close')">
    <div class="pt-3 text-center">
      <h2 class="title-3">{{ title }}</h2>
      <p class="mt-1.5 whitespace-pre-line text-[13px] leading-relaxed text-text-2">{{ message }}</p>
    </div>
    <label v-if="typeToConfirm" class="mt-4 block text-[12.5px] text-text-2">
      {{ t('common.typeToConfirm') }} <b class="font-semibold text-text">{{ typeToConfirm }}</b>
      <input v-model="typed" class="input mt-1.5" autocomplete="off" spellcheck="false" @keydown.enter="ready && emit('confirm')" />
    </label>
    <div class="mt-5 grid grid-cols-2 gap-2">
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button :class="danger ? 'btn-danger' : 'btn-primary'" :disabled="!ready" @click="emit('confirm')">
        {{ confirmLabel || t('common.confirm') }}
      </button>
    </div>
  </Overlay>
</template>

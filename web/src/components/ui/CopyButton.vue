<script setup lang="ts">
// CopyButton copies `value` to the clipboard and briefly shows a check. Use it
// inline next to serials, fingerprints, PEM blobs, generated passwords/tokens.
import { ref, computed } from 'vue'
import { Copy, Check } from 'lucide-vue-next'
import { i18n } from '@/i18n'

const props = withDefaults(
  defineProps<{
    value: string
    label?: string // optional text shown next to the icon
    title?: string // tooltip / aria-label
  }>(),
  { label: '', title: '' },
)

// Global t: works without the plugin installed (the spec mounts it bare).
const t = i18n.global.t
const tip = computed(() => props.title || t('acct.copyToClipboard'))

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

async function copy() {
  try {
    await navigator.clipboard.writeText(props.value)
    copied.value = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 1500)
  } catch {
    /* clipboard blocked (insecure context / permission) — no-op */
  }
}
</script>

<template>
  <button
    type="button"
    class="inline-flex items-center gap-1 rounded text-text-3 transition hover:text-text"
    :title="tip"
    :aria-label="tip"
    @click.stop="copy"
  >
    <Check v-if="copied" class="h-3.5 w-3.5 text-ok-text" />
    <Copy v-else class="h-3.5 w-3.5" />
    <span v-if="label" class="text-[12px]">{{ copied ? t('settings.copied') : label }}</span>
  </button>
</template>

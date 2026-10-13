<script setup lang="ts">
// CopyButton copies `value` to the clipboard and briefly shows a check. Use it
// inline next to serials, fingerprints, PEM blobs, generated passwords/tokens.
import { ref } from 'vue'
import { Copy, Check } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    value: string
    label?: string // optional text shown next to the icon
    title?: string // tooltip / aria-label
  }>(),
  { label: '', title: 'Copy to clipboard' },
)

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
    :title="title"
    :aria-label="title"
    @click.stop="copy"
  >
    <Check v-if="copied" class="h-3.5 w-3.5 text-ok-text" />
    <Copy v-else class="h-3.5 w-3.5" />
    <span v-if="label" class="text-[12px]">{{ copied ? 'Copied' : label }}</span>
  </button>
</template>

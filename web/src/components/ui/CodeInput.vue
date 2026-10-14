<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'

// Segmented numeric code input (one box per digit) with auto-advance, backspace,
// arrow navigation, paste, and password-manager / browser one-time-code autofill.
const props = withDefaults(
  defineProps<{ modelValue: string; length?: number; autocomplete?: string }>(),
  { length: 6, autocomplete: 'one-time-code' },
)
const emit = defineEmits<{ 'update:modelValue': [string]; complete: [string] }>()
const { t } = useI18n()

const boxes = ref<HTMLInputElement[]>([])
const digits = ref<string[]>(Array.from({ length: props.length }, () => ''))

// Reset boxes when the parent clears the value (e.g. after an error).
watch(
  () => props.modelValue,
  (v) => {
    if (v === '') digits.value = Array.from({ length: props.length }, () => '')
  },
)

function setRef(el: any, i: number) {
  if (el) boxes.value[i] = el as HTMLInputElement
}
function focusBox(i: number) {
  nextTick(() => boxes.value[i]?.focus())
}
function emitAll() {
  const s = digits.value.join('')
  emit('update:modelValue', s)
  if (s.length === props.length && !digits.value.includes('')) emit('complete', s)
}
// Spread a whole code across every box (paste, or a password manager that drops
// the full value into a single field).
function fill(text: string) {
  const t = text.replace(/\D/g, '').slice(0, props.length)
  for (let i = 0; i < props.length; i++) {
    digits.value[i] = t[i] || ''
    if (boxes.value[i]) boxes.value[i].value = digits.value[i]
  }
  emitAll()
  focusBox(Math.min(t.length, props.length - 1))
}
function onInput(i: number, e: Event) {
  const el = e.target as HTMLInputElement
  const v = el.value.replace(/\D/g, '')
  // Autofill (Bitwarden, browser one-time-code, …) sets the whole code into one
  // box: distribute it instead of keeping a single digit.
  if (v.length > 1) {
    fill(v)
    return
  }
  digits.value[i] = v ? v[v.length - 1] : ''
  el.value = digits.value[i]
  emitAll()
  if (v && i < props.length - 1) focusBox(i + 1)
}
function onKeydown(i: number, e: KeyboardEvent) {
  if (e.key === 'Backspace' && !digits.value[i] && i > 0) focusBox(i - 1)
  else if (e.key === 'ArrowLeft' && i > 0) focusBox(i - 1)
  else if (e.key === 'ArrowRight' && i < props.length - 1) focusBox(i + 1)
}
function onPaste(e: ClipboardEvent) {
  e.preventDefault()
  fill(e.clipboardData?.getData('text') || '')
}
</script>

<template>
  <div class="flex gap-2" @paste="onPaste">
    <input
      v-for="(_, i) in digits"
      :key="i"
      :ref="(el) => setRef(el, i)"
      :value="digits[i]"
      type="text"
      inputmode="numeric"
      :autocomplete="autocomplete"
      :name="i === 0 ? 'one-time-code' : undefined"
      :aria-label="t('acct.digit', { n: i + 1 })"
      maxlength="1"
      class="h-12 w-11 rounded-xl border border-border bg-surface-2 text-center text-[18px] font-semibold outline-none focus:border-accent"
      @input="onInput(i, $event)"
      @keydown="onKeydown(i, $event)"
      @focus="(e) => (e.target as HTMLInputElement).select()"
    />
  </div>
</template>

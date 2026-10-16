<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NodeViewWrapper, NodeViewContent, nodeViewProps } from '@tiptap/vue-3'
import { LANGUAGES, languageOf } from '@/lib/syntax'

// The code block's node view: the code, and (top right, on hover or focus)
// which language it's highlighted as — "Auto" detects it. A fence language
// Orbit doesn't know (```openssl) stays as written and is listed as is.
const props = defineProps(nodeViewProps)
const { t } = useI18n()

const current = computed(() => (props.node.attrs.language as string | null) ?? '')
const known = computed(() => languageOf(current.value))
const options = computed(() => {
  const list = LANGUAGES.map((l) => ({ id: l.id, label: l.label }))
  if (current.value && !LANGUAGES.some((l) => l.id === current.value)) list.unshift({ id: current.value, label: current.value })
  return list
})
function pick(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  props.updateAttributes({ language: v || null })
}
</script>

<template>
  <NodeViewWrapper class="code-block" :data-known="known || undefined">
    <select
      v-if="editor.isEditable"
      class="code-lang"
      contenteditable="false"
      :value="current"
      :title="t('editor.codeLanguage')"
      :aria-label="t('editor.codeLanguage')"
      @change="pick"
    >
      <option value="">{{ t('editor.codeAuto') }}</option>
      <option v-for="o in options" :key="o.id" :value="o.id">{{ o.label }}</option>
    </select>
    <pre><NodeViewContent as="code" /></pre>
  </NodeViewWrapper>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NodeViewWrapper, NodeViewContent, nodeViewProps } from '@tiptap/vue-3'
import EmojiPicker from './EmojiPicker.vue'

// The callout's node view: its emoji is a button (change it) outside the
// editable content.
const props = defineProps(nodeViewProps)
const { t } = useI18n()
const btn = ref<HTMLElement | null>(null)
const picking = ref(false)

function toggle() {
  if (props.editor.isEditable) picking.value = !picking.value
}
</script>

<template>
  <NodeViewWrapper class="callout" data-type="callout">
    <button
      ref="btn"
      type="button"
      class="callout-emoji"
      contenteditable="false"
      :title="editor.isEditable ? t('editor.calloutEmoji') : undefined"
      :aria-label="t('editor.calloutEmoji')"
      @click="toggle"
    >
      {{ node.attrs.emoji || '💡' }}
    </button>
    <NodeViewContent class="callout-body" />
    <EmojiPicker v-if="picking" :anchor="btn" @pick="(e) => updateAttributes({ emoji: e })" @close="picking = false" />
  </NodeViewWrapper>
</template>

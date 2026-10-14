<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { NodeViewWrapper, NodeViewContent, nodeViewProps } from '@tiptap/vue-3'
import { Check } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { ALERTS, alertOf, type AlertType } from './alerts'

// The callout's node view: its icon is a button (outside the editable
// content) that picks which of GitHub's five alerts it is — note, tip,
// important, warning, caution — since that is how it's written in Markdown.
const props = defineProps(nodeViewProps)
const { t } = useI18n()
const btn = ref<HTMLElement | null>(null)
const picking = ref(false)
const kind = computed(() => alertOf(props.node.attrs.emoji))
const kinds = Object.keys(ALERTS) as AlertType[]

function toggle() {
  if (props.editor.isEditable) picking.value = !picking.value
}
function pick(k: AlertType) {
  props.updateAttributes({ emoji: ALERTS[k] })
  picking.value = false
}
</script>

<template>
  <NodeViewWrapper class="callout" :class="`callout-${kind.toLowerCase()}`" data-type="callout">
    <button
      ref="btn"
      type="button"
      class="callout-emoji"
      contenteditable="false"
      :title="editor.isEditable ? t('editor.alertPick') : t(`editor.alert.${kind}`)"
      :aria-label="t('editor.alertPick')"
      @click="toggle"
    >
      {{ ALERTS[kind] }}
    </button>
    <NodeViewContent class="callout-body" />
    <Popover v-if="picking" :anchor="btn" :width="200" @close="picking = false">
      <button v-for="k in kinds" :key="k" type="button" class="menu-item" @click="pick(k)">
        <span class="w-5 text-center">{{ ALERTS[k] }}</span>
        <span class="flex-1">{{ t(`editor.alert.${k}`) }}</span>
        <Check v-if="k === kind" class="h-4 w-4 text-accent" />
      </button>
    </Popover>
  </NodeViewWrapper>
</template>

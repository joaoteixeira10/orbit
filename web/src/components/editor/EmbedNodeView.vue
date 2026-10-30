<script setup lang="ts">
import { NodeViewWrapper, nodeViewProps } from '@tiptap/vue-3'
import PageEmbed from '@/components/embeds/PageEmbed.vue'

// The embed block in the editor: the same as the reader sees (a double
// click opens it in the big editor). Dragged by its block handle.
const props = defineProps(nodeViewProps)
</script>

<template>
  <NodeViewWrapper class="doc-embed" :class="{ picked: props.selected }" data-type="embed" contenteditable="false">
    <PageEmbed :kind="props.node.attrs.kind" :ws="props.node.attrs.ws" :id="props.node.attrs.id" />
  </NodeViewWrapper>
</template>

<style scoped>
.doc-embed {
  border-radius: 13px;
}
.doc-embed.picked {
  box-shadow: 0 0 0 2px var(--accent);
}
.doc-embed :deep(.sheet-embed),
.doc-embed :deep(.diagram-embed),
.doc-embed :deep(.board-embed) {
  margin: 0;
}
</style>

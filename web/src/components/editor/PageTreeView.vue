<script setup lang="ts">
import { NodeViewWrapper, nodeViewProps } from '@tiptap/vue-3'
import SubpageList from '@/components/pages/SubpageList.vue'

// ::pagetree in the editor: the live list (it isn't edited; it follows the pages).
const props = defineProps(nodeViewProps)
</script>

<template>
  <NodeViewWrapper class="page-tree-node" :class="{ picked: props.selected }" contenteditable="false"><SubpageList /></NodeViewWrapper>
</template>

<style scoped>
.page-tree-node.picked {
  border-radius: 11px;
  box-shadow: 0 0 0 2px var(--accent);
}
</style>

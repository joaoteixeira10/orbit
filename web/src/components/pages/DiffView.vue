<script setup lang="ts">
import { computed } from 'vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { diffMarkdown, blocks } from '@/lib/diff'

// A page's text as it reads, with what changed since before marked: added
// blocks green, removed ones red and struck. No before: just the text.
const props = defineProps<{ after: string; before?: string | null }>()
const ops = computed(() => (props.before == null ? blocks(props.after).map((text) => ({ op: 'same' as const, text })) : diffMarkdown(props.before, props.after)))
</script>

<template>
  <div class="diff-view">
    <div v-for="(o, i) in ops" :key="i" class="d-block" :class="`d-${o.op}`" :data-op="o.op">
      <MarkdownView :content="o.text" :editable="false" />
    </div>
  </div>
</template>

<style scoped>
.d-block {
  position: relative;
  padding: 0 8px;
  border-radius: 6px;
}
.d-block :deep(.md-view) {
  padding-bottom: 0;
}
.d-block :deep(.md-view > :first-child) {
  margin-top: 0.35em;
}
.d-block :deep(.md-view > :last-child) {
  margin-bottom: 0.35em;
}
.d-add {
  background: color-mix(in srgb, #34c759 16%, transparent);
  box-shadow: inset 3px 0 0 #34c759;
}
.d-del {
  background: color-mix(in srgb, #ff3b30 13%, transparent);
  box-shadow: inset 3px 0 0 #ff3b30;
}
.d-del :deep(.md-view) {
  text-decoration: line-through;
  text-decoration-color: color-mix(in srgb, #ff3b30 60%, transparent);
  opacity: 0.75;
}
</style>

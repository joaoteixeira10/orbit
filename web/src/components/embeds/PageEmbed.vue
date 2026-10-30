<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Maximize2 } from 'lucide-vue-next'
import SheetEmbed from '@/components/sheets/SheetEmbed.vue'
import DiagramEmbed from '@/components/diagrams/DiagramEmbed.vue'
import WhiteboardEmbed from './WhiteboardEmbed.vue'
import { embedEdits, openEmbed, type EmbedKind } from '@/lib/embeds'

// A sheet, a diagram or a whiteboard in a page, as it reads; a double click
// (or the button in its corner) opens it in the big editor over the page,
// and it's drawn again when that closes.
const props = defineProps<{ kind: EmbedKind; ws: string; id: string }>()
const { t } = useI18n()
const view = computed(() => (props.kind === 'sheet' ? SheetEmbed : props.kind === 'diagram' ? DiagramEmbed : WhiteboardEmbed))
const open = () => openEmbed({ kind: props.kind, ws: props.ws, id: props.id })
</script>

<template>
  <div class="page-embed" :data-embed="kind" :title="t('embed.dblclick')" @dblclick.stop.prevent="open">
    <component :is="view" :key="embedEdits[id] ?? 0" :ws="ws" :id="id" />
    <button type="button" class="pe-expand" :aria-label="t('embed.expand')" :title="t('embed.expand')" data-testid="embed-expand" @click.stop="open"><Maximize2 class="h-3.5 w-3.5" /></button>
  </div>
</template>

<style scoped>
.page-embed {
  position: relative;
}
.pe-expand {
  position: absolute;
  right: 8px;
  bottom: 8px;
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 8px;
  background: color-mix(in srgb, var(--surface) 90%, transparent);
  box-shadow: 0 0 0 0.5px var(--border), 0 1px 3px rgba(0, 0, 0, 0.1);
  color: var(--text-2);
  opacity: 0;
  transition: opacity var(--speed);
}
.page-embed:hover .pe-expand,
.pe-expand:focus-visible {
  opacity: 1;
}
@media (hover: none) {
  .pe-expand {
    opacity: 1;
  }
}
</style>

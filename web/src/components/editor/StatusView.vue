<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NodeViewWrapper, nodeViewProps } from '@tiptap/vue-3'
import Popover from '@/components/ui/Popover.vue'
import { STATUS_COLORS } from './statusColors'

// A status badge in the editor: a click changes its words and colour.
const props = defineProps(nodeViewProps)
const { t } = useI18n()
const el = ref<HTMLElement | null>(null)
const open = ref(false)
const text = ref('')
const input = ref<HTMLInputElement | null>(null)
async function edit() {
  if (!props.editor.isEditable) return
  text.value = props.node.attrs.text
  open.value = true
  await nextTick()
  input.value?.select()
}
function set(patch: { text?: string; color?: string }) {
  props.updateAttributes(patch)
}
function done() {
  set({ text: text.value.trim().slice(0, 40) || props.node.attrs.text })
  open.value = false
}
</script>

<template>
  <NodeViewWrapper as="span" class="status-node">
    <span ref="el" class="status-badge" :data-status="node.attrs.color" role="button" tabindex="0" data-testid="status-badge" @click="edit">{{ node.attrs.text }}</span>
    <Popover v-if="open" :anchor="el" :width="220" @close="done">
      <form class="p-2" @submit.prevent="done">
        <input ref="input" v-model="text" class="field !py-1 text-[13px]" maxlength="40" :aria-label="t('blocks.statusText')" data-testid="status-text" />
        <div class="mt-2 flex flex-wrap gap-1.5">
          <button v-for="c in STATUS_COLORS" :key="c" type="button" class="status-badge" :data-status="c" :aria-label="c" @click="set({ color: c })">{{ c === node.attrs.color ? '✓' : '·' }}</button>
        </div>
      </form>
    </Popover>
  </NodeViewWrapper>
</template>

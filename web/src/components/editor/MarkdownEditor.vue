<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import CodeArea from './CodeArea.vue'
import { normalizeMarkdown } from '@/lib/markdown'
import { markdownFor, uploadFile } from '@/lib/uploads'
import { mentionItems } from '@/lib/mentions'
import { useToastStore } from '@/stores/toast'
import { useWorkspacesStore } from '@/stores/workspaces'
import { usePagesStore } from '@/stores/pages'

// The page body, edited as Markdown source, the way Visual Studio Code edits
// text (CodeArea: several cursors, moving lines, find and replace…), with
// the helps of a Markdown editor — Enter continues a list, task or quote
// (an empty item ends it), Tab / ⇧Tab indent, ⌘B ⌘I ⌘E ⌘K wrap the
// selection in bold, italic, code, a link — sums answered after "=", files
// pasted or dropped, @-mentions. Same contract as PageEditor: the parent
// saves; this hands out the Markdown.
const props = defineProps<{ content: string; editable: boolean; pageId?: string; ws?: string }>()
const emit = defineEmits<{ change: [] }>()
const { t } = useI18n()
const toast = useToastStore()
const wss = useWorkspacesStore()
const pagesStore = usePagesStore()

const text = ref(props.content)
const area = ref<InstanceType<typeof CodeArea> | null>(null)

// Files pasted or dropped in: uploaded to the page, their Markdown (an image
// or a link) put where they landed.
async function upload(f: File): Promise<string> {
  toast.info(t('editor.uploading', { name: f.name }))
  try {
    return markdownFor(await uploadFile('page', props.pageId!, f))
  } catch (e: any) {
    toast.error(e?.message ?? String(e))
    throw e
  }
}
const mentions = (q: string) =>
  props.ws ? mentionItems(q, { ws: props.ws, members: wss.members[props.ws] ?? [], pages: pagesStore.byWs[props.ws] ?? [], self: props.pageId }, t) : []

// getContent is the page's Markdown, tidied the way every save is.
function getContent(): string {
  return normalizeMarkdown(text.value)
}
// setContent replaces the text (someone else's save), keeping the caret.
function setContent(md: string) {
  text.value = md
}
function focusStart() {
  area.value?.focus('start')
}
defineExpose({ getContent, setContent, focusStart })
</script>

<template>
  <CodeArea
    ref="area"
    v-model="text"
    class="md-source"
    mono
    sums
    :readonly="!editable"
    :placeholder="t('editor.mdPlaceholder')"
    :label="t('editor.mdLabel')"
    :upload="pageId && editable ? upload : null"
    :mentions="ws ? mentions : null"
    testid="md-source"
    @change="emit('change')"
  />
</template>

<style scoped>
.md-source {
  display: block;
  min-height: 40vh;
  padding: 2px 0 24px;
  color: var(--text);
  font-size: 14px;
  line-height: 1.7;
}
</style>

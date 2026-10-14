<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { SquareCheck, Type } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import NoteFields from './NoteFields.vue'
import { draftOf, inputOf, isEmptyDraft, toChecklist, toText, type Draft } from './noteModel'
import { ensureNoteColorStyles } from './noteColors'
import { useNotesStore } from '@/stores/notes'
import { useToastStore } from '@/stores/toast'

// A quick note from anywhere (⌥N, or ⌘K → Quick note): a sheet to write in
// the moment, saved to the user's Quick Notes when it closes — no workspace
// to choose; that can wait.
ensureNoteColorStyles()
const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()
const notes = useNotesStore()
const toast = useToastStore()
const { t } = useI18n()

const draft = ref<Draft>(draftOf(null, { color: 'yellow' }))
const fields = ref<InstanceType<typeof NoteFields> | null>(null)
watch(
  () => props.open,
  async (o) => {
    if (!o) return
    draft.value = draftOf(null, { color: 'yellow' })
    await nextTick()
    void fields.value?.focus('body')
  },
)

async function close() {
  const d = draft.value
  emit('close')
  if (isEmptyDraft(d)) return
  try {
    await notes.create('me', inputOf(d))
    toast.success(t('notes.mine.saved'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
function toggleChecklist() {
  if (draft.value.checklist) toText(draft.value)
  else toChecklist(draft.value)
  void fields.value?.focus('end')
}
</script>

<template>
  <Overlay :open="open" size="md" @close="close">
    <div class="note-sheet pt-2.5" data-note="yellow">
      <NoteFields ref="fields" v-model="draft" big @submit="close" />
    </div>
    <template #footer>
      <div class="note-sheet flex min-w-0 flex-1 items-center gap-1" data-note="yellow">
        <button class="tb" :aria-label="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" :title="draft.checklist ? t('notes.hideChecks') : t('notes.showChecks')" @click="toggleChecklist">
          <Type v-if="draft.checklist" class="h-[18px] w-[18px]" />
          <SquareCheck v-else class="h-[18px] w-[18px]" />
        </button>
        <span class="ml-2 min-w-0 flex-1 truncate text-[12px] text-text-2">{{ t('notes.mine.title') }} · {{ t('notes.mine.onlyYou') }}</span>
        <button class="btn btn-sm done" @click="close">{{ t('common.done') }}</button>
      </div>
    </template>
  </Overlay>
</template>

<style scoped>
.tb {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 999px;
  color: var(--text-2);
}
.tb:hover {
  background: var(--fill);
  color: var(--text);
}
.done {
  background: var(--note-line);
  font-weight: 600;
}
</style>

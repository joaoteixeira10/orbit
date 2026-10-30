<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Undo2 } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { api, ApiError } from '@/lib/api'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { usePagesStore } from '@/stores/pages'
import { useToastStore } from '@/stores/toast'

// My recent changes to a workspace's pages — made, moved, renamed, copied,
// put in the trash (two weeks of them) — each undone with a click. One that
// changed since (someone wrote in the page, moved it again…) asks first.
interface Change {
  id: string
  page_id: string
  kind: 'create' | 'move' | 'rename' | 'copy' | 'trash'
  title: string
  icon?: string
  from_title?: string
  from_parent?: string | null
  to_parent?: string | null
  to_title?: string
  at: string
  undone: boolean
  changed: boolean
  gone: boolean
}
const props = defineProps<{ open: boolean; ws: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const pages = usePagesStore()
const toast = useToastStore()
const loc = locale()

const changes = ref<Change[]>([])
async function load() {
  try {
    changes.value = await api<Change[]>(`/me/page-changes?ws=${encodeURIComponent(props.ws)}`)
  } catch (e: any) {
    toast.error(e.message)
  }
}
watch(
  () => props.open,
  (o) => o && void load(),
  { immediate: true },
)
const whereOf = (c: Change) => (c.to_parent ? c.to_title || t('pages.untitled') : t('changes.top'))
function say(c: Change): string {
  const title = c.title || t('pages.untitled')
  switch (c.kind) {
    case 'create':
      return t('changes.create', { title })
    case 'move':
      return t('changes.move', { title, where: whereOf(c) })
    case 'rename':
      return t('changes.rename', { from: c.from_title || t('pages.untitled'), title })
    case 'copy':
      return t('changes.copy', { title })
    default:
      return t('changes.trash', { title })
  }
}
const asking = ref<Change | null>(null)
async function undo(c: Change, force = false) {
  asking.value = null
  try {
    await api(`/page-changes/${c.id}/undo${force ? '?force=1' : ''}`, { method: 'POST' })
    await Promise.all([load(), pages.load(props.ws)])
    toast.success(t('changes.undone'))
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 409 && e.body?.changed) asking.value = c
    else toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open" :title="t('changes.title')" mode="side" @close="emit('close')">
    <p class="mb-3 text-[12.5px] text-text-3">{{ t('changes.lead') }}</p>
    <p v-if="!changes.length" class="py-6 text-center text-[13px] text-text-3">{{ t('changes.none') }}</p>
    <div v-for="c in changes" :key="c.id" class="ch-row" :class="{ done: c.undone || c.gone }" data-testid="change-row">
      <div class="min-w-0 flex-1">
        <div class="text-[13px]">{{ say(c) }}</div>
        <div class="text-[11.5px] text-text-3">
          {{ ago(c.at, loc) }}<template v-if="c.undone"> · {{ t('changes.wasUndone') }}</template><template v-else-if="c.gone"> · {{ t('changes.gone') }}</template><template v-else-if="c.changed"> · {{ t('changes.changedSince') }}</template>
        </div>
      </div>
      <button v-if="!c.undone && !c.gone" type="button" class="btn btn-sm shrink-0" data-testid="change-undo" @click="undo(c)"><Undo2 class="h-3.5 w-3.5" />{{ t('changes.undo') }}</button>
    </div>
    <Confirm
      :open="!!asking"
      :title="t('changes.askTitle')"
      :message="t('changes.askBody', { title: asking?.title || t('pages.untitled') })"
      :confirm-label="t('changes.undoAnyway')"
      @confirm="asking && undo(asking, true)"
      @close="asking = null"
    />
  </Overlay>
</template>

<style scoped>
.ch-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 0.5px solid var(--border);
}
.ch-row.done {
  opacity: 0.55;
}
</style>

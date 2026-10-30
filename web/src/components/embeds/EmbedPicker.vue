<script setup lang="ts">
import { computed, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { Brush, Search, Sheet, Workflow } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import { filesApi, type FileItem, type FileKind } from '@/lib/filesApi'
import type { EmbedKind } from '@/lib/embeds'

// Picks a sheet, a diagram or a whiteboard there is in the workspace (of the
// kinds it has), to show in a page; newest change first, found by name.
const props = defineProps<{ open: boolean; ws: string; kinds: EmbedKind[] }>()
const emit = defineEmits<{ close: []; pick: [it: { kind: EmbedKind; id: string; title: string }] }>()
const { t } = useI18n()

const FILE: Record<EmbedKind, FileKind> = { sheet: 'sheet', diagram: 'diagram', board: 'board' }
const ICON: Record<FileKind, Component> = { sheet: Sheet, diagram: Workflow, board: Brush }
const COLOR: Record<FileKind, string> = { sheet: '#34c759', diagram: '#ff9500', board: '#af52de' }
const UNTITLED: Record<FileKind, string> = { sheet: 'sheets.untitled', diagram: 'diagrams.untitled', board: 'draw.untitled' }

const items = ref<FileItem[]>([])
const loading = ref(false)
const q = ref('')
watch(
  () => props.open,
  async (o) => {
    if (!o) return
    q.value = ''
    loading.value = true
    try {
      items.value = (await filesApi.files(props.ws, props.kinds.map((k) => FILE[k]))).sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    } catch {
      items.value = []
    } finally {
      loading.value = false
    }
  },
)
const shown = computed(() => {
  const s = q.value.trim().toLowerCase()
  return s ? items.value.filter((x) => x.title.toLowerCase().includes(s)) : items.value
})
function pick(x: FileItem) {
  const kind = (Object.keys(FILE) as EmbedKind[]).find((k) => FILE[k] === x.kind)!
  emit('pick', { kind, id: x.id, title: x.title || t(UNTITLED[x.kind]) })
}
</script>

<template>
  <Overlay :open="open" :title="t('embed.pickTitle')" @close="emit('close')">
    <label class="relative mb-3 block">
      <Search class="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-3" />
      <input v-model="q" class="field !pl-8" :placeholder="t('embed.pickSearch')" data-testid="embed-pick-search" />
    </label>
    <p v-if="loading" class="py-6 text-center text-[13px] text-text-3">{{ t('common.loading') }}</p>
    <p v-else-if="!shown.length" class="py-6 text-center text-[13px] text-text-3">{{ t('embed.pickNone') }}</p>
    <ul v-else class="flex flex-col">
      <li v-for="x in shown" :key="x.kind + x.id">
        <button type="button" class="menu-item w-full" data-testid="embed-pick-item" @click="pick(x)">
          <component :is="ICON[x.kind]" class="h-4 w-4 shrink-0" :style="{ color: COLOR[x.kind] }" />
          <span class="min-w-0 flex-1 truncate text-left">{{ x.title || t(UNTITLED[x.kind]) }}</span>
        </button>
      </li>
    </ul>
  </Overlay>
</template>

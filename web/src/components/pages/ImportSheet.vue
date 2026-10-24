<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Upload, FileText, NotebookPen, StickyNote, LoaderCircle } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import type { ImportResult } from '@/lib/importRun'
import { usePagesStore } from '@/stores/pages'
import { useToastStore } from '@/stores/toast'

// Importing pages: Markdown files, a Notion export, Apple Notes. One picker
// takes them all (zips too); the sheet says how to get each out of its app,
// then shows the progress and what came in.
const props = defineProps<{ open: boolean; ws: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const pages = usePagesStore()
const toast = useToastStore()

const input = ref<HTMLInputElement | null>(null)
const busy = ref(false)
const done = ref(0)
const total = ref(0)
const result = ref<ImportResult | null>(null)
watch(
  () => props.open,
  (o) => {
    if (o && !busy.value) result.value = null
  },
)

async function picked(e: Event) {
  const files = [...((e.target as HTMLInputElement).files ?? [])]
  ;(e.target as HTMLInputElement).value = ''
  if (!files.length) return
  busy.value = true
  done.value = 0
  total.value = 0
  try {
    // Loaded when needed: the unzipping, and the editor that reads HTML.
    const [{ gather }, { importPages }] = await Promise.all([import('@/lib/importPages'), import('@/lib/importRun')])
    const all = await gather(files)
    result.value = await importPages(props.ws, all, null, (d, n) => ((done.value = d), (total.value = n)))
    await pages.load(props.ws)
    if (result.value.pages) toast.success(t('pages.imported', { n: result.value.pages }, result.value.pages))
  } catch (err: any) {
    toast.error(err?.message ?? String(err))
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Overlay :open="open" :title="t('importer.title')" size="md" @close="busy || emit('close')">
    <div class="space-y-4" data-testid="import-sheet">
      <p class="text-[14px] leading-relaxed text-text-2">{{ t('importer.intro') }}</p>
      <div class="group">
        <div class="group-row !items-start">
          <span class="tile mt-0.5 h-7 w-7" style="--tile: #8e8e93"><FileText class="h-4 w-4" /></span>
          <div class="min-w-0 flex-1 py-0.5">
            <div class="text-[14px] font-medium">{{ t('importer.md') }}</div>
            <div class="text-[12.5px] leading-snug text-text-3">{{ t('importer.mdHow') }}</div>
          </div>
        </div>
        <div class="group-row !items-start">
          <span class="tile mt-0.5 h-7 w-7" style="--tile: #1d1d1f"><NotebookPen class="h-4 w-4" /></span>
          <div class="min-w-0 flex-1 py-0.5">
            <div class="text-[14px] font-medium">{{ t('importer.notion') }}</div>
            <div class="text-[12.5px] leading-snug text-text-3">{{ t('importer.notionHow') }}</div>
          </div>
        </div>
        <div class="group-row !items-start">
          <span class="tile mt-0.5 h-7 w-7" style="--tile: #ffcc00"><StickyNote class="h-4 w-4" /></span>
          <div class="min-w-0 flex-1 py-0.5">
            <div class="text-[14px] font-medium">{{ t('importer.apple') }}</div>
            <div class="text-[12.5px] leading-snug text-text-3">{{ t('importer.appleHow') }}</div>
          </div>
        </div>
      </div>

      <div v-if="busy" class="flex items-center gap-2 rounded-xl bg-fill-2 px-4 py-3 text-[13.5px]" aria-live="polite">
        <LoaderCircle class="h-4 w-4 animate-spin" />
        {{ total ? t('importer.progress', { done, total }) : t('importer.reading') }}
      </div>
      <div v-else-if="result" class="rounded-xl bg-fill-2 px-4 py-3 text-[13.5px]" aria-live="polite" data-testid="import-result">
        {{ t('importer.result', { pages: t('importer.pagesN', { n: result.pages }, result.pages), files: t('importer.filesN', { n: result.files }, result.files) }) }}
        <p v-if="result.failed.length" class="mt-1 text-crit-text">{{ t('importer.failed', { list: result.failed.slice(0, 5).join(', ') }, result.failed.length) }}</p>
      </div>
      <input
        ref="input"
        type="file"
        class="hidden"
        multiple
        accept=".md,.markdown,.txt,.html,.htm,.csv,.zip,text/markdown,text/plain,text/html,application/zip"
        data-testid="import-input"
        @change="picked"
      />
    </div>
    <template #footer>
      <button type="button" class="btn" :disabled="busy" @click="emit('close')">{{ result ? t('common.close') : t('common.cancel') }}</button>
      <button type="button" class="btn-primary" :disabled="busy" @click="input?.click()"><Upload class="h-4 w-4" />{{ t('importer.choose') }}</button>
    </template>
  </Overlay>
</template>

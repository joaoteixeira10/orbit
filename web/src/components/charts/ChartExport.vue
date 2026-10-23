<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Download, Image, Sheet } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { chartPng, chartRows, csvOf, saveBlob, type ChartSpec } from '@/lib/chartExport'
import { fileName } from '@/lib/markdown'
import { useToastStore } from '@/stores/toast'

// A chart card's download button: the chart as a picture or its numbers.
const props = defineProps<{ spec: () => ChartSpec; name: string }>()
const { t } = useI18n()
const toast = useToastStore()
const anchor = ref<HTMLElement | null>(null)
const open = ref(false)

const base = () => `${fileName(props.name)}-${new Date().toISOString().slice(0, 10)}`
async function png() {
  open.value = false
  try {
    saveBlob(`${base()}.png`, await chartPng(props.spec()))
  } catch (e: any) {
    toast.error(e.message)
  }
}
function csv() {
  open.value = false
  saveBlob(`${base()}.csv`, new Blob([csvOf(chartRows(props.spec(), t('charts.total')))], { type: 'text/csv;charset=utf-8' }))
}
</script>

<template>
  <button ref="anchor" type="button" class="icon-btn h-7 w-7 shrink-0" :title="t('charts.export')" :aria-label="t('charts.export')" data-testid="chart-export" @click="open = !open">
    <Download class="h-4 w-4" />
  </button>
  <Popover v-if="open" :anchor="anchor" placement="bottom-end" :width="200" @close="open = false">
    <div class="p-1.5">
      <button type="button" class="menu-item w-full" data-testid="chart-png" @click="png"><Image class="h-4 w-4 text-text-2" />{{ t('charts.png') }}</button>
      <button type="button" class="menu-item w-full" data-testid="chart-csv" @click="csv"><Sheet class="h-4 w-4 text-text-2" />{{ t('charts.csv') }}</button>
    </div>
  </Popover>
</template>

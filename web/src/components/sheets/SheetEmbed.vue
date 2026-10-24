<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ExternalLink, Sheet } from 'lucide-vue-next'
import { sheetsApi, type SheetDoc } from '@/lib/sheetsApi'
import { Book, addr, colName, parseAddr, readData, show, type SheetData } from '@/lib/sheet'
import { dayOf } from '@/lib/formula'
import { locale, todayStr } from '@/lib/dates'

// A sheet shown in a page (a link to it alone on its line): its cells as
// they read, a tab at a time, the used part (up to 50 rows and 15
// columns), and a way to open it.
const props = defineProps<{ ws: string; id: string }>()
const { t } = useI18n()
const router = useRouter()

const doc = ref<SheetDoc | null>(null)
const data = ref<SheetData | null>(null)
const failed = ref(false)
const tabId = ref('')
watch(
  () => props.id,
  async (id) => {
    try {
      doc.value = await sheetsApi.get(id)
      data.value = readData(doc.value.data, t('sheets.tabName', { n: 1 }))
      tabId.value = data.value.tabs[0].id
    } catch {
      failed.value = true
    }
  },
  { immediate: true },
)
const loc = locale()
const book = computed(() => (data.value ? new Book(data.value, dayOf(todayStr()), loc.startsWith('pt')) : null))
const tab = computed(() => data.value?.tabs.find((x) => x.id === tabId.value))
const size = computed(() => {
  let rows = 0
  let cols = 0
  for (const [a, c] of Object.entries(tab.value?.cells ?? {})) {
    const p = parseAddr(a)
    if (p && c.v !== '') {
      rows = Math.max(rows, p.r + 1)
      cols = Math.max(cols, p.c + 1)
    }
  }
  return { rows: Math.min(rows, 50), cols: Math.min(cols, 15), more: rows > 50 || cols > 15 }
})
const words = computed(() => ({ yes: t('sheets.true'), no: t('sheets.false') }))
function cell(r: number, c: number) {
  const tb = tab.value!
  return show(book.value!.value(tb.id, r, c), tb.cells[addr(r, c)], loc, words.value)
}
const open = () => router.push({ name: 'ws-sheet', params: { ws: props.ws, id: props.id } })
</script>

<template>
  <div class="sheet-embed" data-testid="sheet-embed">
    <div class="se-head">
      <Sheet class="h-4 w-4 text-[#34c759]" />
      <span class="min-w-0 flex-1 truncate font-semibold">{{ failed ? t('sheets.notFound') : doc?.title || t('sheets.untitled') }}</span>
      <button v-if="doc" type="button" class="se-open" @click="open"><ExternalLink class="h-3.5 w-3.5" />{{ t('sheets.open') }}</button>
    </div>
    <div v-if="tab && book && size.rows" class="se-scroll">
      <table>
        <thead>
          <tr>
            <th />
            <th v-for="c in size.cols" :key="c">{{ colName(c - 1) }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in size.rows" :key="r">
            <th>{{ r }}</th>
            <td v-for="c in size.cols" :key="c" :class="[`al-${cell(r - 1, c - 1).align}`, { b: tab.cells[addr(r - 1, c - 1)]?.b, i: tab.cells[addr(r - 1, c - 1)]?.i, err: cell(r - 1, c - 1).error }]">
              {{ cell(r - 1, c - 1).text }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="data && data.tabs.length > 1" class="se-tabs">
      <button v-for="tb in data.tabs" :key="tb.id" type="button" :class="{ on: tb.id === tabId }" @click="tabId = tb.id">{{ tb.name }}</button>
    </div>
    <p v-if="size.more" class="px-3 pb-2 text-[12px] text-text-3">{{ t('sheets.embedMore') }}</p>
  </div>
</template>

<style scoped>
.sheet-embed {
  margin: 0.6em 0 1em;
  border-radius: 12px;
  box-shadow: 0 0 0 0.5px var(--border);
  overflow: hidden;
  background: var(--bg);
  font-size: 13px;
}
.se-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 0.5px solid var(--border);
}
.se-open {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12.5px;
  color: var(--accent-text);
}
.se-scroll {
  overflow-x: auto;
}
table {
  border-collapse: collapse;
  margin: 0 !important;
  font-variant-numeric: tabular-nums;
}
th,
td {
  min-width: 70px;
  max-width: 260px;
  padding: 3px 8px;
  border: 0.5px solid var(--line-soft);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
th {
  min-width: 0;
  background: var(--grouped);
  font-weight: 500;
  font-size: 11px;
  color: var(--text-3);
  text-align: center;
}
.al-r {
  text-align: right;
}
.al-c {
  text-align: center;
}
.b {
  font-weight: 700;
}
.i {
  font-style: italic;
}
.err {
  color: var(--crit-text);
}
.se-tabs {
  display: flex;
  gap: 2px;
  padding: 4px 8px;
  border-top: 0.5px solid var(--border);
  background: var(--grouped);
}
.se-tabs button {
  padding: 2px 9px;
  border-radius: 6px;
  font-size: 12px;
  color: var(--text-2);
}
.se-tabs button.on {
  background: var(--surface);
  color: var(--text);
  font-weight: 600;
}
</style>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Link, Plus, Upload } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import FileBrowser from '@/components/files/FileBrowser.vue'
import { isGoogleSheet, sheetsApi } from '@/lib/sheetsApi'
import { emptySheet, textOf } from '@/lib/sheet'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's sheets, in its folders; a new one (in the folder shown),
// or one from an Excel file (.xlsx), a CSV or a Google Sheets link.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

// The browser (folders, the sheets in them): new ones go in the folder it's in.
const browser = ref<InstanceType<typeof FileBrowser> | null>(null)
const folder = () => browser.value?.here ?? ''
const open = (id: string) => router.push({ name: 'ws-sheet', params: { ws: ws.value, id } })
async function create() {
  try {
    const s = await sheetsApi.create(ws.value, { title: '', data: emptySheet(t('sheets.tabName', { n: 1 })), folder_id: folder() })
    await open(s.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Import: an Excel file or a CSV becomes a new sheet.
const fileEl = ref<HTMLInputElement | null>(null)
const importing = ref(false)
async function imported(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  ;(e.target as HTMLInputElement).value = ''
  if (!f) return
  importing.value = true
  try {
    const { readCsv, readXlsx } = await import('@/lib/xlsx')
    const name = f.name.replace(/\.(xlsx|csv|tsv|txt)$/i, '')
    const data = /\.xlsx$/i.test(f.name) ? readXlsx(new Uint8Array(await f.arrayBuffer())) : readCsv(await f.text(), name.slice(0, 31) || t('sheets.tabName', { n: 1 }))
    const s = await sheetsApi.create(ws.value, { title: name, data, text: textOf(data), folder_id: folder() })
    toast.success(t('sheets.imported', { name }))
    await open(s.id)
  } catch (err: any) {
    toast.error(t('sheets.importFailed', { error: err?.message ?? String(err) }))
  } finally {
    importing.value = false
  }
}

// A Google Sheets spreadsheet, by its link (shared "Anyone with the link").
const googleAnchor = ref<HTMLElement | null>(null)
const googleOpen = ref(false)
const googleLink = ref('')
const googleEl = ref<HTMLInputElement | null>(null)
async function openGoogle() {
  googleOpen.value = !googleOpen.value
  await nextTick()
  googleEl.value?.focus()
}
async function fromGoogle() {
  const link = googleLink.value.trim()
  if (!isGoogleSheet(link)) {
    toast.error(t('db.sheet.notGoogle'))
    return
  }
  googleOpen.value = false
  importing.value = true
  try {
    const { readXlsx } = await import('@/lib/xlsx')
    const got = await sheetsApi.google(ws.value, link)
    const data = readXlsx(got.bytes)
    const name = got.name || t('sheets.fromGoogle')
    const s = await sheetsApi.create(ws.value, { title: name, data, text: textOf(data), folder_id: folder() })
    googleLink.value = ''
    toast.success(t('sheets.imported', { name }))
    await open(s.id)
  } catch (err: any) {
    toast.error(t('sheets.importFailed', { error: err?.message ?? String(err) }))
  } finally {
    importing.value = false
  }
}

</script>

<template>
  <div class="sheets-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.sheets') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }}</p>
      </div>
      <template v-if="editable">
        <input ref="fileEl" type="file" class="hidden" accept=".xlsx,.csv,.tsv,.txt,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" data-testid="sheet-import-input" @change="imported" />
        <button class="btn btn-ghost" :disabled="importing" :title="t('sheets.importHint')" @click="fileEl?.click()"><Upload class="h-4 w-4" />{{ t('sheets.import') }}</button>
        <button ref="googleAnchor" class="btn btn-ghost" :disabled="importing" data-testid="sheet-google" @click="openGoogle"><Link class="h-4 w-4" />{{ t('sheets.google') }}</button>
        <button class="btn-primary" data-testid="new-sheet" @click="create"><Plus class="h-4 w-4" />{{ t('sheets.new') }}</button>
      </template>
    </header>

    <FileBrowser ref="browser" :ws="ws" :kinds="['sheet']" :editable="editable" :root-name="t('modules.sheets')">
      <template #empty>{{ t('sheets.noneHint') }}</template>
    </FileBrowser>

    <Popover v-if="googleOpen" :anchor="googleAnchor" placement="bottom-end" :width="340" @close="googleOpen = false">
      <form class="space-y-2 p-2.5" @submit.prevent="fromGoogle">
        <input ref="googleEl" v-model="googleLink" class="field !py-1.5 text-[13.5px]" :placeholder="t('db.sheet.googlePlaceholder')" :aria-label="t('sheets.google')" data-testid="sheet-google-link" />
        <p class="text-[12px] leading-snug text-text-3">{{ t('sheets.googleHint') }}</p>
        <button type="submit" class="btn-primary btn-sm w-full justify-center" :disabled="!googleLink.trim()">{{ t('sheets.googleBring') }}</button>
      </form>
    </Popover>
  </div>
</template>

<style scoped>
.sheets-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
</style>

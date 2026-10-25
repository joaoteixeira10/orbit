<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Link, Plus, Sheet, Trash2, Upload, MoreHorizontal } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { isGoogleSheet, sheetsApi, type SheetSummary } from '@/lib/sheetsApi'
import { emptySheet, textOf } from '@/lib/sheet'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A workspace's sheets, newest change first; a new one, or one from an
// Excel file (.xlsx) or a CSV.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const list = ref<SheetSummary[]>([])
const loaded = ref(false)
async function load() {
  try {
    list.value = await sheetsApi.list(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, () => {
  loaded.value = false
  void load()
}, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && e.what === 'sheets') void load()
})
onUnmounted(off)

const open = (id: string) => router.push({ name: 'ws-sheet', params: { ws: ws.value, id } })
async function create() {
  try {
    const s = await sheetsApi.create(ws.value, { title: '', data: emptySheet(t('sheets.tabName', { n: 1 })) })
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
    const s = await sheetsApi.create(ws.value, { title: name, data, text: textOf(data) })
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
    const s = await sheetsApi.create(ws.value, { title: name, data, text: textOf(data) })
    googleLink.value = ''
    toast.success(t('sheets.imported', { name }))
    await open(s.id)
  } catch (err: any) {
    toast.error(t('sheets.importFailed', { error: err?.message ?? String(err) }))
  } finally {
    importing.value = false
  }
}

const menu = ref<{ s: SheetSummary; el: HTMLElement } | null>(null)
const removing = ref<SheetSummary | null>(null)
async function remove() {
  const s = removing.value
  removing.value = null
  if (!s) return
  try {
    await sheetsApi.remove(s.id)
    list.value = list.value.filter((x) => x.id !== s.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="sheets-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('modules.sheets') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }} · {{ list.length }}</p>
      </div>
      <template v-if="editable">
        <input ref="fileEl" type="file" class="hidden" accept=".xlsx,.csv,.tsv,.txt,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" data-testid="sheet-import-input" @change="imported" />
        <button class="btn btn-ghost" :disabled="importing" :title="t('sheets.importHint')" @click="fileEl?.click()"><Upload class="h-4 w-4" />{{ t('sheets.import') }}</button>
        <button ref="googleAnchor" class="btn btn-ghost" :disabled="importing" data-testid="sheet-google" @click="openGoogle"><Link class="h-4 w-4" />{{ t('sheets.google') }}</button>
        <button class="btn-primary" data-testid="new-sheet" @click="create"><Plus class="h-4 w-4" />{{ t('sheets.new') }}</button>
      </template>
    </header>

    <div v-if="loaded && !list.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #34c759"><Sheet class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('sheets.none') }}</p>
      <p class="mt-1 max-w-[360px] text-[13px] text-text-2">{{ t('sheets.noneHint') }}</p>
      <button v-if="editable" class="btn-primary mt-5" @click="create"><Plus class="h-4 w-4" />{{ t('sheets.new') }}</button>
    </div>

    <div class="group">
      <div v-for="s in list" :key="s.id" class="group-row cursor-pointer" role="link" tabindex="0" data-testid="sheet-row" @click="open(s.id)" @keydown.enter="open(s.id)">
        <span class="tile h-8 w-8" style="--tile: #34c759"><Sheet class="h-4 w-4" /></span>
        <div class="min-w-0 flex-1">
          <div class="truncate text-[14.5px] font-medium">{{ s.title || t('sheets.untitled') }}</div>
          <div class="text-[12px] text-text-3">{{ t('sheets.edited', { when: ago(s.updated_at, locale()) }) }}</div>
        </div>
        <button v-if="editable" class="icon-btn h-7 w-7 shrink-0" :aria-label="t('sheets.more')" @click.stop="menu = { s, el: $event.currentTarget as HTMLElement }"><MoreHorizontal class="h-4 w-4" /></button>
      </div>
    </div>

    <Popover v-if="googleOpen" :anchor="googleAnchor" placement="bottom-end" :width="340" @close="googleOpen = false">
      <form class="space-y-2 p-2.5" @submit.prevent="fromGoogle">
        <input ref="googleEl" v-model="googleLink" class="field !py-1.5 text-[13.5px]" :placeholder="t('db.sheet.googlePlaceholder')" :aria-label="t('sheets.google')" data-testid="sheet-google-link" />
        <p class="text-[12px] leading-snug text-text-3">{{ t('sheets.googleHint') }}</p>
        <button type="submit" class="btn-primary btn-sm w-full justify-center" :disabled="!googleLink.trim()">{{ t('sheets.googleBring') }}</button>
      </form>
    </Popover>
    <Popover v-if="menu" :anchor="menu.el" placement="bottom-end" :width="200" @close="menu = null">
      <button class="menu-item danger" @click="(removing = menu.s), (menu = null)"><Trash2 class="h-4 w-4" />{{ t('sheets.delete') }}</button>
    </Popover>
    <Confirm :open="!!removing" :title="t('sheets.deleteTitle', { name: removing?.title || t('sheets.untitled') })" :message="t('sheets.deleteBody')" :confirm-label="t('sheets.delete')" danger @confirm="remove" @close="removing = null" />
  </div>
</template>

<style scoped>
.sheets-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
</style>

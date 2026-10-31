<script setup lang="ts">
import PresenceAvatars from '@/components/ui/PresenceAvatars.vue'
import { usePresence } from '@/lib/presence'
import { computed, onBeforeUnmount, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router'
import { Check, ChevronLeft, Download, Link, LoaderCircle, MoreHorizontal, Trash2 } from 'lucide-vue-next'
import SheetEditor from '@/components/sheets/SheetEditor.vue'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { sheetsApi, type SheetDoc } from '@/lib/sheetsApi'
import { Book, addr, readData, show, textOf, type SheetData } from '@/lib/sheet'
import { dayOf } from '@/lib/formula'
import { locale, todayStr } from '@/lib/dates'
import { fileName } from '@/lib/markdown'
import { ApiError, CLIENT_ID } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// One sheet, as big as the window: its name, a menu (download as .xlsx or
// .csv, delete), the editor. It saves itself a moment after each change; a
// save over someone else's newer one is refused, and theirs is shown (with a
// word saying so). Their changes show here when nothing of ours is waiting.
// Embedded (ws and id given), it is the big editor over a page: Done instead
// of the way back.
const props = defineProps<{ ws?: string; id?: string; embedded?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => props.ws ?? String(route.params.ws))
const id = computed(() => props.id ?? String(route.params.id))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const sheet = ref<SheetDoc | null>(null)
const data = ref<SheetData | null>(null)
const title = ref('')
const missing = ref(false)
async function load() {
  try {
    const s = await sheetsApi.get(id.value)
    sheet.value = s
    title.value = s.title
    data.value = readData(s.data, t('sheets.tabName', { n: 1 }))
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 404) missing.value = true
    else toast.error(e.message)
  }
}
watch(id, load, { immediate: true })

// ---- Saving ----
const state = ref<'idle' | 'waiting' | 'saving' | 'saved' | 'error'>('idle')
let timer: ReturnType<typeof setTimeout> | undefined
let running: Promise<void> = Promise.resolve()
function changed() {
  state.value = 'waiting'
  clearTimeout(timer)
  timer = setTimeout(() => void save(), 900)
}
function save(extra: { title?: string } = {}): Promise<void> {
  clearTimeout(timer)
  const run = running.then(async () => {
    const s = sheet.value
    if (!s || !data.value || !editable.value) return
    const withData = state.value === 'waiting'
    if (!withData && extra.title === undefined) return
    state.value = 'saving'
    try {
      const body = withData ? { data: data.value, text: textOf(data.value) } : {}
      const saved = await sheetsApi.save(s.id, { ...body, ...extra, version: s.version })
      sheet.value = { ...s, ...saved, data: undefined }
      if (state.value === 'saving') state.value = 'saved'
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 409 && e.body?.id) {
        // Saved elsewhere meanwhile: theirs wins; show it.
        const theirs = e.body as SheetDoc
        sheet.value = theirs
        data.value = readData(theirs.data, t('sheets.tabName', { n: 1 }))
        title.value = theirs.title
        state.value = 'idle'
        toast.info(t('sheets.changedElsewhere'))
        return
      }
      state.value = 'error'
      toast.error(e.message)
    }
  })
  running = run
  return run
}
async function flush() {
  if (state.value === 'waiting') await save()
  else await running
}
if (!props.embedded) onBeforeRouteLeave(() => flush())
async function done() {
  await flush()
  emit('close')
}
defineExpose({ flush })
onBeforeUnmount(() => void flush())
function onUnload(e: BeforeUnloadEvent) {
  if (state.value !== 'waiting' && state.value !== 'saving') return
  void flush()
  e.preventDefault()
}
window.addEventListener('beforeunload', onUnload)
onUnmounted(() => window.removeEventListener('beforeunload', onUnload))

function saveTitle() {
  const v = title.value.trim()
  if (!sheet.value || v === sheet.value.title) return
  void save({ title: v })
}

// Someone else's save: take it when nothing of ours is waiting.
const off = onLiveEvent('ws', async (e) => {
  const s = sheet.value
  if (!s || e.origin === CLIENT_ID || e.what !== 'sheets' || String(e.ws) !== ws.value || state.value === 'waiting' || state.value === 'saving') return
  try {
    const fresh = await sheetsApi.get(s.id)
    if (fresh.version > s.version) {
      sheet.value = fresh
      data.value = readData(fresh.data, t('sheets.tabName', { n: 1 }))
      title.value = fresh.title
    }
  } catch {
    /* gone: the next load says so */
  }
})
onUnmounted(off)

// ---- Downloads ----
const menuEl = ref<HTMLElement | null>(null)
const menu = ref(false)
const editor = ref<InstanceType<typeof SheetEditor> | null>(null)
async function download(kind: 'xlsx' | 'csv') {
  menu.value = false
  if (!data.value) return
  const { csvOf, writeXlsx } = await import('@/lib/xlsx')
  const book = new Book(data.value, dayOf(todayStr()), locale().startsWith('pt'))
  const name = fileName(title.value || t('sheets.untitled'))
  let blob: Blob
  if (kind === 'xlsx') {
    blob = new Blob([writeXlsx(data.value, book) as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  } else {
    const tab = data.value.tabs.find((x) => x.id === editor.value?.tabId) ?? data.value.tabs[0]
    const words = { yes: t('sheets.true'), no: t('sheets.false') }
    blob = new Blob([csvOf(tab, (r, c) => show(book.value(tab.id, r, c), tab.cells[addr(r, c)], locale(), words).text)], { type: 'text/csv;charset=utf-8' })
  }
  const url = URL.createObjectURL(blob)
  const a = Object.assign(document.createElement('a'), { href: url, download: `${name}.${kind}` })
  document.body.append(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
// Its link: on its own line in a page, the sheet shows there.
async function copyLink() {
  menu.value = false
  await navigator.clipboard.writeText(`${location.origin}/w/${ws.value}/s/${id.value}`)
  toast.success(t('sheets.linkCopied'))
}
const confirmDelete = ref(false)
async function remove() {
  if (!sheet.value) return
  try {
    clearTimeout(timer)
    state.value = 'idle'
    await sheetsApi.remove(sheet.value.id)
    if (props.embedded) emit('close')
    else void router.push({ name: 'ws-sheets', params: { ws: ws.value } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

const fill = ref<HTMLElement | null>(null)
useFillHeight(props.embedded ? ref(null) : fill, 420)
const status = computed(() =>
  state.value === 'waiting' || state.value === 'saving' ? t('common.saving') : state.value === 'saved' ? t('common.saved') : state.value === 'error' ? t('editor.saveFailed') : '',
)

// Who else is here now (and whether I'm changing it).
const here = usePresence('sheet', id, editable)
</script>

<template>
  <div class="sheet-view" :class="{ embedded }">
    <header class="mb-3 flex items-center gap-2">
      <RouterLink v-if="!embedded" :to="{ name: 'ws-sheets', params: { ws } }" class="btn-plain -ml-2 shrink-0"><ChevronLeft class="h-4 w-4" />{{ t('modules.sheets') }}</RouterLink>
      <input
        v-model="title"
        class="min-w-0 flex-1 rounded-lg bg-transparent px-2 py-1 text-[20px] font-bold outline-none hover:bg-fill-2 focus:bg-fill-2"
        :placeholder="t('sheets.untitled')"
        :readonly="!editable"
        maxlength="300"
        data-testid="sheet-title"
        @keydown.enter="($event.target as HTMLInputElement).blur()"
        @blur="saveTitle"
      />
      <span v-if="status" class="flex shrink-0 items-center gap-1 text-[12px] text-text-3" data-testid="sheet-state">
        <LoaderCircle v-if="state === 'waiting' || state === 'saving'" class="h-3.5 w-3.5 animate-spin" />
        <Check v-else-if="state === 'saved'" class="h-3.5 w-3.5" />
        {{ status }}
      </span>
      <PresenceAvatars :people="here.others.value" class="shrink-0" />
      <button ref="menuEl" type="button" class="icon-btn shrink-0" :aria-label="t('sheets.more')" data-testid="sheet-menu" @click="menu = !menu"><MoreHorizontal class="h-[18px] w-[18px]" /></button>
      <button v-if="embedded" type="button" class="btn-primary shrink-0" data-testid="embed-done" @click="done">{{ t('common.done') }}</button>
    </header>
    <p v-if="missing" class="py-24 text-center text-[14px] text-text-3">{{ t('sheets.notFound') }}</p>
    <div v-else ref="fill" class="sheet-frame">
      <SheetEditor v-if="data" ref="editor" v-model="data" :editable="editable" @change="changed" />
    </div>

    <Popover v-if="menu" :anchor="menuEl" :width="230" placement="bottom-end" @close="menu = false">
      <div class="p-1.5">
        <button type="button" class="menu-item w-full" @click="copyLink"><Link class="h-4 w-4 text-text-2" />{{ t('sheets.copyLink') }}</button>
        <button type="button" class="menu-item w-full" data-testid="sheet-xlsx" @click="download('xlsx')"><Download class="h-4 w-4 text-text-2" />{{ t('sheets.downloadXlsx') }}</button>
        <button type="button" class="menu-item w-full" data-testid="sheet-csv" @click="download('csv')"><Download class="h-4 w-4 text-text-2" />{{ t('sheets.downloadCsv') }}</button>
        <p class="px-2.5 pb-1 pt-1 text-[11.5px] leading-snug text-text-3">{{ t('sheets.toGoogle') }}</p>
        <template v-if="editable">
          <div class="menu-sep" />
          <button type="button" class="menu-item danger w-full" @click="(menu = false), (confirmDelete = true)"><Trash2 class="h-4 w-4" />{{ t('sheets.delete') }}</button>
        </template>
      </div>
    </Popover>
    <Confirm :open="confirmDelete" :title="t('sheets.deleteTitle', { name: title || t('sheets.untitled') })" :message="t('sheets.deleteBody')" :confirm-label="t('sheets.delete')" danger @confirm="remove" @close="confirmDelete = false" />
  </div>
</template>

<style scoped>
.sheet-view {
  padding: 16px clamp(12px, 2vw, 24px) 0;
}
.sheet-frame {
  display: flex;
  flex-direction: column;
  min-height: 420px;
}
.sheet-view.embedded {
  display: flex;
  height: 100%;
  flex-direction: column;
  padding: 12px 16px 16px;
}
.embedded .sheet-frame {
  flex: 1;
  min-height: 0;
}
</style>

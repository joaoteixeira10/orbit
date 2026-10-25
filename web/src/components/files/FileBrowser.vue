<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  Brush, ChevronRight, Copy, Download, ExternalLink, Folder as FolderIcon, FolderInput, FolderPlus, Info, LayoutGrid, Link, List, MoreHorizontal, Pencil, Sheet,
  Trash2, Workflow, X,
} from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Confirm from '@/components/ui/Confirm.vue'
import Overlay from '@/components/ui/Overlay.vue'
import { filesApi, inside, pathTo, sizeText, tree, type FileItem, type FileKind, type Folder } from '@/lib/filesApi'
import { download, duplicate, linkOf, remove, rename, routeOf } from '@/lib/fileOps'
import { svgUrl } from '@/lib/drawio'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { onLiveEvent } from '@/lib/live'
import { COLORS } from '@/lib/icons'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// A workspace's files in folders, as in a file manager: the folders here
// and the files here (of the kinds asked for: a module shows its own; Files
// shows them all), as tiles or a list; a path back up; folders inside
// folders. Each has a menu (also on a right click): open, rename, copy,
// move, download, its link, its details, delete. Drag one onto a folder or
// onto the path to move it. Where you are is in the address (?folder=).
const props = defineProps<{ ws: string; kinds: FileKind[]; editable: boolean; rootName: string }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const toast = useToastStore()

const folders = ref<Folder[]>([])
const files = ref<FileItem[]>([])
const loaded = ref(false)
async function load() {
  try {
    const [fs, xs] = await Promise.all([filesApi.folders(props.ws), filesApi.files(props.ws, props.kinds)])
    folders.value = fs
    files.value = xs
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(
  () => [props.ws, props.kinds.join()],
  () => {
    loaded.value = false
    void load()
    if (!wss.members[props.ws]) void wss.loadMembers(props.ws).catch(() => undefined)
  },
  { immediate: true },
)
let reloadTimer: ReturnType<typeof setTimeout> | undefined
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) !== props.ws || !['files', 'sheets', 'diagrams', 'drawings'].includes(String(e.what))) return
  clearTimeout(reloadTimer)
  reloadTimer = setTimeout(() => void load(), 150)
})
onUnmounted(() => {
  off()
  clearTimeout(reloadTimer)
})

// ---- Where we are ----
const known = computed(() => new Set(folders.value.map((f) => f.id)))
const here = computed(() => {
  const q = typeof route.query.folder === 'string' ? route.query.folder : ''
  return known.value.has(q) ? q : ''
})
const path = computed(() => pathTo(folders.value, here.value))
const go = (id: string) => router.push({ query: { ...route.query, folder: id || undefined } })
// A file in a folder that's gone shows at the top.
const folderOf = (f: { folder_id: string }) => (known.value.has(f.folder_id) ? f.folder_id : '')
const subfolders = computed(() => folders.value.filter((f) => (known.value.has(f.parent_id) ? f.parent_id : '') === here.value).sort((a, b) => a.name.localeCompare(b.name)))
const sort = ref<'modified' | 'name'>((localStorage.getItem('orbit_files_sort') as 'name') === 'name' ? 'name' : 'modified')
watch(sort, (v) => localStorage.setItem('orbit_files_sort', v))
const items = computed(() =>
  files.value
    .filter((f) => folderOf(f) === here.value)
    .sort((a, b) => (sort.value === 'name' ? (a.title || '').localeCompare(b.title || '') : b.updated_at.localeCompare(a.updated_at))),
)
const countIn = (id: string) => folders.value.filter((f) => f.parent_id === id).length + files.value.filter((f) => f.folder_id === id).length

const view = ref<'grid' | 'list'>(localStorage.getItem('orbit_files_view') === 'list' ? 'list' : 'grid')
watch(view, (v) => localStorage.setItem('orbit_files_view', v))

// ---- Kinds ----
const KIND_ICON: Record<FileKind, Component> = { sheet: Sheet, diagram: Workflow, board: Brush }
const KIND_COLOR: Record<FileKind, string> = { sheet: '#34c759', diagram: '#ff9500', board: '#af52de' }
const TESTID: Record<FileKind, string> = { sheet: 'sheet-row', diagram: 'diagram-card', board: 'whiteboard-card' }
const kindName = (k: FileKind) => t(`drive.kind.${k}`)
const untitled = (k: FileKind) => t(`drive.untitled.${k}`)
const nameOf = (f: FileItem) => f.title || untitled(f.kind)
const folderColor = (f: Folder) => f.color || '#5ac8fa'
const people = computed(() => new Map((wss.members[props.ws] ?? []).map((m) => [m.user_id, m.display_name || m.username])))
const open = (f: FileItem) => router.push(routeOf(props.ws, f))

// ---- Menus ----
type Target = { file: FileItem } | { folder: Folder }
const menu = ref<{ target: Target; el: HTMLElement; mode: 'menu' | 'rename' } | null>(null)
const renameText = ref('')
const renameEl = ref<HTMLInputElement | null>(null)
function openMenu(target: Target, e: MouseEvent) {
  menu.value = { target, el: (e.currentTarget as HTMLElement) ?? (e.target as HTMLElement), mode: 'menu' }
}
async function startRename() {
  const m = menu.value
  if (!m) return
  renameText.value = 'file' in m.target ? m.target.file.title : m.target.folder.name
  m.mode = 'rename'
  await nextTick()
  renameEl.value?.select()
}
async function doRename() {
  const m = menu.value
  menu.value = null
  if (!m) return
  const name = renameText.value.trim()
  try {
    if ('file' in m.target) {
      if (name !== m.target.file.title) await rename(m.target.file, name)
    } else if (name && name !== m.target.folder.name) await filesApi.updateFolder(m.target.folder.id, { name })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setColor(f: Folder, color: string) {
  try {
    await filesApi.updateFolder(f.id, { color })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
// colorMenu recolours the folder whose menu is open.
function colorMenu(c: string) {
  const m = menu.value
  if (m && 'folder' in m.target) void setColor(m.target.folder, c)
}
async function copyOf(f: FileItem) {
  menu.value = null
  try {
    await duplicate(props.ws, f, t('drive.copyWord'))
    await load()
    toast.success(t('drive.copied', { name: nameOf(f) }))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function downloadOf(f: FileItem) {
  menu.value = null
  try {
    await download(f, untitled(f.kind))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function linkTo(f: FileItem) {
  menu.value = null
  await navigator.clipboard.writeText(linkOf(props.ws, f))
  toast.success(t('drive.linkCopied'))
}

// ---- New folder ----
const newOpen = ref(false)
const newEl = ref<HTMLElement | null>(null)
const newName = ref('')
const newInput = ref<HTMLInputElement | null>(null)
async function startFolder() {
  newName.value = ''
  newOpen.value = true
  await nextTick()
  newInput.value?.focus()
}
async function makeFolder() {
  const name = newName.value.trim()
  newOpen.value = false
  if (!name) return
  try {
    await filesApi.createFolder(props.ws, { name, parent_id: here.value })
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Moving ----
const moving = ref<Target | null>(null)
const moveTo = ref('')
const choices = computed(() => {
  const m = moving.value
  const banned = m && 'folder' in m ? inside(folders.value, m.folder.id) : new Set<string>()
  return tree(folders.value).filter((x) => !banned.has(x.folder.id))
})
function startMove(t: Target) {
  menu.value = null
  moving.value = t
  moveTo.value = 'file' in t ? folderOf(t.file) : t.folder.parent_id
}
async function moveInto(target: Target, folderId: string) {
  const what = 'file' in target ? { kind: target.file.kind, id: target.file.id } : { kind: 'folder' as const, id: target.folder.id }
  if ('folder' in target && inside(folders.value, target.folder.id).has(folderId)) return
  try {
    await filesApi.move(props.ws, folderId, [what])
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function doMove() {
  const m = moving.value
  moving.value = null
  if (m) await moveInto(m, moveTo.value)
}

// Drag a file or folder onto a folder, or onto the path.
const dragged = ref<Target | null>(null)
const over = ref<string | null>(null)
function dragStart(t: Target, e: DragEvent) {
  if (!props.editable) return
  dragged.value = t
  e.dataTransfer?.setData('text/plain', 'file' in t ? nameOf(t.file) : t.folder.name)
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move'
}
function dragOver(id: string, e: DragEvent) {
  const d = dragged.value
  if (!d || ('folder' in d && inside(folders.value, d.folder.id).has(id))) return
  e.preventDefault()
  over.value = id
}
async function drop(id: string) {
  const d = dragged.value
  dragged.value = null
  over.value = null
  if (d) await moveInto(d, id)
}

// ---- Deleting ----
const removing = ref<Target | null>(null)
const removeTitle = computed(() => {
  const r = removing.value
  if (!r) return ''
  return 'file' in r ? t('drive.deleteTitle', { name: nameOf(r.file) }) : t('drive.deleteFolderTitle', { name: r.folder.name })
})
const removeBody = computed(() => {
  const r = removing.value
  if (!r) return ''
  if ('file' in r) return t(`drive.deleteBody.${r.file.kind}`)
  const up = folders.value.find((f) => f.id === r.folder.parent_id)?.name ?? props.rootName
  return t('drive.deleteFolderBody', { where: up })
})
async function doRemove() {
  const r = removing.value
  removing.value = null
  if (!r) return
  try {
    if ('file' in r) await remove(r.file)
    else {
      await filesApi.removeFolder(r.folder.id)
      if (inside(folders.value, r.folder.id).has(here.value)) void go(r.folder.parent_id)
    }
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Details ----
const wide = ref(window.matchMedia('(min-width: 1180px)').matches)
const mq = window.matchMedia('(min-width: 1180px)')
const onMq = () => (wide.value = mq.matches)
onMounted(() => mq.addEventListener('change', onMq))
onUnmounted(() => mq.removeEventListener('change', onMq))
const infoOpen = ref(localStorage.getItem('orbit_files_info') === '1')
watch(infoOpen, (v) => localStorage.setItem('orbit_files_info', v ? '1' : '0'))
const infoOf = ref<Target | null>(null)
function showInfo(t: Target) {
  menu.value = null
  infoOf.value = t
  infoOpen.value = true
}
// What the details show: the one chosen, else the folder we're in (else the top).
const infoFile = computed<FileItem | null>(() => {
  const x = infoOf.value
  return x && 'file' in x ? (files.value.find((y) => y.kind === x.file.kind && y.id === x.file.id) ?? null) : null
})
const infoFolder = computed<Folder | null>(() => {
  if (infoFile.value) return null
  const x = infoOf.value
  const id = x && 'folder' in x ? x.folder.id : here.value
  return folders.value.find((y) => y.id === id) ?? null
})
const totals = (id: string) => {
  const all = inside(folders.value, id)
  const fs = files.value.filter((f) => all.has(f.folder_id))
  return { folders: all.size - 1, files: fs.length, size: fs.reduce((s, f) => s + f.size, 0) }
}
const topTotals = computed(() => ({ folders: folders.value.length, files: files.value.length, size: files.value.reduce((s, f) => s + f.size, 0) }))
const when = (iso: string) => (iso ? new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(iso)) : '—')
const where = (folderId: string) => [props.rootName, ...pathTo(folders.value, folderId).map((f) => f.name)].join(' › ')
const loc = locale()

defineExpose({ here, reload: load })
</script>

<template>
  <div class="fb" :class="{ 'with-info': infoOpen && wide }">
    <div class="fb-main">
      <!-- The path, and the tools -->
      <div class="fb-bar">
        <nav class="crumbs" :aria-label="t('drive.path')">
          <button
            type="button"
            class="crumb"
            :class="{ over: over === '' }"
            data-testid="crumb"
            @click="go('')"
            @dragover="dragOver('', $event)"
            @dragleave="over = null"
            @drop.prevent="drop('')"
          >
            {{ rootName }}
          </button>
          <template v-for="f in path" :key="f.id">
            <ChevronRight class="h-3.5 w-3.5 shrink-0 text-text-3" />
            <button type="button" class="crumb" :class="{ over: over === f.id }" data-testid="crumb" @click="go(f.id)" @dragover="dragOver(f.id, $event)" @dragleave="over = null" @drop.prevent="drop(f.id)">
              {{ f.name }}
            </button>
          </template>
        </nav>
        <div class="flex items-center gap-1">
          <button v-if="editable" ref="newEl" type="button" class="btn btn-sm" data-testid="new-folder" @click="startFolder"><FolderPlus class="h-3.5 w-3.5" />{{ t('drive.newFolder') }}</button>
          <select v-model="sort" class="input !w-auto !py-1 text-[12.5px]" :aria-label="t('drive.sortBy')">
            <option value="modified">{{ t('drive.byModified') }}</option>
            <option value="name">{{ t('drive.byName') }}</option>
          </select>
          <div class="seg" role="group" :aria-label="t('drive.view')">
            <button type="button" :class="{ on: view === 'grid' }" :aria-pressed="view === 'grid'" :title="t('drive.grid')" data-testid="view-grid" @click="view = 'grid'"><LayoutGrid class="h-4 w-4" /></button>
            <button type="button" :class="{ on: view === 'list' }" :aria-pressed="view === 'list'" :title="t('drive.list')" data-testid="view-list" @click="view = 'list'"><List class="h-4 w-4" /></button>
          </div>
          <button type="button" class="icon-btn h-8 w-8" :class="{ 'text-accent': infoOpen }" :aria-pressed="infoOpen" :title="t('drive.details')" :aria-label="t('drive.details')" data-testid="files-info" @click="(infoOpen = !infoOpen), (infoOf = null)"><Info class="h-4 w-4" /></button>
        </div>
      </div>

      <p v-if="loaded && !subfolders.length && !items.length" class="py-14 text-center text-[13.5px] text-text-3" data-testid="files-empty">
        <slot name="empty">{{ t('drive.empty') }}</slot>
      </p>

      <!-- Tiles -->
      <template v-if="view === 'grid'">
        <section v-if="subfolders.length" class="mb-6">
          <h2 class="cap mb-2">{{ t('drive.folders') }}</h2>
          <div class="folders">
            <div
              v-for="f in subfolders"
              :key="f.id"
              class="folder"
              :class="{ over: over === f.id }"
              role="link"
              tabindex="0"
              :draggable="editable"
              data-testid="folder-tile"
              @click="go(f.id)"
              @keydown.enter="go(f.id)"
              @contextmenu.prevent="openMenu({ folder: f }, $event)"
              @dragstart="dragStart({ folder: f }, $event)"
              @dragover="dragOver(f.id, $event)"
              @dragleave="over = null"
              @drop.prevent="drop(f.id)"
            >
              <FolderIcon class="h-9 w-9 shrink-0" :style="{ color: folderColor(f), fill: `color-mix(in srgb, ${folderColor(f)} 30%, transparent)` }" :stroke-width="1.6" />
              <div class="min-w-0 flex-1">
                <div class="truncate text-[13.5px] font-medium">{{ f.name }}</div>
                <div class="text-[11.5px] text-text-3">{{ t('drive.items', { n: countIn(f.id) }, countIn(f.id)) }}</div>
              </div>
              <button type="button" class="icon-btn h-7 w-7 shrink-0" :aria-label="t('drive.more')" data-testid="folder-more" @click.stop="openMenu({ folder: f }, $event)"><MoreHorizontal class="h-4 w-4" /></button>
            </div>
          </div>
        </section>
        <section v-if="items.length">
          <h2 v-if="subfolders.length" class="cap mb-2">{{ t('drive.files') }}</h2>
          <div class="cards">
            <div
              v-for="f in items"
              :key="f.kind + f.id"
              class="card-f"
              role="link"
              tabindex="0"
              :draggable="editable"
              :data-testid="TESTID[f.kind]"
              @click="open(f)"
              @keydown.enter="open(f)"
              @contextmenu.prevent="openMenu({ file: f }, $event)"
              @dragstart="dragStart({ file: f }, $event)"
            >
              <div class="pic">
                <img v-if="f.preview" :src="svgUrl(f.preview)" alt="" draggable="false" loading="lazy" />
                <span v-else class="tile h-12 w-12" :style="{ '--tile': KIND_COLOR[f.kind] }"><component :is="KIND_ICON[f.kind]" class="h-6 w-6" /></span>
              </div>
              <div class="flex items-center gap-2 px-3 py-2.5">
                <component :is="KIND_ICON[f.kind]" class="h-4 w-4 shrink-0" :style="{ color: KIND_COLOR[f.kind] }" />
                <div class="min-w-0 flex-1">
                  <div class="truncate text-[14px] font-medium">{{ nameOf(f) }}</div>
                  <div class="text-[12px] text-text-3">{{ t('drive.edited', { when: ago(f.updated_at, loc) }) }}</div>
                </div>
                <button type="button" class="icon-btn h-7 w-7 shrink-0" :aria-label="t('drive.more')" data-testid="file-more" @click.stop="openMenu({ file: f }, $event)"><MoreHorizontal class="h-4 w-4" /></button>
              </div>
            </div>
          </div>
        </section>
      </template>

      <!-- A list -->
      <table v-else-if="subfolders.length || items.length" class="fb-list">
        <thead>
          <tr>
            <th>{{ t('drive.name') }}</th>
            <th class="hide-sm">{{ t('drive.kindCol') }}</th>
            <th class="hide-sm">{{ t('drive.modified') }}</th>
            <th class="hide-sm text-right">{{ t('drive.size') }}</th>
            <th class="w-10" />
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="f in subfolders"
            :key="f.id"
            :class="{ over: over === f.id }"
            :draggable="editable"
            data-testid="folder-tile"
            @click="go(f.id)"
            @contextmenu.prevent="openMenu({ folder: f }, $event)"
            @dragstart="dragStart({ folder: f }, $event)"
            @dragover="dragOver(f.id, $event)"
            @dragleave="over = null"
            @drop.prevent="drop(f.id)"
          >
            <td>
              <span class="flex items-center gap-2.5"><FolderIcon class="h-5 w-5 shrink-0" :style="{ color: folderColor(f), fill: `color-mix(in srgb, ${folderColor(f)} 30%, transparent)` }" /><span class="truncate font-medium">{{ f.name }}</span></span>
            </td>
            <td class="hide-sm text-text-3">{{ t('drive.kind.folder') }}</td>
            <td class="hide-sm" />
            <td class="hide-sm text-right text-text-3">{{ t('drive.items', { n: countIn(f.id) }, countIn(f.id)) }}</td>
            <td><button type="button" class="icon-btn h-7 w-7" :aria-label="t('drive.more')" @click.stop="openMenu({ folder: f }, $event)"><MoreHorizontal class="h-4 w-4" /></button></td>
          </tr>
          <tr
            v-for="f in items"
            :key="f.kind + f.id"
            :draggable="editable"
            :data-testid="TESTID[f.kind]"
            @click="open(f)"
            @contextmenu.prevent="openMenu({ file: f }, $event)"
            @dragstart="dragStart({ file: f }, $event)"
          >
            <td>
              <span class="flex items-center gap-2.5"><component :is="KIND_ICON[f.kind]" class="h-5 w-5 shrink-0" :style="{ color: KIND_COLOR[f.kind] }" /><span class="truncate font-medium">{{ nameOf(f) }}</span></span>
            </td>
            <td class="hide-sm text-text-3">{{ kindName(f.kind) }}</td>
            <td class="hide-sm text-text-3">{{ ago(f.updated_at, loc) }}</td>
            <td class="hide-sm text-right tabular-nums text-text-3">{{ sizeText(f.size, loc) }}</td>
            <td><button type="button" class="icon-btn h-7 w-7" :aria-label="t('drive.more')" data-testid="file-more" @click.stop="openMenu({ file: f }, $event)"><MoreHorizontal class="h-4 w-4" /></button></td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Details -->
    <component :is="wide ? 'aside' : Overlay" v-if="infoOpen" v-bind="wide ? { class: 'fb-info' } : { open: true, mode: 'side', title: t('drive.details') }" data-testid="files-details" @close="infoOpen = false">
      <div class="info-body">
        <div class="mb-3 flex items-start gap-2">
          <div class="min-w-0 flex-1">
            <template v-if="infoFile">
              <div class="pic info-pic">
                <img v-if="infoFile.preview" :src="svgUrl(infoFile.preview)" alt="" />
                <span v-else class="tile h-14 w-14" :style="{ '--tile': KIND_COLOR[infoFile.kind] }"><component :is="KIND_ICON[infoFile.kind]" class="h-7 w-7" /></span>
              </div>
              <div class="mt-3 break-words text-[15px] font-semibold">{{ nameOf(infoFile) }}</div>
            </template>
            <template v-else>
              <FolderIcon class="h-12 w-12" :style="infoFolder ? { color: folderColor(infoFolder), fill: `color-mix(in srgb, ${folderColor(infoFolder)} 30%, transparent)` } : { color: '#5ac8fa' }" :stroke-width="1.4" />
              <div class="mt-2 break-words text-[15px] font-semibold">{{ infoFolder ? infoFolder.name : rootName }}</div>
            </template>
          </div>
          <button v-if="wide" type="button" class="icon-btn h-7 w-7" :aria-label="t('common.close')" @click="infoOpen = false"><X class="h-4 w-4" /></button>
        </div>
        <dl class="info-rows">
          <template v-if="infoFile">
            <dt>{{ t('drive.kindCol') }}</dt><dd>{{ kindName(infoFile.kind) }}</dd>
            <dt>{{ t('drive.size') }}</dt><dd>{{ sizeText(infoFile.size, loc) }}</dd>
            <dt>{{ t('drive.where') }}</dt><dd>{{ where(folderOf(infoFile)) }}</dd>
            <dt>{{ t('drive.created') }}</dt><dd>{{ when(infoFile.created_at) }}<template v-if="people.get(infoFile.created_by)"> · {{ people.get(infoFile.created_by) }}</template></dd>
            <dt>{{ t('drive.modified') }}</dt><dd>{{ when(infoFile.updated_at) }}<template v-if="people.get(infoFile.updated_by)"> · {{ people.get(infoFile.updated_by) }}</template></dd>
          </template>
          <template v-else-if="infoFolder">
            <dt>{{ t('drive.kindCol') }}</dt><dd>{{ t('drive.kind.folder') }}</dd>
            <dt>{{ t('drive.holds') }}</dt><dd>{{ t('drive.holdsText', { folders: totals(infoFolder.id).folders, files: totals(infoFolder.id).files }) }}</dd>
            <dt>{{ t('drive.size') }}</dt><dd>{{ sizeText(totals(infoFolder.id).size, loc) }}</dd>
            <dt>{{ t('drive.where') }}</dt><dd>{{ where(infoFolder.parent_id) }}</dd>
            <dt>{{ t('drive.created') }}</dt><dd>{{ when(infoFolder.created_at) }}<template v-if="people.get(infoFolder.created_by)"> · {{ people.get(infoFolder.created_by) }}</template></dd>
          </template>
          <template v-else>
            <dt>{{ t('drive.holds') }}</dt><dd>{{ t('drive.holdsText', { folders: topTotals.folders, files: topTotals.files }) }}</dd>
            <dt>{{ t('drive.size') }}</dt><dd>{{ sizeText(topTotals.size, loc) }}</dd>
          </template>
        </dl>
        <div v-if="infoFile" class="mt-4 flex flex-wrap gap-2">
          <button type="button" class="btn-primary btn-sm" @click="open(infoFile)"><ExternalLink class="h-3.5 w-3.5" />{{ t('drive.open') }}</button>
          <button type="button" class="btn btn-sm" @click="downloadOf(infoFile)"><Download class="h-3.5 w-3.5" />{{ t('drive.download') }}</button>
        </div>
      </div>
    </component>

    <!-- Menus -->
    <Popover v-if="menu" :anchor="menu.el" :width="230" placement="bottom-end" @close="menu = null">
      <form v-if="menu.mode === 'rename'" class="p-2.5" @submit.prevent="doRename">
        <input ref="renameEl" v-model="renameText" class="field !py-1.5 text-[13.5px]" maxlength="300" :aria-label="t('drive.rename')" data-testid="rename-input" />
      </form>
      <div v-else-if="'file' in menu.target" class="p-1.5" data-testid="file-menu">
        <button type="button" class="menu-item w-full" @click="open(menu.target.file)"><ExternalLink class="h-4 w-4 text-text-2" />{{ t('drive.open') }}</button>
        <template v-if="editable">
          <button type="button" class="menu-item w-full" data-testid="file-rename" @click="startRename"><Pencil class="h-4 w-4 text-text-2" />{{ t('drive.rename') }}</button>
          <button type="button" class="menu-item w-full" data-testid="file-duplicate" @click="copyOf(menu.target.file)"><Copy class="h-4 w-4 text-text-2" />{{ t('drive.duplicate') }}</button>
          <button type="button" class="menu-item w-full" data-testid="file-move" @click="startMove(menu.target)"><FolderInput class="h-4 w-4 text-text-2" />{{ t('drive.move') }}</button>
        </template>
        <button type="button" class="menu-item w-full" data-testid="file-download" @click="downloadOf(menu.target.file)"><Download class="h-4 w-4 text-text-2" />{{ t(`drive.downloadAs.${menu.target.file.kind}`) }}</button>
        <button type="button" class="menu-item w-full" @click="linkTo(menu.target.file)"><Link class="h-4 w-4 text-text-2" />{{ t('drive.copyLink') }}</button>
        <button type="button" class="menu-item w-full" data-testid="file-info" @click="showInfo(menu.target)"><Info class="h-4 w-4 text-text-2" />{{ t('drive.details') }}</button>
        <template v-if="editable">
          <div class="menu-sep" />
          <button type="button" class="menu-item danger w-full" data-testid="file-delete" @click="(removing = menu.target), (menu = null)"><Trash2 class="h-4 w-4" />{{ t('drive.delete') }}</button>
        </template>
      </div>
      <div v-else class="p-1.5" data-testid="folder-menu">
        <button type="button" class="menu-item w-full" @click="(go(menu.target.folder.id), (menu = null))"><ExternalLink class="h-4 w-4 text-text-2" />{{ t('drive.open') }}</button>
        <template v-if="editable">
          <button type="button" class="menu-item w-full" data-testid="folder-rename" @click="startRename"><Pencil class="h-4 w-4 text-text-2" />{{ t('drive.rename') }}</button>
          <button type="button" class="menu-item w-full" data-testid="folder-move" @click="startMove(menu.target)"><FolderInput class="h-4 w-4 text-text-2" />{{ t('drive.move') }}</button>
          <div class="flex flex-wrap gap-1.5 px-2.5 py-2">
            <button
              v-for="c in COLORS"
              :key="c"
              type="button"
              class="dot"
              :class="{ on: folderColor(menu.target.folder).toLowerCase() === c.toLowerCase() }"
              :style="{ background: c }"
              :aria-label="t('drive.colour', { c })"
              @click="colorMenu(c)"
            />
          </div>
        </template>
        <button type="button" class="menu-item w-full" @click="showInfo(menu.target)"><Info class="h-4 w-4 text-text-2" />{{ t('drive.details') }}</button>
        <template v-if="editable">
          <div class="menu-sep" />
          <button type="button" class="menu-item danger w-full" data-testid="folder-delete" @click="(removing = menu.target), (menu = null)"><Trash2 class="h-4 w-4" />{{ t('drive.deleteFolder') }}</button>
        </template>
      </div>
    </Popover>

    <Popover v-if="newOpen" :anchor="newEl" :width="260" placement="bottom-end" @close="newOpen = false">
      <form class="flex gap-2 p-2.5" @submit.prevent="makeFolder">
        <input ref="newInput" v-model="newName" class="field min-w-0 flex-1 !py-1.5 text-[13.5px]" maxlength="200" :placeholder="t('drive.folderName')" :aria-label="t('drive.folderName')" data-testid="folder-name" />
        <button type="submit" class="btn-primary btn-sm" :disabled="!newName.trim()">{{ t('drive.create') }}</button>
      </form>
    </Popover>

    <Overlay :open="!!moving" :title="t('drive.moveTitle')" size="sm" @close="moving = null">
      <div class="move-list" data-testid="move-list">
        <button type="button" class="move-row" :class="{ on: moveTo === '' }" data-testid="move-target" @click="moveTo = ''">
          <FolderIcon class="h-4 w-4 shrink-0 text-text-3" />{{ rootName }}
        </button>
        <button
          v-for="x in choices"
          :key="x.folder.id"
          type="button"
          class="move-row"
          :class="{ on: moveTo === x.folder.id }"
          :style="{ paddingLeft: `${12 + (x.depth + 1) * 18}px` }"
          data-testid="move-target"
          @click="moveTo = x.folder.id"
        >
          <FolderIcon class="h-4 w-4 shrink-0" :style="{ color: folderColor(x.folder) }" />{{ x.folder.name }}
        </button>
      </div>
      <div class="mt-4 flex justify-end gap-2">
        <button type="button" class="btn" @click="moving = null">{{ t('common.cancel') }}</button>
        <button type="button" class="btn-primary" data-testid="move-here" @click="doMove">{{ t('drive.moveHere') }}</button>
      </div>
    </Overlay>
    <Confirm :open="!!removing" :title="removeTitle" :message="removeBody" :confirm-label="t('drive.delete')" danger @confirm="doRemove" @close="removing = null" />
  </div>
</template>

<style scoped>
.fb {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.fb.with-info {
  grid-template-columns: minmax(0, 1fr) 280px;
}
.fb-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 12px;
  margin-bottom: 16px;
}
.crumbs {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 2px;
}
.crumb {
  max-width: 220px;
  overflow: hidden;
  padding: 3px 8px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text-2);
}
.crumb:last-of-type {
  color: var(--text);
}
.crumb:hover,
.crumb.over {
  background: var(--fill-2);
}
/* The app's segmented control (.seg), with icons. */
.seg button {
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  color: var(--text-2);
}
.seg button.on {
  color: var(--text);
}
.folders {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 10px;
}
.folder {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 8px 10px 12px;
  border: 0.5px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  cursor: pointer;
  transition: box-shadow var(--speed), background var(--speed);
}
.folder:hover,
.folder:focus-visible,
.card-f:hover,
.card-f:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--ring);
}
.folder.over,
.fb-list tr.over {
  background: var(--accent-bg, var(--fill));
  box-shadow: 0 0 0 2px var(--accent);
}
.cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 14px;
}
.card-f {
  overflow: hidden;
  border: 0.5px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
  cursor: pointer;
  transition: box-shadow var(--speed);
}
/* Pictures are drawn on white (as their apps show them); they fill the box, scaled to fit. */
.pic {
  display: flex;
  height: 140px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  padding: 10px;
  background: #fff;
  border-bottom: 0.5px solid var(--border);
}
.pic img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.fb-list {
  width: 100%;
  border-collapse: collapse;
  font-size: 13.5px;
}
.fb-list th {
  padding: 6px 10px;
  border-bottom: 0.5px solid var(--border);
  font-size: 12px;
  font-weight: 600;
  text-align: left;
  color: var(--text-3);
}
.fb-list td {
  max-width: 0;
  padding: 7px 10px;
  border-bottom: 0.5px solid var(--line-soft, var(--border));
  white-space: nowrap;
}
.fb-list td:first-child {
  width: 55%;
}
.fb-list tbody tr {
  cursor: pointer;
}
.fb-list tbody tr:hover {
  background: var(--fill-2);
}
@media (max-width: 640px) {
  .hide-sm {
    display: none;
  }
  .fb-list td:first-child {
    width: auto;
  }
}
.fb-info {
  position: sticky;
  top: 16px;
  border: 0.5px solid var(--border);
  border-radius: 14px;
  background: var(--surface);
}
.info-body {
  padding: 16px;
}
.info-pic {
  height: 150px;
  border: 0.5px solid var(--border);
  border-radius: 10px;
}
.info-rows {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 12px;
  font-size: 12.5px;
}
.info-rows dt {
  color: var(--text-3);
}
.info-rows dd {
  min-width: 0;
  overflow-wrap: anywhere;
}
.dot {
  width: 18px;
  height: 18px;
  border-radius: 50%;
}
.dot.on {
  box-shadow: 0 0 0 2px var(--surface), 0 0 0 3.5px var(--accent);
}
.move-list {
  max-height: 50vh;
  overflow-y: auto;
  border: 0.5px solid var(--border);
  border-radius: 10px;
}
.move-row {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 13.5px;
  text-align: left;
}
.move-row:hover {
  background: var(--fill-2);
}
.move-row.on {
  background: var(--accent-bg, var(--fill));
  color: var(--accent-text, var(--accent));
}
</style>

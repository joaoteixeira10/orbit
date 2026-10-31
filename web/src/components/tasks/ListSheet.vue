<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { GripVertical, Plus, X, Check, Trash2, GitBranch, RefreshCw, TriangleAlert, Zap, FolderOpen } from 'lucide-vue-next'
import CopyButton from '@/components/ui/CopyButton.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import Popover from '@/components/ui/Popover.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import IconColorPicker from '@/components/ui/IconColorPicker.vue'
import ColorDots from '@/components/board/ColorDots.vue'
import GitListSheet from './GitListSheet.vue'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import { useSortable } from '@/lib/sortable'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { tasksApi, gitApi, type Column, type TaskList, type GitHookSetup } from '@/api/planner'
import { filesApi, tree, type Folder } from '@/lib/filesApi'

// Creates or edits a task list, as Reminders' list sheet: its name, colour
// and icon, the folder it's the project of (with Files on), plus the board
// columns (statuses) its tasks move through.
// Columns are edited locally and saved together; the server validates them
// (names, ≤12, at least one open and one done) and re-homes the tasks of
// removed columns.
const props = defineProps<{ open: boolean; ws: string; listId?: string }>()
const emit = defineEmits<{ close: []; saved: [list: TaskList] }>()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const MAX_COLUMNS = 12
// A column being edited. key is local and stable (sortable and v-for need
// one before the server assigns ids to new columns).
interface DraftColumn {
  key: string
  id: string
  name: string
  color: string
  done: boolean
}

const list = computed(() => (props.listId ? tasks.listsOf(props.ws).find((l) => l.id === props.listId) : undefined))
const creating = computed(() => !props.listId)
const workspace = computed(() => wss.byId(props.ws))
const editable = computed(
  () => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only,
)

const name = ref('')
const icon = ref('')
const color = ref('#007AFF')
const columns = ref<DraftColumn[]>([])
const busy = ref(false)
const tried = ref(false)
// Its folder (a project): '' none, a folder's id, or NEW_FOLDER (one named
// after the list). A folder has one list: picking one joined to another
// list takes it over.
const NEW_FOLDER = '+new'
const hasFiles = computed(() => !!workspace.value?.modules.includes('files'))
const folders = ref<Folder[]>([])
const folderId = ref('')
const folderOptions = computed(() =>
  tree(folders.value).map((x) => {
    const other = x.folder.list_id && x.folder.list_id !== props.listId ? tasks.listsOf(props.ws).find((l) => l.id === x.folder.list_id) : undefined
    return { id: x.folder.id, label: `${'\u00a0\u00a0\u00a0'.repeat(x.depth)}${x.folder.name}${other ? ` · ${other.name}` : ''}` }
  }),
)
// Creating from a git repository instead (its own sheet).
const fromGit = ref(false)
const gitOn = computed(() => auth.features.git_integration !== false)
let seq = 0
const draft = (c: Partial<Column> & { name: string }): DraftColumn => ({
  key: `c${++seq}`,
  id: c.id ?? '',
  name: c.name,
  color: c.color ?? '',
  done: !!c.done,
})

watch(
  () => [props.open, props.listId] as const,
  ([o]) => {
    if (!o) return
    tried.value = false
    busy.value = false
    fromGit.value = false
    if (list.value) {
      name.value = list.value.name
      icon.value = list.value.icon
      color.value = list.value.color || '#007AFF'
      columns.value = list.value.columns.map(draft)
    } else {
      name.value = ''
      icon.value = 'list-checks'
      color.value = workspace.value?.color || '#007AFF'
      columns.value = [draft({ name: t('lists.todo') }), draft({ name: t('lists.doing') }), draft({ name: t('lists.finished'), done: true })]
    }
    // A new list gets a folder of its own unless another is chosen.
    folderId.value = list.value ? (list.value.folder_id ?? '') : NEW_FOLDER
    if (hasFiles.value) {
      filesApi
        .folders(props.ws)
        .then((f) => (folders.value = f))
        .catch(() => (folders.value = []))
    }
  },
  { immediate: true },
)

// What stops a save, shown under the columns (the server checks it too).
const problem = computed(() => {
  if (columns.value.some((c) => !c.name.trim())) return t('lists.needColumnName')
  if (!columns.value.some((c) => !c.done) || !columns.value.some((c) => c.done)) return t('lists.needOpenDone')
  if (columns.value.length > MAX_COLUMNS) return t('lists.maxColumns')
  return ''
})
const canSave = computed(() => editable.value && !busy.value && !!name.value.trim() && !problem.value)

// ---- Columns ----
const colsEl = ref<HTMLElement | null>(null)
useSortable(
  colsEl,
  (m) => {
    const arr = [...columns.value]
    const from = arr.findIndex((c) => c.key === m.id)
    if (from < 0) return
    const [moved] = arr.splice(from, 1)
    const at = m.beforeId ? arr.findIndex((c) => c.key === m.beforeId) + 1 : 0
    arr.splice(at, 0, moved)
    columns.value = arr
  },
  { handle: '.drag', draggable: '.col-row' },
)

async function addColumn() {
  if (columns.value.length >= MAX_COLUMNS) return
  const c = draft({ name: '' })
  columns.value.push(c)
  await nextTick()
  colsEl.value?.querySelector<HTMLInputElement>(`[data-id="${c.key}"] input`)?.focus()
}
function removeColumn(key: string) {
  if (columns.value.length > 1) columns.value = columns.value.filter((c) => c.key !== key)
}

// The colour popover of one column.
const colorFor = ref<DraftColumn | null>(null)
const colorAnchor = ref<HTMLElement | null>(null)
function pickColor(c: DraftColumn, e: MouseEvent) {
  colorAnchor.value = e.currentTarget as HTMLElement
  colorFor.value = colorFor.value === c ? null : c
}
function setColor(v: string) {
  if (colorFor.value) colorFor.value.color = v
  colorFor.value = null
}

// ---- Save / delete ----
function upsertList(l: TaskList) {
  const arr = (tasks.lists[props.ws] ??= [])
  const i = arr.findIndex((x) => x.id === l.id)
  if (i >= 0) arr[i] = l
  else arr.push(l)
}

async function save() {
  tried.value = true
  if (!canSave.value) return
  busy.value = true
  const body = {
    name: name.value.trim(),
    color: color.value,
    icon: icon.value,
    columns: columns.value.map((c) => ({ id: c.id, name: c.name.trim(), color: c.color, done: c.done })),
  }
  try {
    const before = list.value?.columns.map((c) => c.id) ?? []
    // Made: its folder comes with it (its own, another, or none).
    const into = folderId.value === NEW_FOLDER ? undefined : folderId.value || 'none'
    const saved = props.listId ? await tasksApi.updateList(props.listId, body) : await tasksApi.createList(props.ws, { ...body, folder_id: into })
    const was = list.value?.folder_id ?? ''
    let folder = folderId.value
    if (!props.listId) folder = saved.folder_id ?? ''
    else if (hasFiles.value && folder !== was) {
      if (folder === NEW_FOLDER) folder = (await filesApi.createFolder(props.ws, { name: saved.name, color: saved.color, list_id: saved.id })).id
      else if (folder) await filesApi.updateFolder(folder, { list_id: saved.id })
      else await filesApi.updateFolder(was, { list_id: '' })
    }
    upsertList({ ...saved, folder_id: hasFiles.value ? folder || undefined : saved.folder_id })
    // Tasks of removed columns were moved (and maybe completed): refetch them.
    if (before.some((id) => !saved.columns.some((c) => c.id === id))) void tasks.load(props.ws).catch(() => {})
    emit('saved', saved)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

// ---- Git ----
const git = computed(() => list.value?.git)
// The bot account: chosen by whoever linked the list, or an owner.
const canBot = computed(() => !!git.value && editable.value && (git.value.linked_by === auth.user?.id || wss.byId(props.ws)?.role === 'owner'))
const botToken = ref('')
const botBusy = ref(false)
const hostOf = (u: string) => {
  try {
    return new URL(u).host
  } catch {
    return u
  }
}
async function saveBot(token: string) {
  botBusy.value = true
  try {
    upsertList(await gitApi.setBot(props.listId!, token))
    botToken.value = ''
    toast.success(token ? t('git.bot.set') : t('git.bot.removed'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    botBusy.value = false
  }
}
const syncing = ref(false)
const confirmUnlink = ref(false)
const gitMode = computed(() => {
  const m = git.value?.mode
  return m === 'labels' ? t('git.list.modeLabels') : m === 'project' ? t('git.list.modeProject') : t('git.list.modeIssues')
})
async function syncNow() {
  if (!props.listId) return
  syncing.value = true
  try {
    upsertList(await gitApi.syncList(props.listId))
    void tasks.load(props.ws).catch(() => {})
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    syncing.value = false
  }
}
// Instant updates: a webhook on the repository (Orbit makes it when the
// account may; otherwise the address and secret to add by hand, once).
const hookBusy = ref(false)
const hookSetup = ref<GitHookSetup | null>(null)
async function setInstant(on: boolean) {
  if (!props.listId) return
  hookBusy.value = true
  try {
    if (on) {
      const s = await gitApi.hookOn(props.listId)
      if (s.auto) toast.success(t('git.link.instantAuto'))
      else hookSetup.value = s
    } else {
      await gitApi.hookOff(props.listId)
      hookSetup.value = null
    }
    upsertList(await gitApi.syncList(props.listId))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    hookBusy.value = false
  }
}
async function unlink() {
  if (!props.listId || !list.value) return
  try {
    await gitApi.unlinkList(props.listId)
    upsertList({ ...list.value, git: undefined })
    confirmUnlink.value = false
    toast.success(t('git.link.unlinked'))
  } catch (e: any) {
    toast.error(e.message)
  }
}

const confirmDelete = ref(false)
async function destroy() {
  if (!props.listId) return
  const id = props.listId
  try {
    await tasksApi.removeList(id)
    tasks.lists[props.ws] = tasks.listsOf(props.ws).filter((l) => l.id !== id)
    tasks.tasks[props.ws] = tasks.tasksOf(props.ws).filter((x) => x.list_id !== id)
    confirmDelete.value = false
    toast.success(t('lists.deleted'))
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <GitListSheet v-if="creating" :open="open && fromGit" :ws="ws" @close="emit('close')" @saved="emit('saved', $event)" />
  <Overlay :open="open && (creating || !!list) && !fromGit" :title="creating ? t('lists.new') : t('lists.edit')" size="md" @close="emit('close')">
    <div class="space-y-5">
      <!-- Name and look -->
      <div class="flex flex-col items-center gap-3 pt-1">
        <WsIcon :icon="icon" :color="color" :size="64" :name="name" />
        <input
          v-model="name"
          class="w-full rounded-xl bg-fill-2 px-4 py-2.5 text-center text-[20px] font-semibold outline-none placeholder:text-text-faint focus:shadow-[0_0_0_3px_var(--ring)]"
          :class="{ 'shadow-[0_0_0_2px_var(--crit-border)]': tried && !name.trim() }"
          :style="{ color }"
          :placeholder="t('lists.namePlaceholder')"
          :aria-label="t('lists.namePlaceholder')"
          :disabled="!editable"
          maxlength="120"
          autofocus
          @keydown.enter.prevent="save"
        />
      </div>
      <!-- Its repository -->
      <section v-if="git">
        <h3 class="cap mb-1.5 px-1">{{ t('git.link.section') }}</h3>
        <div class="group">
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #f05032"><GitBranch class="h-4 w-4" /></span>
            <div class="min-w-0 flex-1">
              <a :href="git.url" target="_blank" rel="noopener noreferrer" class="block truncate text-[14px] font-medium hover:underline" :title="t('git.link.open')">{{ git.owner }}/{{ git.repo }}</a>
              <div class="truncate text-[12px] text-text-3">
                {{ gitMode }} · {{ git.synced_at ? t('git.link.synced', { when: ago(git.synced_at, locale()) }) : t('git.link.never') }}
              </div>
            </div>
            <button type="button" class="btn-plain shrink-0" :disabled="syncing" @click="syncNow">
              <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': syncing }" />{{ t('git.link.syncNow') }}
            </button>
          </div>
          <div v-if="git.last_error" class="group-row !min-h-0 !py-2 text-[12.5px] text-crit-text">
            <TriangleAlert class="h-3.5 w-3.5 shrink-0" />{{ t('git.link.syncFailed', { error: git.last_error }) }}
          </div>
        </div>
        <!-- Instant updates: a webhook -->
        <div class="group mt-3" data-testid="git-instant">
          <div class="group-row">
            <Zap class="h-4 w-4 shrink-0" :class="git.instant ? 'text-accent' : 'text-text-3'" />
            <div class="min-w-0 flex-1">
              <div class="text-[14px]">{{ t('git.link.instant') }}</div>
              <div class="text-[12.5px] text-text-3">
                {{ git.instant ? (git.hook_at ? t('git.link.instantOn', { when: ago(git.hook_at, locale()) }) : t('git.link.instantWaiting')) : t('git.link.instantOff') }}
              </div>
            </div>
            <button v-if="editable" type="button" class="btn-plain shrink-0" :disabled="hookBusy" @click="setInstant(!git.instant)">{{ git.instant ? t('git.link.turnOff') : t('git.link.turnOn') }}</button>
          </div>
          <div v-if="hookSetup?.secret" class="space-y-2 px-3.5 py-3 text-[12.5px]">
            <p class="leading-snug text-text-2">{{ t('git.link.instantManual') }}</p>
            <div class="flex items-center gap-2"><span class="w-16 shrink-0 text-text-3">{{ t('git.link.hookUrl') }}</span><code class="min-w-0 flex-1 truncate">{{ hookSetup.url }}</code><CopyButton :value="hookSetup.url" /></div>
            <div class="flex items-center gap-2"><span class="w-16 shrink-0 text-text-3">{{ t('git.link.hookSecret') }}</span><code class="min-w-0 flex-1 truncate">{{ hookSetup.secret }}</code><CopyButton :value="hookSetup.secret" /></div>
          </div>
        </div>

        <!-- Members without their own account: read only, or as a bot — never as whoever linked it -->
        <div class="group mt-3" data-testid="git-bot">
          <div class="group-row !items-start">
            <div class="min-w-0 flex-1">
              <div class="text-[14px]">{{ t('git.bot.title') }}</div>
              <div class="text-[12px] text-text-3">
                {{ git.bot_login ? t('git.bot.asBot', { bot: git.bot_login }) : t('git.bot.readOnly') }}
              </div>
            </div>
            <button v-if="canBot && git.bot_login" type="button" class="btn-plain shrink-0 !text-danger" @click="saveBot('')">{{ t('git.bot.remove') }}</button>
          </div>
          <form v-if="canBot && !git.bot_login" class="group-row flex-wrap !gap-y-2" @submit.prevent="saveBot(botToken)">
            <input v-model="botToken" type="password" class="input min-w-0 flex-1 !py-1 text-[13.5px]" :placeholder="t('git.bot.tokenPh')" autocomplete="off" :aria-label="t('git.bot.token')" />
            <button class="btn btn-sm" :disabled="!botToken.trim() || botBusy">{{ t('git.bot.use') }}</button>
          </form>
        </div>
        <p class="footnote mt-1 px-1 leading-snug">{{ t('git.bot.hint', { host: hostOf(git.host) }) }}</p>
        <p v-if="git.write === 'none'" class="mt-2 rounded-xl bg-fill-2 px-3 py-2 text-[12.5px] text-warn-text">{{ t('git.bot.youRead', { host: hostOf(git.host) }) }}</p>
        <button v-if="editable" type="button" class="btn-plain mt-1.5 !text-danger" @click="confirmUnlink = true">{{ t('git.link.unlink') }}</button>
      </section>

      <div v-if="editable" class="rounded-xl bg-fill-2 p-4">
        <IconColorPicker v-model:icon="icon" v-model:color="color" compact />
      </div>

      <!-- Its folder -->
      <section v-if="hasFiles">
        <h3 class="cap mb-1.5 px-1">{{ t('lists.folder') }}</h3>
        <div class="group">
          <label class="group-row !gap-2.5">
            <FolderOpen class="h-4 w-4 shrink-0 text-text-2" />
            <select v-model="folderId" class="input min-w-0 flex-1 !py-1 text-[14px]" :disabled="!editable" :aria-label="t('lists.folder')" data-testid="list-folder-pick">
              <option value="">{{ t('lists.noFolder') }}</option>
              <option v-for="o in folderOptions" :key="o.id" :value="o.id">{{ o.label }}</option>
              <option :value="NEW_FOLDER">{{ t('lists.newFolder', { name: name.trim() || t('lists.namePlaceholder') }) }}</option>
            </select>
          </label>
        </div>
        <p class="footnote mt-1 px-1 leading-snug">{{ t('lists.folderHint') }}</p>
      </section>

      <!-- Columns -->
      <section>
        <h3 class="cap mb-1.5 px-1">{{ t('lists.columns') }}</h3>
        <div ref="colsEl" class="group">
          <div v-for="c in columns" :key="c.key" :data-id="c.key" class="col-row group-row !gap-2 !py-1.5 !pl-1.5 !pr-2">
            <span v-if="editable" class="drag grid h-8 w-6 shrink-0 cursor-grab place-items-center text-text-faint active:cursor-grabbing" :title="t('lists.reorder')">
              <GripVertical class="h-4 w-4" />
            </span>
            <button
              type="button"
              class="grid h-7 w-7 shrink-0 place-items-center rounded-full hover:bg-fill"
              :title="t('lists.colour')"
              :aria-label="t('lists.colour')"
              :disabled="!editable"
              @click="pickColor(c, $event)"
            >
              <span class="h-3 w-3 rounded-full" :style="{ background: c.color || 'var(--text-faint)' }" />
            </button>
            <input
              v-model="c.name"
              class="min-w-0 flex-1 bg-transparent py-1 text-[14.5px] outline-none placeholder:text-text-faint"
              :class="{ 'placeholder:!text-crit-text': tried && !c.name.trim() }"
              :placeholder="t('lists.columnPlaceholder')"
              :aria-label="t('lists.columnPlaceholder')"
              :disabled="!editable"
              maxlength="60"
              @keydown.enter.prevent="($event.target as HTMLInputElement).blur()"
            />
            <button
              type="button"
              class="done-pill"
              :class="{ on: c.done }"
              :title="t('lists.doneHint')"
              :aria-pressed="c.done"
              :disabled="!editable"
              @click="c.done = !c.done"
            >
              <Check class="h-3 w-3" :stroke-width="3" />{{ t('lists.done') }}
            </button>
            <button
              v-if="editable"
              type="button"
              class="icon-btn h-7 w-7"
              :title="t('lists.removeColumn')"
              :aria-label="t('lists.removeColumn')"
              :disabled="columns.length <= 1"
              @click="removeColumn(c.key)"
            >
              <X class="h-4 w-4" />
            </button>
          </div>
        </div>
        <button v-if="editable" type="button" class="btn-plain mt-1.5" :disabled="columns.length >= MAX_COLUMNS" @click="addColumn">
          <Plus class="h-4 w-4" />{{ t('lists.addColumn') }}
        </button>
        <p v-if="problem" class="mt-1 px-1 text-[12.5px] text-crit-text">{{ problem }}</p>
        <p v-else class="footnote mt-1 px-1 leading-snug">{{ git ? t('git.link.columnsHint') : t('lists.columnsHint') }}</p>
      </section>
    </div>

    <template #footer>
      <button v-if="creating && editable && gitOn" type="button" class="btn-ghost mr-auto !px-2.5" @click="fromGit = true">
        <GitBranch class="h-4 w-4" />{{ t('git.list.fromGit') }}
      </button>
      <button v-if="!creating && editable" type="button" class="btn-ghost mr-auto !px-2.5 !text-danger" @click="confirmDelete = true">
        <Trash2 class="h-4 w-4" />{{ t('lists.delete') }}
      </button>
      <template v-if="editable">
        <button type="button" class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn-primary" :disabled="!canSave" @click="save">{{ creating ? t('lists.create') : t('common.save') }}</button>
      </template>
      <button v-else type="button" class="btn-primary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </Overlay>

  <Popover v-if="colorFor" :anchor="colorAnchor" placement="bottom-start" @close="colorFor = null">
    <ColorDots :model-value="colorFor.color" @update:model-value="setColor" />
  </Popover>

  <Confirm
    :open="confirmUnlink"
    :title="t('git.link.unlinkTitle', { repo: git ? `${git.owner}/${git.repo}` : '' })"
    :message="t('git.link.unlinkBody')"
    :confirm-label="t('git.link.unlink')"
    danger
    @confirm="unlink"
    @close="confirmUnlink = false"
  />

  <Confirm
    :open="confirmDelete"
    :title="t('lists.deleteTitle', { name: list?.name ?? '' })"
    :message="t('lists.deleteBody')"
    :confirm-label="t('common.delete')"
    danger
    @confirm="destroy"
    @close="confirmDelete = false"
  />
</template>

<style scoped>
.col-row {
  min-height: 44px;
  background: var(--surface);
}
:global(html.dark) .col-row {
  background: #2c2c2e;
}
.done-pill {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 4px;
  padding: 3px 9px 3px 7px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 500;
  background: var(--fill-2);
  color: var(--text-3);
  transition: background var(--speed) var(--ease), color var(--speed) var(--ease);
}
.done-pill svg {
  opacity: 0.35;
}
.done-pill:hover:not(:disabled) {
  background: var(--fill);
}
.done-pill.on {
  background: var(--ok-bg);
  color: var(--ok-text);
}
.done-pill.on svg {
  opacity: 1;
}
.col-row.sortable-ghost {
  opacity: 0.4;
}
.col-row.sortable-chosen {
  box-shadow: var(--shadow);
}
</style>

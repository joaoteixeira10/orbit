<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Paperclip, Pencil, Trash2, X } from 'lucide-vue-next'
import Avatar from '@/components/ui/Avatar.vue'
import Confirm from '@/components/ui/Confirm.vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { commentsApi, type Member, type TaskActivity, type TaskComment } from '@/api/planner'
import { useAuthStore } from '@/stores/auth'
import { useTasksStore } from '@/stores/tasks'
import { useToastStore } from '@/stores/toast'
import { onLiveEvent } from '@/lib/live'
import { CLIENT_ID } from '@/lib/api'
import { dueLabel, locale } from '@/lib/dates'
import { fileSize, uploadBlobUrl, uploadFile, uploadsApi, type Upload } from '@/lib/uploads'
import { openFile } from '@/lib/fileView'

// A task's activity, as in Planner: oldest first — its comments (Markdown,
// with author and when; the author edits or deletes theirs, an owner any),
// its changes, and the files put on it. The box at the bottom writes a
// comment ("@" suggests the workspace's members) and takes files (dropped
// on it, or the clip). side: the column beside the details, with a close.
const props = defineProps<{ ws: string; taskId: string; canWrite: boolean; canAttach: boolean; isOwner: boolean; members: Member[]; side?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const auth = useAuthStore()
const toast = useToastStore()
const tasksStore = useTasksStore()

const comments = ref<TaskComment[]>([])
// Keep the count on the task's row and card in step.
watch(
  () => comments.value.length,
  (n) => {
    const tk = tasksStore.allTasksOf(props.ws).find((x) => x.id === props.taskId)
    if (tk) tk.comment_count = n
  },
)
const draft = ref('')
const sending = ref(false)
const editing = ref('')
const editText = ref('')
const askDelete = ref<TaskComment | null>(null)
const box = ref<HTMLTextAreaElement | null>(null)

// The task's changes, among the comments (as in Plane); hidden by choice.
const activity = ref<TaskActivity[]>([])
const showActivity = ref(readShow())
function readShow() {
  try {
    return localStorage.getItem('orbit_task_activity') !== '0'
  } catch {
    return true
  }
}
function setShow(v: boolean) {
  showActivity.value = v
  try {
    localStorage.setItem('orbit_task_activity', v ? '1' : '0')
  } catch {
    /* private mode */
  }
}
type Item = { at: string; c?: TaskComment; a?: TaskActivity; f?: Upload }
const timeline = computed<Item[]>(() => {
  const items: Item[] = comments.value.map((c) => ({ at: c.created_at, c }))
  items.push(...files.value.map((f) => ({ at: f.created_at, f })))
  if (showActivity.value) items.push(...activity.value.map((a) => ({ at: a.created_at, a })))
  return items.sort((x, y) => x.at.localeCompare(y.at))
})
const keyOf = (it: Item) => it.c?.id ?? it.f?.id ?? it.a!.id

// ---- Files on the task ----
const files = ref<Upload[]>([])
const thumbs = ref<Record<string, string>>({})
const uploading = ref(0)
const over = ref(false)
const fileEl = ref<HTMLInputElement | null>(null)
const nameOf = (id: string) => {
  const m = props.members.find((x) => x.user_id === id)
  return m ? m.display_name || m.username : t('comments.someone')
}
const avatarOf = (id: string) => props.members.find((x) => x.user_id === id)?.avatar_url
function extOf(f: Upload): string {
  const m = /\.([a-z0-9]{1,4})$/i.exec(f.name)
  return (m ? m[1] : 'file').toUpperCase()
}
async function loadFiles() {
  try {
    files.value = await uploadsApi.list('task', props.taskId)
  } catch {
    files.value = []
  }
  for (const f of files.value) {
    if (f.image && !thumbs.value[f.id]) void uploadBlobUrl(f.url).then((u) => (thumbs.value = { ...thumbs.value, [f.id]: u })).catch(() => {})
  }
}
async function attach(list: FileList | File[] | null) {
  for (const f of [...(list ?? [])]) {
    uploading.value++
    try {
      await uploadFile('task', props.taskId, f)
    } catch (e: any) {
      toast.error(e?.message ?? String(e))
    } finally {
      uploading.value--
    }
  }
  await loadFiles()
}
async function removeFile(f: Upload) {
  try {
    await uploadsApi.remove(f.id)
    files.value = files.value.filter((x) => x.id !== f.id)
  } catch (e: any) {
    toast.error(e?.message ?? String(e))
  }
}
function onDrop(e: DragEvent) {
  over.value = false
  if (!props.canAttach || !e.dataTransfer?.files.length) return
  e.preventDefault()
  void attach(e.dataTransfer.files)
}
function onDragOver(e: DragEvent) {
  if (!props.canAttach || ![...(e.dataTransfer?.types ?? [])].includes('Files')) return
  e.preventDefault()
  over.value = true
}
function describe(a: TaskActivity): string {
  const actor = a.actor_name || nameOf(a.actor_id)
  const p = a.params ?? {}
  switch (a.kind) {
    case 'due':
      return p.date ? t('comments.act.due', { actor, when: dueLabel(p.date, p.time || null) }) : t('comments.act.dueNone', { actor })
    case 'priority':
      return t('comments.act.priority', { actor, p: t(`tasks.prio.${p.to ?? 0}`).toLowerCase() })
    case 'assigned':
    case 'unassigned':
      return t(`comments.act.${a.kind}`, { actor, names: (p.names ?? []).join(', ') })
    default:
      return t(`comments.act.${a.kind}`, { actor, to: p.to ?? '', list: p.list ?? '' })
  }
}

async function load() {
  const [c, a] = await Promise.all([commentsApi.list(props.taskId).catch(() => []), commentsApi.activity(props.taskId).catch(() => []), loadFiles()])
  comments.value = c
  // A task from before its changes were kept: its start, from the task itself.
  const tk = tasksStore.allTasksOf(props.ws).find((x) => x.id === props.taskId)
  const acts: TaskActivity[] = a
  if (tk && !acts.some((x) => x.kind === 'created')) {
    acts.unshift({ id: `created-${tk.id}`, kind: 'created', actor_id: tk.created_by, actor_name: '', params: {}, created_at: tk.created_at })
  }
  activity.value = acts
}
watch(() => props.taskId, () => ((draft.value = ''), (editing.value = ''), load()), { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) !== props.ws) return
  // Others' comments; anyone's change to tasks (ours too: a new line).
  if ((e.what === 'comments' && e.origin !== CLIENT_ID) || e.what === 'tasks') void load()
  else if (e.what === 'uploads') void loadFiles()
})
onUnmounted(off)

// When: "5 minutes ago" today, else the day ("25 Oct"; the year if not this one).
function when(iso: string): string {
  const d = new Date(iso)
  const m = Math.floor((Date.now() - d.getTime()) / 60000)
  if (m < 1) return t('comments.justNow')
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  if (m < 60) return rtf.format(-m, 'minute')
  if (m < 12 * 60) return rtf.format(-Math.floor(m / 60), 'hour')
  const year = d.getFullYear() === new Date().getFullYear() ? undefined : 'numeric'
  return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year }).format(d)
}

async function send() {
  const body = draft.value.trim()
  if (!body || sending.value) return
  sending.value = true
  try {
    const c = await commentsApi.add(props.taskId, body)
    comments.value.push(c)
    if (c.git_error) toast.error(t('comments.gitError', { error: c.git_error }))
    draft.value = ''
    mention.value = null
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}
function startEdit(c: TaskComment) {
  editing.value = c.id
  editText.value = c.body
}
async function saveEdit(c: TaskComment) {
  const body = editText.value.trim()
  if (!body) return
  try {
    const u = await commentsApi.update(c.id, body)
    comments.value = comments.value.map((x) => (x.id === u.id ? u : x))
    editing.value = ''
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function remove() {
  const c = askDelete.value
  if (!c) return
  try {
    await commentsApi.remove(c.id)
    comments.value = comments.value.filter((x) => x.id !== c.id)
    askDelete.value = null
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- @mentions: the word at the caret, when it starts with "@" ----
const mention = ref<{ start: number; q: string } | null>(null)
const pick = ref(0)
const suggestions = computed(() => {
  if (!mention.value) return []
  const q = mention.value.q.toLowerCase()
  return props.members
    .filter((m) => m.user_id !== auth.user?.id && (m.username.toLowerCase().startsWith(q) || (m.display_name || '').toLowerCase().includes(q)))
    .slice(0, 6)
})
function onInput() {
  const el = box.value
  if (!el) return
  const upto = el.value.slice(0, el.selectionStart ?? 0)
  const m = /(?:^|\s)@([\w.-]{0,32})$/.exec(upto)
  mention.value = m ? { start: upto.length - m[1].length - 1, q: m[1] } : null
  pick.value = 0
}
async function choose(m: Member) {
  const el = box.value
  if (!el || !mention.value) return
  const end = el.selectionStart ?? draft.value.length
  const ins = `@${m.username} `
  draft.value = draft.value.slice(0, mention.value.start) + ins + draft.value.slice(end)
  const caret = mention.value.start + ins.length
  mention.value = null
  await nextTick()
  el.focus()
  el.setSelectionRange(caret, caret)
}
function onKey(e: KeyboardEvent) {
  if (suggestions.value.length) {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      pick.value = (pick.value + (e.key === 'ArrowDown' ? 1 : -1) + suggestions.value.length) % suggestions.value.length
      return
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      void choose(suggestions.value[pick.value])
      return
    }
    if (e.key === 'Escape') {
      e.stopPropagation()
      mention.value = null
      return
    }
  }
  if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
    e.preventDefault()
    void send()
  }
}
</script>

<template>
  <section class="act-col" :class="{ side }" data-testid="comments">
    <header class="act-head">
      <h3 class="min-w-0 flex-1 text-[15px] font-semibold">{{ t('comments.activity') }}</h3>
      <button v-if="activity.length" type="button" class="text-[12.5px] text-accent-text hover:underline" @click="setShow(!showActivity)">{{ showActivity ? t('comments.hideActivity') : t('comments.showActivity') }}</button>
      <button v-if="side" type="button" class="icon-btn h-7 w-7" :aria-label="t('common.close')" :title="t('common.close')" @click="emit('close')"><X class="h-4 w-4" /></button>
    </header>
    <ol class="act-list">
      <li v-for="it in timeline" :key="keyOf(it)" :class="it.a ? 'act' : 'post'" :data-testid="it.a ? 'activity-line' : it.f ? 'activity-file' : undefined">
        <template v-if="it.a">
          <span class="act-dot" aria-hidden="true" />
          <span class="min-w-0 flex-1">{{ describe(it.a) }} · <span :title="new Date(it.a.created_at).toLocaleString()">{{ when(it.a.created_at) }}</span></span>
        </template>
        <template v-else-if="it.f">
          <Avatar :name="nameOf(it.f.owner_id)" :src="avatarOf(it.f.owner_id)" :size="26" class="mt-0.5 shrink-0" />
          <div class="min-w-0 flex-1">
            <p class="text-[13px] text-text-3">
              <b class="text-text">{{ nameOf(it.f.owner_id) }}</b> {{ t('comments.attached') }} · <span :title="new Date(it.f.created_at).toLocaleString()">{{ when(it.f.created_at) }}</span>
            </p>
            <div class="file-card">
              <button type="button" class="flex min-w-0 flex-1 items-center gap-3 text-left" :title="it.f.name" @click="openFile(it.f.id)">
                <img v-if="thumbs[it.f.id]" :src="thumbs[it.f.id]" alt="" class="file-thumb" />
                <span v-else class="file-ext" :class="{ pdf: extOf(it.f) === 'PDF' }">{{ extOf(it.f) }}</span>
                <span class="min-w-0">
                  <span class="block truncate text-[13.5px]">{{ it.f.name }}</span>
                  <span class="block text-[12px] text-text-3">{{ fileSize(it.f.size) }}</span>
                </span>
              </button>
              <button v-if="canAttach" type="button" class="icon-btn h-6 w-6 shrink-0" :aria-label="t('comments.removeFile', { name: it.f.name })" :title="t('comments.removeFile', { name: it.f.name })" data-testid="activity-file-remove" @click="removeFile(it.f)"><X class="h-3.5 w-3.5" /></button>
            </div>
          </div>
        </template>
        <template v-else-if="it.c">
          <Avatar :name="it.c.author_name || '?'" :src="it.c.author_avatar" :size="26" class="mt-0.5 shrink-0" />
          <div class="min-w-0 flex-1">
            <div class="flex items-baseline gap-2">
              <b class="truncate text-[13.5px]">{{ it.c.author_name }}</b>
              <span v-if="it.c.git_author" class="pill shrink-0 !py-0 text-[11px]">{{ t('comments.fromGit') }}</span>
              <span class="shrink-0 text-[12px] text-text-3" :title="new Date(it.c.created_at).toLocaleString()">{{ when(it.c.created_at) }}<template v-if="it.c.edited_at"> · {{ t('comments.edited') }}</template></span>
              <span class="ml-auto flex shrink-0 gap-0.5">
                <button v-if="canWrite && it.c.author_id === auth.user?.id && editing !== it.c.id" class="icon-btn h-6 w-6" :title="t('comments.edit')" :aria-label="t('comments.edit')" @click="startEdit(it.c)"><Pencil class="h-3.5 w-3.5" /></button>
                <button v-if="canWrite && (it.c.author_id === auth.user?.id || isOwner)" class="icon-btn h-6 w-6" :title="t('comments.delete')" :aria-label="t('comments.delete')" @click="askDelete = it.c"><Trash2 class="h-3.5 w-3.5" /></button>
              </span>
            </div>
            <form v-if="editing === it.c.id" class="mt-1 space-y-1.5" @submit.prevent="saveEdit(it.c)">
              <textarea v-model="editText" class="comment-edit" rows="3" maxlength="10000" @keydown.meta.enter.prevent="saveEdit(it.c)" @keydown.ctrl.enter.prevent="saveEdit(it.c)" @keydown.esc.stop="editing = ''" />
              <div class="flex justify-end gap-2">
                <button type="button" class="btn btn-sm" @click="editing = ''">{{ t('comments.cancel') }}</button>
                <button class="btn-primary btn-sm" :disabled="!editText.trim()">{{ t('comments.save') }}</button>
              </div>
            </form>
            <div v-else class="bubble"><MarkdownView class="comment-md" :content="it.c.body" :editable="false" breaks /></div>
          </div>
        </template>
      </li>
      <li v-if="!timeline.length" class="px-1 text-[13px] text-text-3">{{ t('comments.none') }}</li>
    </ol>

    <form v-if="canWrite" class="act-foot" @submit.prevent="send">
      <div class="composer" :class="{ over }" @dragover="onDragOver" @dragleave="over = false" @drop="onDrop">
        <textarea
          ref="box"
          v-model="draft"
          class="comment-box"
          rows="2"
          maxlength="10000"
          :placeholder="canAttach ? t('comments.placeholderFiles') : t('comments.placeholder')"
          :aria-label="t('comments.placeholder')"
          data-testid="comment-box"
          @input="onInput"
          @click="onInput"
          @keydown="onKey"
          @blur="mention = null"
        />
        <ul v-if="suggestions.length" class="mentions" role="listbox">
          <li v-for="(m, i) in suggestions" :key="m.user_id" role="option" :aria-selected="i === pick">
            <button type="button" :class="{ on: i === pick }" @mousedown.prevent="choose(m)">
              <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="20" />
              <span class="truncate">{{ m.display_name || m.username }}</span><span class="text-text-3">@{{ m.username }}</span>
            </button>
          </li>
        </ul>
        <div class="flex items-center justify-end gap-1.5 px-2 pb-2">
          <template v-if="canAttach">
            <input ref="fileEl" type="file" multiple class="hidden" data-testid="comment-files" @change="attach(($event.target as HTMLInputElement).files); ($event.target as HTMLInputElement).value = ''" />
            <span v-if="uploading" class="mr-auto pl-1 text-[12px] text-text-3">{{ t('comments.uploading') }}</span>
            <button type="button" class="icon-btn h-8 w-8" :title="t('comments.attach')" :aria-label="t('comments.attach')" @click="fileEl?.click()"><Paperclip class="h-4 w-4" /></button>
          </template>
          <button class="btn-primary btn-sm !px-4" :disabled="!draft.trim() || sending">{{ t('comments.send') }}</button>
        </div>
      </div>
      <p class="mt-1.5 px-1 text-[11.5px] text-text-3">{{ t('comments.sendHint') }}</p>
    </form>
    <Confirm :open="!!askDelete" :title="t('comments.deleteTitle')" :message="t('comments.deleteBody')" :confirm-label="t('comments.delete')" danger @confirm="remove" @close="askDelete = null" />
  </section>
</template>

<style scoped>
.act-col {
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.act-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 20px 10px;
}
.act-list {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 8px 20px 16px;
}
/* Beside the details: the list scrolls, the box stays at the bottom. */
.act-col.side {
  height: 100%;
}
.act-col.side .act-head {
  border-bottom: 0.5px solid var(--border);
  padding-bottom: 14px;
}
.act-col.side .act-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-top: 16px;
}
.act-foot {
  position: relative;
  padding: 0 20px 16px;
}
.act {
  display: flex;
  align-items: baseline;
  gap: 12px;
  padding-left: 8px;
  font-size: 13px;
  color: var(--text-3);
}
.act-dot {
  flex: none;
  width: 6px;
  height: 6px;
  margin-right: 8px;
  border-radius: 50%;
  background: var(--text-faint);
  transform: translateY(-2px);
}
.post {
  display: flex;
  gap: 10px;
}
.file-card {
  display: flex;
  max-width: 400px;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 10px 10px 10px 12px;
  border-radius: 12px;
  box-shadow: 0 0 0 0.5px var(--border-2);
  background: var(--surface);
}
.file-card:hover {
  background: var(--surface-hover);
}
.file-ext {
  display: inline-flex;
  flex: none;
  width: 28px;
  height: 32px;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background: var(--fill);
  color: var(--text-2);
  font-size: 8.5px;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.file-ext.pdf {
  background: var(--crit-bg);
  color: var(--crit-text);
}
.file-thumb {
  flex: none;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  object-fit: cover;
}
.composer {
  position: relative;
  border-radius: 12px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border-2);
}
.composer:focus-within {
  box-shadow: 0 0 0 0.5px var(--accent), 0 0 0 3px var(--ring);
}
.composer.over {
  box-shadow: 0 0 0 2px var(--accent);
  background: var(--accent-bg);
}
.comment-box {
  display: block;
  width: 100%;
  min-height: 3.2em;
  max-height: 30vh;
  padding: 10px 12px 4px;
  resize: none;
  field-sizing: content;
  background: transparent;
  font-size: 14px;
  outline: none;
}
/* The box shows the focus (its ring); the field inside draws none. */
.comment-box:focus-visible {
  box-shadow: none;
}
.comment-edit {
  display: block;
  width: 100%;
  resize: vertical;
  padding: 9px 12px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 14px;
  outline: none;
}
.bubble {
  margin-top: 2px;
}
.bubble .comment-md {
  font-size: 14px;
  line-height: 1.5;
  padding-bottom: 0;
}
.mentions {
  position: absolute;
  left: 8px;
  bottom: calc(100% + 4px);
  z-index: 5;
  min-width: 220px;
  padding: 4px;
  border-radius: 12px;
  background: var(--surface);
  box-shadow: var(--shadow-lg), 0 0 0 0.5px var(--border);
}
.mentions button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 8px;
  font-size: 13.5px;
  text-align: left;
}
.mentions button.on {
  background: var(--fill-2);
}
</style>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessageSquare, Pencil, Trash2, History } from 'lucide-vue-next'
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

// A task's comments, as in Planner: oldest first, each in Markdown with its
// author and when; the author edits or deletes theirs, an owner deletes any.
// The box below writes one; "@" suggests the workspace's members.
const props = defineProps<{ ws: string; taskId: string; canWrite: boolean; isOwner: boolean; members: Member[] }>()
const { t } = useI18n()
const auth = useAuthStore()
const toast = useToastStore()
const tasksStore = useTasksStore()

const comments = ref<TaskComment[]>([])
// Keep the count on the task's row and card in step.
watch(
  () => comments.value.length,
  (n) => {
    const tk = tasksStore.tasksOf(props.ws).find((x) => x.id === props.taskId)
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
type Item = { at: string; c?: TaskComment; a?: TaskActivity }
const timeline = computed<Item[]>(() => {
  const items: Item[] = comments.value.map((c) => ({ at: c.created_at, c }))
  if (showActivity.value) items.push(...activity.value.map((a) => ({ at: a.created_at, a })))
  return items.sort((x, y) => x.at.localeCompare(y.at))
})
function describe(a: TaskActivity): string {
  const actor = a.actor_name || '?'
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
  const [c, a] = await Promise.all([commentsApi.list(props.taskId).catch(() => []), commentsApi.activity(props.taskId).catch(() => [])])
  comments.value = c
  activity.value = a
}
watch(() => props.taskId, () => ((draft.value = ''), (editing.value = ''), load()), { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) !== props.ws) return
  // Others' comments; anyone's change to tasks (ours too: a new line).
  if ((e.what === 'comments' && e.origin !== CLIENT_ID) || e.what === 'tasks') void load()
})
onUnmounted(off)

function when(iso: string): string {
  const m = Math.floor((Date.now() - new Date(iso).getTime()) / 60000)
  if (m < 1) return t('comments.justNow')
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  if (m < 60) return rtf.format(-m, 'minute')
  if (m < 24 * 60) return rtf.format(-Math.floor(m / 60), 'hour')
  if (m < 7 * 24 * 60) return rtf.format(-Math.floor(m / 1440), 'day')
  return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short', year: 'numeric' }).format(new Date(iso))
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
  <section class="mt-4" data-testid="comments">
    <h3 class="cap mb-1.5 flex items-center gap-1.5 px-1">
      <MessageSquare class="h-3.5 w-3.5" />{{ t('comments.activity') }}<span v-if="comments.length" class="font-normal text-text-3">{{ comments.length }}</span>
      <button v-if="activity.length" type="button" class="ml-auto font-normal normal-case text-accent-text hover:underline" @click="setShow(!showActivity)">{{ showActivity ? t('comments.hideActivity') : t('comments.showActivity') }}</button>
    </h3>
    <ol class="space-y-3">
      <template v-for="it in timeline" :key="it.c?.id ?? it.a!.id">
      <li v-if="it.a" class="act" data-testid="activity-line">
        <History class="h-3.5 w-3.5 shrink-0" />
        <span class="min-w-0 flex-1">{{ describe(it.a) }}</span>
        <span class="shrink-0" :title="new Date(it.a.created_at).toLocaleString()">{{ when(it.a.created_at) }}</span>
      </li>
      <li v-else-if="it.c" class="flex gap-2.5">
        <Avatar :name="it.c!.author_name || '?'" :src="it.c!.author_avatar" :size="26" class="mt-0.5" />
        <div class="min-w-0 flex-1">
          <div class="flex items-baseline gap-2">
            <b class="truncate text-[13.5px]">{{ it.c!.author_name }}</b>
            <span v-if="it.c!.git_author" class="pill shrink-0 !py-0 text-[11px]">{{ t('comments.fromGit') }}</span>
            <span class="text-[12px] text-text-3" :title="new Date(it.c!.created_at).toLocaleString()">{{ when(it.c!.created_at) }}<template v-if="it.c!.edited_at"> · {{ t('comments.edited') }}</template></span>
            <span class="ml-auto flex shrink-0 gap-0.5">
              <button v-if="canWrite && it.c!.author_id === auth.user?.id && editing !== it.c!.id" class="icon-btn h-6 w-6" :title="t('comments.edit')" :aria-label="t('comments.edit')" @click="startEdit(it.c!)"><Pencil class="h-3.5 w-3.5" /></button>
              <button v-if="canWrite && (it.c!.author_id === auth.user?.id || isOwner)" class="icon-btn h-6 w-6" :title="t('comments.delete')" :aria-label="t('comments.delete')" @click="askDelete = it.c!"><Trash2 class="h-3.5 w-3.5" /></button>
            </span>
          </div>
          <form v-if="editing === it.c!.id" class="mt-1 space-y-1.5" @submit.prevent="saveEdit(it.c!)">
            <textarea v-model="editText" class="comment-box" rows="3" maxlength="10000" @keydown.meta.enter.prevent="saveEdit(it.c!)" @keydown.ctrl.enter.prevent="saveEdit(it.c!)" @keydown.esc.stop="editing = ''" />
            <div class="flex justify-end gap-2">
              <button type="button" class="btn btn-sm" @click="editing = ''">{{ t('comments.cancel') }}</button>
              <button class="btn-primary btn-sm" :disabled="!editText.trim()">{{ t('comments.save') }}</button>
            </div>
          </form>
          <div v-else class="bubble"><MarkdownView class="comment-md" :content="it.c!.body" :editable="false" /></div>
        </div>
      </li>
      </template>
    </ol>
    <p v-if="!comments.length && !canWrite" class="px-1 text-[13px] text-text-3">{{ t('comments.none') }}</p>

    <form v-if="canWrite" class="relative mt-3" @submit.prevent="send">
      <textarea
        ref="box"
        v-model="draft"
        class="comment-box"
        rows="2"
        maxlength="10000"
        :placeholder="t('comments.placeholder')"
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
      <div class="mt-1.5 flex items-center justify-end gap-3">
        <span class="text-[11.5px] text-text-3">{{ t('comments.sendHint') }}</span>
        <button class="btn-primary btn-sm" :disabled="!draft.trim() || sending">{{ t('comments.send') }}</button>
      </div>
    </form>
    <Confirm :open="!!askDelete" :title="t('comments.deleteTitle')" :message="t('comments.deleteBody')" :confirm-label="t('comments.delete')" danger @confirm="remove" @close="askDelete = null" />
  </section>
</template>

<style scoped>
.comment-box {
  display: block;
  width: 100%;
  resize: vertical;
  min-height: 3.2em;
  padding: 9px 12px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 14px;
  outline: none;
}
.comment-box:focus {
  box-shadow: 0 0 0 3px var(--ring);
}
.act {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-left: 6px;
  font-size: 12.5px;
  color: var(--text-3);
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
  bottom: calc(100% - 34px);
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

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ArrowLeft, ArrowUp, Check, FileText, History, Loader2, Maximize2, Minimize2, Paperclip, Plus, Sparkles, Square, Trash2, TriangleAlert, X } from 'lucide-vue-next'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import Confirm from '@/components/ui/Confirm.vue'
import AssistantPlan from './AssistantPlan.vue'
import { MAX_FILES, useAssistantStore } from '@/stores/assistant'
import { useToastStore } from '@/stores/toast'
import { downloadUpload, fileSize } from '@/lib/uploads'
import { fmtTook, fmtWhen, relTime } from '@/lib/format'
import type { AssistantFile, AssistantStep } from '@/api/assistant'

// The assistant's chat window, floating at the bottom right — or large in the
// middle of the screen (expand); the whole screen on a phone. Closed, a round
// button in that corner opens it (on a computer): a conversation in bubbles (the person's, its answers with the
// tools it used above them), the box to ask in (📎 or a drop attaches
// files), and the list of past conversations. Ask mode reads; Agent mode also lines up changes, shown in
// one card to approve. ⌘J / Ctrl+J opens and closes it.
const { t, locale } = useI18n()
const route = useRoute()
const store = useAssistantStore()
const draft = ref('')
const view = ref<'chat' | 'history'>('chat')
const body = ref<HTMLElement | null>(null)
const input = ref<HTMLTextAreaElement | null>(null)
const confirmAll = ref(false)

function stepText(s: AssistantStep) {
  const key = `assistant.step.${s.tool}`
  if (s.tool === 'search' && !s.hint) return t('assistant.step.list_pages')
  // A change (Agent mode) is only lined up; the card says what it is.
  if (/^(create|update|delete|trash|move|complete|comment|save|page_from)_/.test(s.tool)) return t('assistant.step.change')
  return t(key) === key ? t('assistant.step.other', { tool: s.tool }) : t(key, { hint: s.hint ?? '' })
}

// What's on screen, so "this page" means something.
function context() {
  const title = document.querySelector('main h1')?.textContent?.trim() ?? ''
  return {
    path: route.fullPath,
    title,
    workspace_id: typeof route.params.ws === 'string' ? route.params.ws : '',
    now: new Date().toLocaleString(locale.value, { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', hour: '2-digit', minute: '2-digit' }),
    time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone,
  }
}

// Files: from 📎 or dropped on the window, for the next message.
const toast = useToastStore()
const picker = ref<HTMLInputElement | null>(null)
const dropping = ref(false)
function addFiles(list: FileList | null | undefined) {
  const files = Array.from(list ?? [])
  if (store.attached.length + files.length > MAX_FILES) toast.error(t('assistant.tooMany', { n: MAX_FILES }))
  for (const f of files.slice(0, Math.max(0, MAX_FILES - store.attached.length))) void store.attach(f)
  if (picker.value) picker.value.value = ''
  view.value = 'chat'
}
function onDrop(e: DragEvent) {
  dropping.value = false
  addFiles(e.dataTransfer?.files)
}
function onDrag(e: DragEvent) {
  if (e.dataTransfer?.types.includes('Files')) dropping.value = true
}
const uploading = computed(() => store.attached.some((a) => a.busy))
function download(f: AssistantFile) {
  void downloadUpload(f.url || `/api/v1/uploads/${f.id}/${encodeURIComponent(f.name)}`, f.name).catch((e) => toast.error(e.message))
}

async function send(text = draft.value) {
  const msg = text.trim()
  if (!msg || store.sending || uploading.value) return
  draft.value = ''
  view.value = 'chat'
  await store.send(msg, context())
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    void send()
  }
}

async function showHistory() {
  view.value = 'history'
  await store.loadChats()
}
async function pick(id: string) {
  await store.openChat(id)
  view.value = 'chat'
}
function fresh() {
  store.newChat()
  view.value = 'chat'
  void nextTick(() => input.value?.focus())
}

// Keep the latest in sight — as the answer is written too, unless they've
// scrolled up to read; the box grows with what's typed.
watch(
  () => [store.turns.length, store.steps.length, store.sending, store.error],
  () => nextTick(() => body.value?.scrollTo({ top: body.value.scrollHeight })),
)
watch(
  () => store.draft,
  () => {
    const el = body.value
    if (!el || el.scrollHeight - el.scrollTop - el.clientHeight > 80) return
    void nextTick(() => el.scrollTo({ top: el.scrollHeight }))
  },
)
watch(draft, () =>
  nextTick(() => {
    const el = input.value
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 120)}px`
  }),
)
watch(
  () => store.open,
  (o) => o && nextTick(() => input.value?.focus()),
)

function onGlobalKey(e: KeyboardEvent) {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'j' && store.enabled) {
    e.preventDefault()
    store.open = !store.open
  } else if (e.key === 'Escape' && store.open) {
    if (store.big) store.setBig(false)
    else store.open = false
  }
}
onMounted(() => window.addEventListener('keydown', onGlobalKey))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKey))

const left = computed(() => store.left)

// While an answer is on its way, how long it's been (a slow model can take
// minutes).
const now = ref(Date.now())
let tick: number | undefined
watch(
  () => store.sending,
  (on) => {
    clearInterval(tick)
    now.value = Date.now()
    if (on) tick = window.setInterval(() => (now.value = Date.now()), 1000)
  },
)
onUnmounted(() => clearInterval(tick))
</script>

<template>
  <Transition name="ai-fade">
    <div v-if="store.open && store.enabled && store.big" class="ai-backdrop" aria-hidden="true" @click="store.setBig(false)" />
  </Transition>
  <button v-if="store.enabled && !store.open" class="ai-fab" :title="`${t('assistant.open')} (⌘J)`" :aria-label="t('assistant.open')" data-testid="assistant-fab" @click="store.open = true">
    <Sparkles class="h-[22px] w-[22px]" />
  </button>
  <Transition name="ai-pop">
  <aside
    v-if="store.open && store.enabled"
    class="ai-panel"
    :class="{ big: store.big }"
    :aria-label="t('assistant.title')"
    data-testid="assistant-panel"
    @dragover.prevent="onDrag"
    @dragleave.self="dropping = false"
    @drop.prevent="onDrop"
  >
    <div v-if="dropping" class="ai-drop"><Paperclip class="h-6 w-6" />{{ t('assistant.attachDrop') }}</div>
    <header class="ai-head">
      <button v-if="view === 'history'" class="icon-btn h-8 w-8" :title="t('assistant.back')" :aria-label="t('assistant.back')" @click="view = 'chat'"><ArrowLeft class="h-4 w-4" /></button>
      <Sparkles v-else class="h-[18px] w-[18px] text-accent" />
      <div class="min-w-0 flex-1">
        <div class="truncate text-[14px] font-semibold">{{ view === 'history' ? t('assistant.history') : t('assistant.title') }}</div>
        <div v-if="store.status?.model" class="truncate text-[11px] text-text-3" data-testid="assistant-model-name">{{ store.status.model }}<template v-if="store.status.own"> · {{ t('assistant.yours') }}</template></div>
      </div>
      <button v-if="view === 'chat'" class="icon-btn h-8 w-8" :title="t('assistant.history')" :aria-label="t('assistant.history')" data-testid="assistant-history" @click="showHistory"><History class="h-4 w-4" /></button>
      <button class="icon-btn h-8 w-8" :title="t('assistant.newChat')" :aria-label="t('assistant.newChat')" data-testid="assistant-new" @click="fresh"><Plus class="h-4 w-4" /></button>
      <button class="icon-btn ai-grow h-8 w-8" :title="store.big ? t('assistant.shrink') : t('assistant.grow')" :aria-label="store.big ? t('assistant.shrink') : t('assistant.grow')" data-testid="assistant-grow" @click="store.setBig(!store.big)">
        <Minimize2 v-if="store.big" class="h-4 w-4" /><Maximize2 v-else class="h-4 w-4" />
      </button>
      <button class="icon-btn h-8 w-8" :title="t('assistant.close')" :aria-label="t('assistant.close')" @click="store.open = false"><X class="h-4 w-4" /></button>
    </header>

    <!-- Past conversations -->
    <div v-if="view === 'history'" class="ai-body">
      <p v-if="!store.chats.length" class="px-4 py-6 text-center text-[13px] text-text-3">{{ t('assistant.noChats') }}</p>
      <div v-for="c in store.chats" :key="c.id" class="ai-chat group/c" :class="{ on: c.id === store.chatId }" data-testid="assistant-chat-item">
        <button class="min-w-0 flex-1 text-left" @click="pick(c.id)">
          <span class="block truncate text-[13.5px] font-medium">{{ c.title }}</span>
          <span class="block text-[11.5px] text-text-3">{{ relTime(c.updated_at) }}</span>
        </button>
        <button class="icon-btn h-7 w-7 opacity-0 group-hover/c:opacity-100 focus:opacity-100" :title="t('assistant.deleteChat')" :aria-label="t('assistant.deleteChat')" @click="store.removeChat(c.id)"><Trash2 class="h-3.5 w-3.5" /></button>
      </div>
      <div v-if="store.chats.length" class="p-3 text-center">
        <button class="text-[12.5px] text-crit-text hover:underline" data-testid="assistant-delete-all" @click="confirmAll = true">{{ t('assistant.deleteAll') }}</button>
      </div>
    </div>

    <!-- The conversation -->
    <div v-else ref="body" class="ai-body" aria-live="polite">
      <div v-if="!store.turns.length && !store.sending" class="ai-empty">
        <Sparkles class="mx-auto h-8 w-8 text-accent" :stroke-width="1.6" />
        <h3 class="mt-3 text-[15px] font-semibold">{{ t('assistant.empty') }}</h3>
        <p class="mt-1 text-[12.5px] text-text-3">{{ store.mode === 'agent' ? t('assistant.emptyHintAgent') : t('assistant.emptyHint') }}</p>
        <div class="mt-4 flex flex-col gap-2">
          <button v-for="k in ['week', 'today', 'page']" :key="k" class="ai-suggest" @click="send(t(`assistant.suggest.${k}`))">{{ t(`assistant.suggest.${k}`) }}</button>
        </div>
      </div>
      <template v-for="(m, i) in store.turns" :key="i">
        <div v-if="m.role === 'user'" class="ai-q">
          <div class="ai-user" data-testid="assistant-user">{{ m.text }}</div>
          <div v-if="m.files?.length" class="ai-sent-files">
            <button v-for="f in m.files" :key="f.id" class="ai-file" :title="f.name" data-testid="assistant-sent-file" @click="download(f)">
              <FileText class="h-3.5 w-3.5 flex-none" /><span class="truncate">{{ f.name }}</span>
            </button>
          </div>
          <div v-if="m.at" class="ai-meta" data-testid="assistant-asked">{{ fmtWhen(m.at) }}</div>
        </div>
        <div v-else class="ai-answer" data-testid="assistant-answer">
          <ul v-if="m.steps?.length" class="ai-steps">
            <li v-for="(s, k) in m.steps" :key="k"><Check class="h-3 w-3" />{{ stepText(s) }}</li>
          </ul>
          <div class="ai-bubble"><MarkdownView :content="m.text" :editable="false" plain class="ai-md" /></div>
          <div v-if="m.at" class="ai-meta" data-testid="assistant-took">
            {{ fmtWhen(m.at) }}<template v-if="m.took_ms"> · <span :title="t('assistant.tookFor', { d: fmtTook(m.took_ms) })">{{ t('assistant.took', { d: fmtTook(m.took_ms) }) }}</span></template>
          </div>
          <AssistantPlan v-if="m.actions?.length" :actions="m.actions" :busy="store.applying === i" @apply="store.apply(i, $event)" />
        </div>
      </template>
      <div v-if="store.sending" class="ai-answer" data-testid="assistant-working">
        <ul v-if="store.steps.length" class="ai-steps">
          <li v-for="(s, k) in store.steps" :key="k"><Check class="h-3 w-3" />{{ stepText(s) }}</li>
        </ul>
        <div v-if="store.draft" class="ai-bubble"><MarkdownView :content="store.draft" :editable="false" plain class="ai-md" /></div>
        <div v-else class="ai-bubble ai-typing" role="status" :aria-label="t('assistant.thinking')"><span /><span /><span /></div>
        <div class="ai-meta" data-testid="assistant-elapsed">{{ fmtTook(Math.max(0, now - store.askedAt)) }}</div>
      </div>
      <p v-if="store.error" class="ai-error" role="alert" data-testid="assistant-error">{{ store.error }}</p>
    </div>

    <footer v-if="view === 'chat'" class="ai-foot">
      <div v-if="store.attached.length" class="ai-files">
        <span v-for="a in store.attached" :key="a.key" class="ai-file" :class="{ bad: a.error }" :title="a.error || a.name" data-testid="assistant-attached">
          <Loader2 v-if="a.busy" class="h-3.5 w-3.5 flex-none animate-spin" />
          <TriangleAlert v-else-if="a.error" class="h-3.5 w-3.5 flex-none" />
          <FileText v-else class="h-3.5 w-3.5 flex-none" />
          <span class="truncate">{{ a.name }}</span>
          <span class="flex-none text-text-3">{{ fileSize(a.size) }}</span>
          <button class="ai-file-x" :aria-label="t('assistant.removeFile', { name: a.name })" @click="store.detach(a.key)"><X class="h-3 w-3" /></button>
        </span>
      </div>
      <div class="ai-box">
        <button class="ai-clip" :title="t('assistant.attach')" :aria-label="t('assistant.attach')" data-testid="assistant-attach" @click="picker?.click()"><Paperclip class="h-4 w-4" /></button>
        <input ref="picker" type="file" multiple hidden data-testid="assistant-file-input" @change="addFiles(($event.target as HTMLInputElement).files)" />
        <textarea
          ref="input"
          v-model="draft"
          rows="1"
          :placeholder="t('assistant.placeholder')"
          :aria-label="t('assistant.placeholder')"
          data-testid="assistant-input"
          @keydown="onKey"
        />
        <button v-if="store.sending" class="ai-send" :title="t('assistant.stop')" :aria-label="t('assistant.stop')" @click="store.stop()"><Square class="h-3.5 w-3.5" fill="currentColor" /></button>
        <button v-else class="ai-send" :disabled="!draft.trim() || left === 0 || uploading" :title="t('assistant.send')" :aria-label="t('assistant.send')" data-testid="assistant-send" @click="send()"><ArrowUp class="h-4 w-4" :stroke-width="2.6" /></button>
      </div>
      <div class="mt-1.5 flex items-center justify-between px-1 text-[11px] text-text-3">
        <div class="ai-mode" role="radiogroup" :aria-label="t('assistant.mode.label')">
          <button
            v-for="m in ['ask', 'agent'] as const"
            :key="m"
            role="radio"
            :aria-checked="store.mode === m"
            :class="{ on: store.mode === m }"
            :title="t(`assistant.mode.${m}Hint`)"
            :data-testid="`assistant-mode-${m}`"
            @click="store.setMode(m)"
          >
            {{ t(`assistant.mode.${m}`) }}
          </button>
        </div>
        <span v-if="left !== null">{{ t('assistant.left', { n: left }, left) }}</span>
      </div>
    </footer>

    <Confirm
      :open="confirmAll"
      :title="t('assistant.deleteAll')"
      :message="t('assistant.deleteAllConfirm')"
      :confirm-label="t('assistant.deleteAll')"
      danger
      @confirm="store.removeAll().then(() => (confirmAll = false))"
      @close="confirmAll = false"
    />
  </aside>
  </Transition>
</template>

<style scoped>
.ai-panel {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 46;
  display: flex;
  width: 380px;
  height: min(600px, calc(100dvh - 40px));
  flex-direction: column;
  overflow: hidden;
  border-radius: 18px;
  background: var(--surface);
  box-shadow:
    0 0 0 0.5px var(--border),
    0 16px 48px rgba(0, 0, 0, 0.18),
    0 4px 12px rgba(0, 0, 0, 0.08);
  transform-origin: bottom right;
}
/* Expanded: large, in the middle, over a dimmed page. */
.ai-panel.big {
  inset: 0;
  width: min(780px, calc(100vw - 48px));
  height: min(860px, calc(100dvh - 64px));
  margin: auto;
  transform-origin: center;
}
.ai-panel.big .ai-body {
  padding: 20px 28px;
}
.ai-panel.big .ai-foot {
  padding: 10px 24px 14px;
}
.ai-backdrop {
  position: fixed;
  inset: 0;
  z-index: 45;
  background: rgba(0, 0, 0, 0.32);
}
.ai-fade-enter-active,
.ai-fade-leave-active {
  transition: opacity 0.18s ease;
}
.ai-fade-enter-from,
.ai-fade-leave-to {
  opacity: 0;
}
/* Closed: a round button in the corner it opens in. */
.ai-fab {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 40;
  display: grid;
  width: 48px;
  height: 48px;
  place-items: center;
  border-radius: 999px;
  background: var(--accent);
  color: var(--accent-fg);
  box-shadow:
    0 6px 20px rgba(0, 0, 0, 0.18),
    0 1px 4px rgba(0, 0, 0, 0.1);
  transition: transform var(--speed);
}
.ai-fab:hover {
  transform: scale(1.06);
}
/* A phone gets the whole screen, and opens it from the top bar. */
@media (max-width: 639px) {
  .ai-panel,
  .ai-panel.big {
    inset: 0;
    width: auto;
    height: auto;
    margin: 0;
    border-radius: 0;
  }
  .ai-fab,
  .ai-grow,
  .ai-backdrop {
    display: none;
  }
  .ai-head {
    padding-top: calc(10px + env(safe-area-inset-top));
  }
  .ai-foot {
    padding-bottom: calc(12px + env(safe-area-inset-bottom));
  }
}
.ai-pop-enter-active,
.ai-pop-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}
.ai-pop-enter-from,
.ai-pop-leave-to {
  opacity: 0;
  transform: translateY(12px) scale(0.97);
}
@media (prefers-reduced-motion: reduce) {
  .ai-pop-enter-active,
  .ai-pop-leave-active {
    transition: opacity 0.12s ease;
  }
  .ai-pop-enter-from,
  .ai-pop-leave-to {
    transform: none;
  }
}
.ai-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 10px 10px 16px;
  border-bottom: 0.5px solid var(--border);
}
.ai-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 14px;
}
.ai-empty {
  margin: 24px 8px;
  text-align: center;
}
.ai-suggest {
  padding: 9px 12px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 13px;
  text-align: left;
  transition: background var(--speed);
}
.ai-suggest:hover {
  background: var(--fill);
}
.ai-q {
  display: flex;
  margin-bottom: 12px;
  flex-direction: column;
  align-items: flex-end;
}
.ai-user {
  max-width: 80%;
  width: fit-content;
  padding: 8px 12px;
  border-radius: 18px 18px 4px 18px;
  background: var(--accent);
  color: var(--accent-fg);
  font-size: 14px;
  white-space: pre-wrap;
  word-break: break-word;
}
.ai-answer {
  margin-bottom: 12px;
  max-width: 88%;
  font-size: 14px;
}
/* When it was asked or answered, and how long the answer took. */
.ai-meta {
  margin-top: 3px;
  padding: 0 8px;
  font-size: 11px;
  color: var(--text-3);
  font-variant-numeric: tabular-nums;
}
.ai-bubble {
  width: fit-content;
  max-width: 100%;
  padding: 8px 12px;
  border-radius: 18px 18px 18px 4px;
  background: var(--fill-2);
  word-break: break-word;
}
.ai-steps {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin: 0 0 6px 4px;
  font-size: 11.5px;
  color: var(--text-3);
}
.ai-steps li {
  display: flex;
  align-items: center;
  gap: 6px;
}
/* The page reader's size and spacing, made a message's. */
.ai-bubble .ai-md {
  padding-bottom: 0;
  font-size: 14px;
  line-height: 1.5;
}
.ai-md :deep(p) {
  margin: 0 0 6px;
}
.ai-md :deep(> :last-child) {
  margin-bottom: 0;
}
/* Three dots while it works. */
.ai-typing {
  display: flex;
  gap: 4px;
  padding: 12px 14px;
}
.ai-typing span {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: var(--text-3);
  animation: ai-dot 1.2s infinite ease-in-out;
}
.ai-typing span:nth-child(2) {
  animation-delay: 0.15s;
}
.ai-typing span:nth-child(3) {
  animation-delay: 0.3s;
}
@keyframes ai-dot {
  0%,
  60%,
  100% {
    opacity: 0.3;
    transform: none;
  }
  30% {
    opacity: 1;
    transform: translateY(-3px);
  }
}
@media (prefers-reduced-motion: reduce) {
  .ai-typing span {
    animation: none;
    opacity: 0.6;
  }
}
.ai-error {
  margin-top: 4px;
  font-size: 13px;
  color: var(--crit-text);
}
.ai-chat {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 10px;
}
.ai-chat:hover,
.ai-chat.on {
  background: var(--fill-2);
}
.ai-foot {
  padding: 8px 10px 10px;
  border-top: 0.5px solid var(--border);
}
.ai-box {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  padding: 6px 6px 6px 6px;
  border-radius: 18px;
  background: var(--fill-2);
  box-shadow: inset 0 0 0 0.5px var(--border);
  transition: box-shadow var(--speed);
}
/* The ring is the box's, not the text's. */
.ai-box:focus-within {
  box-shadow: inset 0 0 0 1.5px var(--accent);
}
.ai-box textarea:focus,
.ai-box textarea:focus-visible {
  outline: none !important;
  box-shadow: none !important;
}
.ai-box textarea {
  flex: 1;
  min-height: 24px;
  max-height: 120px;
  resize: none;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text);
  font-size: 14px;
  line-height: 1.45;
  padding: 3px 0;
}
/* 📎, at the box's start. */
.ai-clip {
  display: grid;
  flex: none;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 999px;
  color: var(--text-2);
}
.ai-clip:hover {
  background: var(--fill);
  color: var(--text);
}
/* Files: those for the next message, above the box; those sent, under the
   message. */
.ai-files,
.ai-sent-files {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.ai-files {
  margin-bottom: 8px;
}
.ai-sent-files {
  justify-content: flex-end;
  max-width: 80%;
  margin-top: 4px;
}
.ai-file {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 5px;
  padding: 4px 6px 4px 8px;
  border-radius: 10px;
  background: var(--fill-2);
  box-shadow: inset 0 0 0 0.5px var(--border);
  font-size: 12px;
  color: var(--text);
}
button.ai-file:hover {
  background: var(--fill);
}
.ai-file.bad {
  color: var(--crit-text);
}
.ai-file-x {
  display: grid;
  width: 16px;
  height: 16px;
  place-items: center;
  border-radius: 999px;
  color: var(--text-3);
}
.ai-file-x:hover {
  background: var(--fill);
  color: var(--text);
}
/* Dragging files over the window. */
.ai-drop {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 2px dashed var(--accent);
  border-radius: inherit;
  background: color-mix(in srgb, var(--surface) 88%, transparent);
  color: var(--accent);
  font-size: 13.5px;
  font-weight: 600;
  pointer-events: none;
}
.ai-send {
  display: grid;
  flex: none;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 999px;
  background: var(--accent);
  color: var(--accent-fg);
}
.ai-send:disabled {
  opacity: 0.35;
}
.ai-mode {
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: 8px;
  background: var(--fill-2);
}
.ai-mode button {
  padding: 1px 8px;
  border-radius: 6px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-3);
  transition:
    background var(--speed),
    color var(--speed);
}
.ai-mode button.on {
  background: var(--surface);
  color: var(--text);
  box-shadow: 0 0 0 0.5px var(--border);
}
</style>

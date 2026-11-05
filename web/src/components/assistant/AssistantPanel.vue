<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ArrowLeft, ArrowUp, Check, History, Plus, Sparkles, Square, Trash2, X } from 'lucide-vue-next'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { useAssistantStore } from '@/stores/assistant'
import { relTime } from '@/lib/format'
import type { AssistantStep } from '@/api/assistant'

// The assistant's chat window, floating at the bottom right (the whole screen
// on a phone): a conversation in bubbles (the person's, its answers with the
// tools it used above them), the box to ask in, and the list of past
// conversations. ⌘J / Ctrl+J opens and closes it.
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

async function send(text = draft.value) {
  const msg = text.trim()
  if (!msg || store.sending) return
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

// Keep the latest in sight; the box grows with what's typed.
watch(
  () => [store.turns.length, store.steps.length, store.sending, store.error],
  () => nextTick(() => body.value?.scrollTo({ top: body.value.scrollHeight })),
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
    store.open = false
  }
}
onMounted(() => window.addEventListener('keydown', onGlobalKey))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKey))

const left = computed(() => store.left)
</script>

<template>
  <Transition name="ai-pop">
  <aside v-if="store.open && store.enabled" class="ai-panel" :aria-label="t('assistant.title')" data-testid="assistant-panel">
    <header class="ai-head">
      <button v-if="view === 'history'" class="icon-btn h-8 w-8" :title="t('assistant.back')" :aria-label="t('assistant.back')" @click="view = 'chat'"><ArrowLeft class="h-4 w-4" /></button>
      <Sparkles v-else class="h-[18px] w-[18px] text-accent" />
      <div class="min-w-0 flex-1">
        <div class="truncate text-[14px] font-semibold">{{ view === 'history' ? t('assistant.history') : t('assistant.title') }}</div>
        <div v-if="store.status?.model" class="truncate text-[11px] text-text-3">{{ store.status.model }}</div>
      </div>
      <button v-if="view === 'chat'" class="icon-btn h-8 w-8" :title="t('assistant.history')" :aria-label="t('assistant.history')" data-testid="assistant-history" @click="showHistory"><History class="h-4 w-4" /></button>
      <button class="icon-btn h-8 w-8" :title="t('assistant.newChat')" :aria-label="t('assistant.newChat')" data-testid="assistant-new" @click="fresh"><Plus class="h-4 w-4" /></button>
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
        <p class="mt-1 text-[12.5px] text-text-3">{{ t('assistant.emptyHint') }}</p>
        <div class="mt-4 flex flex-col gap-2">
          <button v-for="k in ['week', 'today', 'page']" :key="k" class="ai-suggest" @click="send(t(`assistant.suggest.${k}`))">{{ t(`assistant.suggest.${k}`) }}</button>
        </div>
      </div>
      <template v-for="(m, i) in store.turns" :key="i">
        <div v-if="m.role === 'user'" class="ai-user" data-testid="assistant-user">{{ m.text }}</div>
        <div v-else class="ai-answer" data-testid="assistant-answer">
          <ul v-if="m.steps?.length" class="ai-steps">
            <li v-for="(s, k) in m.steps" :key="k"><Check class="h-3 w-3" />{{ stepText(s) }}</li>
          </ul>
          <div class="ai-bubble"><MarkdownView :content="m.text" :editable="false" plain class="ai-md" /></div>
        </div>
      </template>
      <div v-if="store.sending" class="ai-answer" data-testid="assistant-working">
        <ul v-if="store.steps.length" class="ai-steps">
          <li v-for="(s, k) in store.steps" :key="k"><Check class="h-3 w-3" />{{ stepText(s) }}</li>
        </ul>
        <div class="ai-bubble ai-typing" role="status" :aria-label="t('assistant.thinking')"><span /><span /><span /></div>
      </div>
      <p v-if="store.error" class="ai-error" role="alert" data-testid="assistant-error">{{ store.error }}</p>
    </div>

    <footer v-if="view === 'chat'" class="ai-foot">
      <div class="ai-box">
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
        <button v-else class="ai-send" :disabled="!draft.trim() || left === 0" :title="t('assistant.send')" :aria-label="t('assistant.send')" data-testid="assistant-send" @click="send()"><ArrowUp class="h-4 w-4" :stroke-width="2.6" /></button>
      </div>
      <div class="mt-1.5 flex justify-between px-1 text-[11px] text-text-3">
        <span>{{ t('assistant.askMode') }}</span>
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
/* A phone gets the whole screen. */
@media (max-width: 639px) {
  .ai-panel {
    inset: 0;
    width: auto;
    height: auto;
    border-radius: 0;
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
.ai-user {
  margin: 0 0 12px auto;
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
  padding: 6px 6px 6px 12px;
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
</style>

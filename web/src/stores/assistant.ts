import { defineStore } from 'pinia'
import { assistantApi, type AssistantChatSummary, type AssistantContext, type AssistantMode, type AssistantStatus, type AssistantStep, type AssistantTurn } from '@/api/assistant'

const MODE_KEY = 'orbit.assistant.mode'
function savedMode(): AssistantMode {
  try {
    return localStorage.getItem(MODE_KEY) === 'agent' ? 'agent' : 'ask'
  } catch {
    return 'ask'
  }
}

// The assistant's panel: whether it's open, the conversation in it (and the
// list of mine), and the answer being worked out (the tools it uses as it goes,
// and its words as they're written).
export const useAssistantStore = defineStore('assistant', {
  state: () => ({
    open: false,
    status: null as AssistantStatus | null,
    chatId: '',
    turns: [] as AssistantTurn[],
    chats: [] as AssistantChatSummary[],
    sending: false,
    steps: [] as AssistantStep[], // of the answer on its way
    draft: '', // its words so far (those before a tool runs aren't the answer)
    error: '',
    controller: null as AbortController | null,
    mode: savedMode(), // ask, or agent: it lines up changes to approve
    applying: -1, // the turn whose changes are being made
  }),
  getters: {
    enabled: (s) => !!s.status?.enabled,
    left: (s) => (s.status?.daily_limit ? Math.max(0, s.status.daily_limit - s.status.used_today) : null),
  },
  actions: {
    async loadStatus() {
      try {
        this.status = await assistantApi.status()
      } catch {
        this.status = null
      }
    },
    async loadChats() {
      this.chats = await assistantApi.chats()
    },
    newChat() {
      this.stop()
      this.chatId = ''
      this.turns = []
      this.error = ''
    },
    async openChat(id: string) {
      this.stop()
      const c = await assistantApi.chat(id)
      this.chatId = c.id
      this.turns = c.turns ?? []
      this.error = ''
    },
    async removeChat(id: string) {
      await assistantApi.removeChat(id)
      this.chats = this.chats.filter((c) => c.id !== id)
      if (this.chatId === id) this.newChat()
    },
    async removeAll() {
      await assistantApi.removeAll()
      this.chats = []
      this.newChat()
    },
    setMode(m: AssistantMode) {
      this.mode = m
      try {
        localStorage.setItem(MODE_KEY, m)
      } catch {
        /* the choice just isn't remembered */
      }
    },
    // apply makes the ticked changes of the answer at turn i.
    async apply(i: number, approve: number[]) {
      if (!this.chatId || this.applying >= 0) return
      this.applying = i
      this.error = ''
      try {
        this.turns[i] = await assistantApi.apply(this.chatId, i, approve)
      } catch (e: any) {
        this.error = e?.message || String(e)
      } finally {
        this.applying = -1
      }
    },
    stop() {
      this.controller?.abort()
      this.controller = null
    },
    async send(message: string, context: AssistantContext) {
      if (this.sending || !message.trim()) return
      this.sending = true
      this.error = ''
      this.steps = []
      this.draft = ''
      const mode = this.mode
      this.turns.push({ role: 'user', text: message, mode })
      const ctl = new AbortController()
      this.controller = ctl
      const got: { answer?: AssistantTurn } = {}
      try {
        await assistantApi.ask({ chat_id: this.chatId || undefined, message, context, mode }, (e) => {
          if (e.type === 'step') {
            this.steps.push({ tool: e.tool, hint: e.hint })
            this.draft = ''
          } else if (e.type === 'delta') this.draft += e.text
          else if (e.type === 'answer') got.answer = { role: 'assistant', text: e.text, steps: [...this.steps], mode, actions: e.actions }
          else if (e.type === 'error') this.error = e.error
          else if (e.type === 'done') this.chatId = e.chat_id
        }, ctl.signal)
        if (got.answer) this.turns.push(got.answer)
      } catch (e: any) {
        if (e?.name !== 'AbortError') this.error = e?.message || String(e)
      } finally {
        this.sending = false
        this.steps = []
        this.draft = ''
        this.controller = null
        void this.loadStatus()
      }
    },
  },
})

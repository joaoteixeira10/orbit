import { defineStore } from 'pinia'
import { assistantApi, type AssistantChatSummary, type AssistantFile, type AssistantContext, type AssistantMode, type AssistantStatus, type AssistantStep, type AssistantTurn } from '@/api/assistant'

// MAX_FILES is how many files go with one message (as the server allows).
export const MAX_FILES = 10

// A file being attached for the next message: sent once it's up (and, a
// PDF, its text read here).
export interface Attaching {
  key: number
  name: string
  size: number
  file?: AssistantFile
  busy: boolean
  error: string
}
let seq = 0

const MODE_KEY = 'orbit.assistant.mode'
const BIG_KEY = 'orbit.assistant.big'
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
    big: (() => {
      try {
        return localStorage.getItem(BIG_KEY) === '1'
      } catch {
        return false
      }
    })(), // in the middle of the screen, large, rather than in the corner
    applying: -1, // the turn whose changes are being made
    askedAt: 0, // when the answer on its way was asked for (ms)
    attached: [] as Attaching[], // for the next message
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
    setBig(on: boolean) {
      this.big = on
      try {
        localStorage.setItem(BIG_KEY, on ? '1' : '')
      } catch {
        /* the choice just isn't remembered */
      }
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
    // attach uploads a file for the next message; a PDF's text is read here
    // (pdf.js), for the model.
    async attach(file: File) {
      this.attached.push({ key: ++seq, name: file.name, size: file.size, busy: true, error: '' })
      const it = this.attached[this.attached.length - 1]
      try {
        it.file = await assistantApi.attach(file)
        if (it.file.type === 'application/pdf') {
          try {
            const { pdfText } = await import('@/lib/fileView')
            const text = await pdfText(new Uint8Array(await file.arrayBuffer()))
            if (text.trim()) {
              await assistantApi.setFileText(it.file.id, text)
              it.file.readable = true
            }
          } catch {
            /* a scan, or one pdf.js can't read: it can still go in Files */
          }
        }
      } catch (e: any) {
        it.error = e?.message || String(e)
      } finally {
        it.busy = false
      }
    },
    detach(key: number) {
      const it = this.attached.find((a) => a.key === key)
      this.attached = this.attached.filter((a) => a.key !== key)
      if (it?.file) void assistantApi.dropFile(it.file.id).catch(() => {})
    },
    stop() {
      this.controller?.abort()
      this.controller = null
    },
    async send(message: string, context: AssistantContext) {
      if (this.sending || !message.trim() || this.attached.some((a) => a.busy)) return
      this.sending = true
      this.error = ''
      this.steps = []
      this.draft = ''
      const mode = this.mode
      this.askedAt = Date.now()
      const files = this.attached.filter((a) => a.file).map((a) => a.file!)
      this.attached = []
      this.turns.push({ role: 'user', text: message, mode, at: new Date().toISOString(), files: files.length ? files : undefined })
      const ctl = new AbortController()
      this.controller = ctl
      const got: { answer?: AssistantTurn } = {}
      try {
        await assistantApi.ask({ chat_id: this.chatId || undefined, message, context, mode, files: files.map((f) => f.id) }, (e) => {
          if (e.type === 'step') {
            this.steps.push({ tool: e.tool, hint: e.hint })
            this.draft = ''
          } else if (e.type === 'delta') this.draft += e.text
          else if (e.type === 'restart') this.draft = ''
          else if (e.type === 'answer') got.answer = { role: 'assistant', text: e.text, steps: [...this.steps], mode, actions: e.actions, at: new Date().toISOString(), took_ms: e.took_ms }
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

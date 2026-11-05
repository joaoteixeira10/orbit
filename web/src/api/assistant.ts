import { api, apiStream } from '@/lib/api'

// The built-in assistant: the owner's set-up, each person's conversations,
// and asking (an answer read as it comes: the tools it uses, then the text).

export type AssistantProvider = 'anthropic' | 'openai' | 'ollama'

export interface AssistantStatus {
  enabled: boolean
  provider?: AssistantProvider
  model?: string
  own: boolean // the model is my own (Settings → Assistant)
  own_allowed: boolean // the owner lets people use their own
  daily_limit: number // 0: none
  used_today: number
}

export interface AssistantConfig {
  enabled: boolean
  provider: AssistantProvider
  model: string
  url: string
  has_key: boolean
  daily_limit: number
  context: number // Ollama: how much the model reads, in tokens (0: 16384)
  allow_own: boolean // people may use their own model instead
}

// My own model for the assistant, when the owner lets people use their own.
export interface AssistantOwn {
  allowed: boolean
  instance: string // the instance's model, if it has one on
  use: boolean // mine, rather than the instance's
  provider: AssistantProvider
  model: string
  url: string
  has_key: boolean
  context: number
  servers: string[] // on Orbit's network, allowed by the operator (AI_SERVERS)
}

export interface AssistantStep {
  tool: string
  hint?: string
}

export type AssistantMode = 'ask' | 'agent'

// A change the assistant lined up in Agent mode, made only once approved.
export interface AssistantAction {
  n: number
  tool: string
  title: string
  summary: string
  destructive?: boolean
  args: Record<string, unknown>
  state: 'proposed' | 'done' | 'skipped' | 'failed'
  result?: string
  made_id?: string
}

// A file attached to a message: the person's alone, kept with the conversation.
export interface AssistantFile {
  id: string
  name: string
  type: string
  size: number
  url?: string
  readable?: boolean // the model can read what it says
}

export interface AssistantTurn {
  role: 'user' | 'assistant'
  text: string
  steps?: AssistantStep[]
  at?: string
  took_ms?: number // an answer: how long it took
  mode?: AssistantMode
  actions?: AssistantAction[]
  files?: AssistantFile[]
}

export interface AssistantChatSummary {
  id: string
  title: string
  updated_at: string
}

export interface AssistantChat {
  id: string
  title: string
  turns: AssistantTurn[]
}

export interface AssistantContext {
  path: string
  title: string
  workspace_id: string
  now: string
  time_zone: string
}

export type AssistantEvent =
  | { type: 'step'; tool: string; hint?: string }
  | { type: 'delta'; text: string }
  | { type: 'restart' }
  | { type: 'answer'; text: string; actions?: AssistantAction[]; took_ms?: number }
  | { type: 'done'; chat_id: string }
  | { type: 'error'; error: string }

const j = (b: unknown) => JSON.stringify(b)

export const assistantApi = {
  status: () => api<AssistantStatus>('/assistant'),
  config: () => api<AssistantConfig>('/assistant/config'),
  setConfig: (b: Partial<AssistantConfig> & { key?: string; clear_key?: boolean }) =>
    api<AssistantConfig>('/assistant/config', { method: 'PUT', body: j(b) }),
  own: () => api<AssistantOwn>('/me/assistant'),
  setOwn: (b: { use: boolean; provider: AssistantProvider; model: string; url: string; key?: string; clear_key?: boolean; context?: number }) =>
    api<AssistantOwn>('/me/assistant', { method: 'PUT', body: j(b) }),
  attach: (file: File) => {
    const form = new FormData()
    form.append('file', file, file.name || 'file')
    return api<AssistantFile>('/assistant/files', { method: 'POST', body: form })
  },
  // A PDF's text, read here (pdf.js), for the model to read.
  setFileText: (id: string, text: string) => api<void>(`/assistant/files/${id}/text`, { method: 'PUT', body: j({ text }) }),
  dropFile: (id: string) => api<void>(`/assistant/files/${id}`, { method: 'DELETE' }),
  chats: () => api<AssistantChatSummary[]>('/assistant/chats'),
  chat: (id: string) => api<AssistantChat>(`/assistant/chats/${id}`),
  removeChat: (id: string) => api<void>(`/assistant/chats/${id}`, { method: 'DELETE' }),
  removeAll: () => api<void>('/assistant/chats', { method: 'DELETE' }),
  // apply makes the changes ticked (by n) of the answer at turn; the rest aren't made.
  apply: (chatId: string, turn: number, approve: number[]) =>
    api<AssistantTurn>(`/assistant/chats/${chatId}/apply`, { method: 'POST', body: j({ turn, approve }) }),

  // ask sends a message and calls onEvent for each event of the answer.
  async ask(b: { chat_id?: string; message: string; context: AssistantContext; mode?: AssistantMode; files?: string[] }, onEvent: (e: AssistantEvent) => void, signal?: AbortSignal) {
    const res = await apiStream('/assistant/chat', { method: 'POST', body: j(b), signal })
    const reader = res.body!.getReader()
    const dec = new TextDecoder()
    let buf = ''
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buf += dec.decode(value, { stream: true })
      let i
      while ((i = buf.indexOf('\n\n')) >= 0) {
        const chunk = buf.slice(0, i)
        buf = buf.slice(i + 2)
        for (const line of chunk.split('\n')) {
          if (line.startsWith('data: ')) onEvent(JSON.parse(line.slice(6)) as AssistantEvent)
        }
      }
    }
  },
}

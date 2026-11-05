import { api, apiStream } from '@/lib/api'

// The built-in assistant: the owner's set-up, each person's conversations,
// and asking (an answer read as it comes: the tools it uses, then the text).

export type AssistantProvider = 'anthropic' | 'openai' | 'ollama'

export interface AssistantStatus {
  enabled: boolean
  provider?: AssistantProvider
  model?: string
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
}

export interface AssistantStep {
  tool: string
  hint?: string
}

export interface AssistantTurn {
  role: 'user' | 'assistant'
  text: string
  steps?: AssistantStep[]
  at?: string
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
  | { type: 'answer'; text: string }
  | { type: 'done'; chat_id: string }
  | { type: 'error'; error: string }

const j = (b: unknown) => JSON.stringify(b)

export const assistantApi = {
  status: () => api<AssistantStatus>('/assistant'),
  config: () => api<AssistantConfig>('/assistant/config'),
  setConfig: (b: Partial<AssistantConfig> & { key?: string; clear_key?: boolean }) =>
    api<AssistantConfig>('/assistant/config', { method: 'PUT', body: j(b) }),
  chats: () => api<AssistantChatSummary[]>('/assistant/chats'),
  chat: (id: string) => api<AssistantChat>(`/assistant/chats/${id}`),
  removeChat: (id: string) => api<void>(`/assistant/chats/${id}`, { method: 'DELETE' }),
  removeAll: () => api<void>('/assistant/chats', { method: 'DELETE' }),

  // ask sends a message and calls onEvent for each event of the answer.
  async ask(b: { chat_id?: string; message: string; context: AssistantContext }, onEvent: (e: AssistantEvent) => void, signal?: AbortSignal) {
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

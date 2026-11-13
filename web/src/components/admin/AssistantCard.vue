<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Sparkles } from 'lucide-vue-next'
import Toggle from '@/components/ui/Toggle.vue'
import { assistantApi, type AssistantConfig, type AssistantProvider } from '@/api/assistant'
import { useAssistantStore } from '@/stores/assistant'
import { useToastStore } from '@/stores/toast'

// Owner → Assistant: the instance's model — Claude with an API key, or an
// OpenAI-style server (OpenAI, Ollama…) at an address — and a daily limit per
// person. The key is never shown again; saving asks for the password again.
const { t } = useI18n()
const toast = useToastStore()
const assistant = useAssistantStore()
const cfg = ref<AssistantConfig | null>(null)
const form = reactive({ enabled: false, provider: 'anthropic' as AssistantProvider, model: '', url: '', key: '', clear_key: false, daily_limit: 0, context: 16384, allow_own: false })
const saving = ref(false)
const providers: AssistantProvider[] = ['anthropic', 'gemini', 'openai', 'ollama']
// Claude and Gemini have their own address; the others are a server you name.
const named = (p: AssistantProvider) => p === 'openai' || p === 'ollama'

function fill(c: AssistantConfig) {
  cfg.value = c
  Object.assign(form, { enabled: c.enabled, provider: c.provider || 'anthropic', model: c.model, url: c.url, key: '', clear_key: false, daily_limit: c.daily_limit, context: c.context || 16384, allow_own: c.allow_own })
}
onMounted(async () => fill(await assistantApi.config()))

function pickProvider(p: AssistantProvider) {
  form.provider = p
  form.model = p === 'anthropic' ? 'claude-opus-5-5' : p === 'gemini' ? 'gemini-flash-latest' : ''
  form.url = p === 'openai' ? 'https://api.openai.com/v1' : ''
}

async function save() {
  saving.value = true
  try {
    const context = form.provider === 'ollama' ? Math.round(form.context || 16384) : 0
    fill(await assistantApi.setConfig({ ...form, daily_limit: Math.max(0, Math.round(form.daily_limit || 0)), context }))
    toast.success(t('assistant.admin.saved'))
    void assistant.loadStatus()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card overflow-hidden" data-testid="assistant-admin">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3">
      <Sparkles class="h-[18px] w-[18px] text-accent" /><h3 class="text-[14px] font-semibold">{{ t('assistant.admin.title') }}</h3>
    </div>
    <form v-if="cfg" class="flex flex-col gap-4 p-5" @submit.prevent="save">
      <p class="text-[12px] text-text-3">{{ t('assistant.admin.hint') }}</p>
      <div class="flex items-center justify-between gap-4">
        <span class="text-[13px] font-semibold">{{ t('assistant.admin.on') }}</span>
        <Toggle v-model="form.enabled" :aria-label="t('assistant.admin.on')" data-testid="assistant-enabled" />
      </div>
      <label class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.provider') }}</span>
        <select class="input" :value="form.provider" data-testid="assistant-provider" @change="pickProvider(($event.target as HTMLSelectElement).value as AssistantProvider)">
          <option v-for="p in providers" :key="p" :value="p">{{ t(`assistant.admin.providers.${p}`) }}</option>
        </select>
      </label>
      <label class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.model') }}</span>
        <input v-model="form.model" class="input" spellcheck="false" autocomplete="off" data-testid="assistant-model" />
        <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.modelHint') }}</span>
      </label>
      <label v-if="named(form.provider)" class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.url') }}</span>
        <input v-model="form.url" class="input" spellcheck="false" autocomplete="off" :placeholder="form.provider === 'ollama' ? 'http://ollama.lan:11434' : 'https://api.openai.com/v1'" data-testid="assistant-url" />
        <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.urlHint') }}</span>
      </label>
      <label v-if="form.provider === 'ollama'" class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.context') }}</span>
        <input v-model.number="form.context" class="input !w-32" type="number" min="2048" step="1024" data-testid="assistant-context" />
        <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.contextHint') }}</span>
      </label>
      <label v-if="form.provider !== 'ollama'" class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.key') }}</span>
        <input v-model="form.key" class="input" type="password" autocomplete="new-password" :placeholder="cfg.has_key ? '••••••••' : ''" data-testid="assistant-key" />
        <span v-if="form.provider === 'gemini' && !cfg.has_key" class="text-[11.5px] text-text-3">{{ t('assistant.admin.geminiKey') }}</span>
      <span v-if="cfg.has_key" class="flex items-center justify-between text-[11.5px] text-text-3">
          {{ t('assistant.admin.keySet') }}
          <button type="button" class="text-crit-text hover:underline" @click="form.clear_key = true; form.key = ''">{{ t('assistant.admin.keyClear') }}</button>
        </span>
      </label>
      <label class="flex flex-col gap-1.5">
        <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.limit') }}</span>
        <input v-model.number="form.daily_limit" class="input !w-28" type="number" min="0" data-testid="assistant-limit" />
        <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.limitHint') }}</span>
      </label>
      <div class="flex items-start justify-between gap-4 border-t border-border pt-4">
        <span>
          <span class="block text-[13px] font-semibold">{{ t('assistant.admin.own') }}</span>
          <span class="mt-0.5 block text-[11.5px] text-text-3">{{ t('assistant.admin.ownHint') }}</span>
        </span>
        <Toggle v-model="form.allow_own" :aria-label="t('assistant.admin.own')" data-testid="assistant-allow-own" />
      </div>
      <div><button type="submit" class="btn-primary btn-sm" :disabled="saving" data-testid="assistant-save">{{ t('assistant.admin.save') }}</button></div>
    </form>
  </div>
</template>

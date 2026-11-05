<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/ui/Toggle.vue'
import { assistantApi, type AssistantOwn, type AssistantProvider } from '@/api/assistant'
import { useAssistantStore } from '@/stores/assistant'
import { useToastStore } from '@/stores/toast'

// Settings → Assistant: my own model — my Claude or OpenAI key, or my own
// Ollama — used instead of the instance's, when the owner lets people use
// their own. The key is never shown again.
const { t } = useI18n()
const toast = useToastStore()
const assistant = useAssistantStore()
const own = ref<AssistantOwn | null>(null)
const form = reactive({ use: false, provider: 'anthropic' as AssistantProvider, model: '', url: '', key: '', clear_key: false, context: 16384 })
const saving = ref(false)
const providers: AssistantProvider[] = ['anthropic', 'openai', 'ollama']

function fill(o: AssistantOwn) {
  own.value = o
  Object.assign(form, { use: o.use, provider: o.provider || 'anthropic', model: o.model || (o.provider ? '' : 'claude-opus-5-5'), url: o.url, key: '', clear_key: false, context: o.context || 16384 })
}
onMounted(async () => fill(await assistantApi.own()))

function pickProvider(p: AssistantProvider) {
  form.provider = p
  form.model = p === 'anthropic' ? 'claude-opus-5-5' : ''
  form.url = p === 'openai' ? 'https://api.openai.com/v1' : ''
}

const urlHint = computed(() =>
  own.value?.servers.length ? t('assistant.own.urlHintServers', { servers: own.value.servers.join(', ') }) : t('assistant.own.urlHint'),
)

async function save() {
  saving.value = true
  try {
    const context = form.provider === 'ollama' ? Math.round(form.context || 16384) : 0
    fill(await assistantApi.setOwn({ ...form, context }))
    toast.success(t('assistant.own.saved'))
    void assistant.loadStatus()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <form v-if="own" class="flex flex-col gap-4" data-testid="assistant-own" @submit.prevent="save">
    <p class="text-[12px] text-text-3">{{ t('assistant.own.hint') }}</p>
    <div class="flex items-start justify-between gap-4">
      <span>
        <span class="block text-[13px] font-semibold">{{ t('assistant.own.use') }}</span>
        <span class="mt-0.5 block text-[11.5px] text-text-3">{{ own.instance ? t('assistant.own.useInstance', { model: own.instance }) : t('assistant.own.noInstance') }}</span>
      </span>
      <Toggle v-model="form.use" :aria-label="t('assistant.own.use')" data-testid="assistant-own-use" />
    </div>
    <label class="flex flex-col gap-1.5">
      <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.provider') }}</span>
      <select class="input" :value="form.provider" data-testid="assistant-own-provider" @change="pickProvider(($event.target as HTMLSelectElement).value as AssistantProvider)">
        <option v-for="p in providers" :key="p" :value="p">{{ t(`assistant.admin.providers.${p}`) }}</option>
      </select>
    </label>
    <label class="flex flex-col gap-1.5">
      <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.model') }}</span>
      <input v-model="form.model" class="input" spellcheck="false" autocomplete="off" data-testid="assistant-own-model" />
      <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.modelHint') }}</span>
    </label>
    <label v-if="form.provider !== 'anthropic'" class="flex flex-col gap-1.5">
      <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.url') }}</span>
      <input v-model="form.url" class="input" spellcheck="false" autocomplete="off" list="assistant-own-servers" :placeholder="form.provider === 'ollama' ? 'https://ollama.example.org' : 'https://api.openai.com/v1'" data-testid="assistant-own-url" />
      <datalist id="assistant-own-servers"><option v-for="s in own.servers" :key="s" :value="s" /></datalist>
      <span class="text-[11.5px] text-text-3">{{ urlHint }}</span>
    </label>
    <label v-if="form.provider === 'ollama'" class="flex flex-col gap-1.5">
      <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.context') }}</span>
      <input v-model.number="form.context" class="input !w-32" type="number" min="2048" step="1024" />
      <span class="text-[11.5px] text-text-3">{{ t('assistant.admin.contextHint') }}</span>
    </label>
    <label v-if="form.provider !== 'ollama'" class="flex flex-col gap-1.5">
      <span class="text-[12px] font-medium text-text-2">{{ t('assistant.admin.key') }}</span>
      <input v-model="form.key" class="input" type="password" autocomplete="new-password" :placeholder="own.has_key ? '••••••••' : ''" data-testid="assistant-own-key" />
      <span v-if="own.has_key" class="flex items-center justify-between text-[11.5px] text-text-3">
        {{ t('assistant.admin.keySet') }}
        <button type="button" class="text-crit-text hover:underline" @click="form.clear_key = true; form.key = ''">{{ t('assistant.admin.keyClear') }}</button>
      </span>
    </label>
    <div><button type="submit" class="btn-primary btn-sm" :disabled="saving" data-testid="assistant-own-save">{{ t('assistant.admin.save') }}</button></div>
  </form>
</template>

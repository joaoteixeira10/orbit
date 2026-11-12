<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, KeyRound } from 'lucide-vue-next'
import Toggle from '@/components/ui/Toggle.vue'
import { api } from '@/lib/api'
import { useToastStore } from '@/stores/toast'

// Owner → SSO → Provisioning (SCIM): Okta or Microsoft Entra make, change and
// deactivate accounts and groups here as in their directory. The base URL
// and a token (shown once) go into the identity provider.
interface SCIMConfig {
  enabled: boolean
  has_token: boolean
  default_role: string
  url: string
  token?: string
}
const { t } = useI18n()
const toast = useToastStore()
const cfg = ref<SCIMConfig | null>(null)
const shown = ref('') // a new token, once
const busy = ref(false)

onMounted(async () => {
  try {
    cfg.value = await api<SCIMConfig>('/scim-config')
  } catch (e: any) {
    toast.error(e.message)
  }
})

async function save(newToken = false) {
  const c = cfg.value
  if (!c) return
  busy.value = true
  try {
    const r = await api<SCIMConfig>('/scim-config', { method: 'PUT', body: JSON.stringify({ enabled: c.enabled, default_role: c.default_role, new_token: newToken }) })
    cfg.value = { ...r, token: undefined }
    if (r.token) shown.value = r.token
    toast.success(t('scim.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
async function copy(s: string) {
  await navigator.clipboard?.writeText(s).catch(() => {})
  toast.info(t('scim.copied'))
}
</script>

<template>
  <div v-if="cfg" class="card flex flex-col gap-3 p-5" data-testid="scim-settings">
    <div class="flex items-center justify-between gap-4">
      <div>
        <h3 class="text-[14px] font-semibold">{{ t('scim.title') }}</h3>
        <div class="text-[12px] text-text-3">{{ t('scim.hint') }}</div>
      </div>
      <Toggle v-model="cfg.enabled" data-testid="scim-on" @update:model-value="save()" />
    </div>
    <div>
      <label class="label">{{ t('scim.url') }}</label>
      <div class="flex gap-2">
        <input :value="cfg.url" class="input font-mono text-[12.5px]" readonly data-testid="scim-url" />
        <button type="button" class="btn-ghost shrink-0" :title="t('scim.copy')" :aria-label="t('scim.copy')" @click="copy(cfg.url)"><Copy class="h-4 w-4" /></button>
      </div>
    </div>
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label class="label">{{ t('scim.defaultRole') }}</label>
        <input v-model="cfg.default_role" class="input" placeholder="none" @change="save()" />
      </div>
      <div>
        <label class="label">{{ t('scim.token') }}</label>
        <button type="button" class="btn w-full" :disabled="busy" data-testid="scim-new-token" @click="save(true)">
          <KeyRound class="h-4 w-4" />{{ cfg.has_token ? t('scim.newToken') : t('scim.makeToken') }}
        </button>
      </div>
    </div>
    <div v-if="shown" class="rounded-xl bg-warn-bg px-3 py-2 text-[12.5px] text-warn-text" data-testid="scim-token">
      {{ t('scim.once') }}
      <div class="mt-1.5 flex gap-2">
        <code class="min-w-0 flex-1 break-all rounded bg-surface px-2 py-1 font-mono text-[12px] text-text">{{ shown }}</code>
        <button type="button" class="btn-ghost shrink-0" :aria-label="t('scim.copy')" @click="copy(shown)"><Copy class="h-4 w-4" /></button>
      </div>
    </div>
    <p class="text-[11px] text-text-3">{{ t('scim.rules') }}</p>
  </div>
</template>

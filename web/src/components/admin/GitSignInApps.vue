<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Trash2 } from 'lucide-vue-next'
import CopyButton from '@/components/ui/CopyButton.vue'
import { gitApi } from '@/api/planner'
import { useToastStore } from '@/stores/toast'

// Owner → the servers people can "Sign in with": Orbit's OAuth2 application
// on each (client id and secret; the secret is never shown again).
const { t } = useI18n()
const toast = useToastStore()

interface Row {
  base_url: string
  client_id: string
  client_secret: string
  has_secret: boolean
  kind: string // gitea | github | gitlab ("" from the address)
}
const rows = ref<Row[]>([])
const redirect = ref('')
const busy = ref(false)

function take(r: { apps: { base_url: string; client_id: string; has_secret: boolean; kind?: string }[]; redirect_uri: string }) {
  rows.value = r.apps.map((a) => ({ ...a, kind: a.kind ?? '', client_secret: '' }))
  redirect.value = r.redirect_uri
}
onMounted(async () => {
  try {
    take(await gitApi.oauthApps())
  } catch (e: any) {
    toast.error(e.message)
  }
})

async function save() {
  busy.value = true
  try {
    take(await gitApi.setOAuthApps(rows.value.filter((r) => r.base_url.trim()).map((r) => ({ base_url: r.base_url.trim(), client_id: r.client_id.trim(), client_secret: r.client_secret.trim() || undefined, kind: r.kind || undefined }))))
    toast.success(t('git.settings.apps.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="rounded-xl bg-fill-2 p-3" data-testid="git-oauth-apps">
    <div class="text-[12.5px] font-semibold">{{ t('git.settings.apps.title') }}</div>
    <p class="mt-0.5 text-[12px] text-text-3">{{ t('git.settings.apps.hint') }}</p>
    <div class="mt-2 flex items-center gap-2">
      <span class="shrink-0 text-[12px] text-text-2">{{ t('git.settings.apps.redirect') }}</span>
      <code class="min-w-0 flex-1 truncate rounded bg-surface px-2 py-1 text-[12px]">{{ redirect }}</code>
      <CopyButton :value="redirect" />
    </div>
    <div v-for="(r, i) in rows" :key="i" class="mt-3 space-y-1.5 rounded-lg bg-surface p-2.5">
      <div class="flex gap-2">
        <input v-model="r.base_url" class="input min-w-0 flex-1 !py-1 text-[13px]" :placeholder="'https://gitea.example.org'" :aria-label="t('git.settings.apps.server')" />
        <select v-model="r.kind" class="input !w-auto !py-1 text-[13px]" :aria-label="t('git.settings.apps.kind')">
          <option value="">{{ t('git.settings.apps.kindAuto') }}</option>
          <option value="gitea">Gitea / Forgejo</option>
          <option value="gitlab">GitLab</option>
          <option value="github">GitHub</option>
        </select>
        <button type="button" class="icon-btn !text-danger" :title="t('git.settings.apps.remove')" :aria-label="t('git.settings.apps.remove')" @click="rows.splice(i, 1)"><Trash2 class="h-4 w-4" /></button>
      </div>
      <input v-model="r.client_id" class="input !py-1 font-mono text-[12.5px]" :placeholder="t('git.settings.apps.clientId')" :aria-label="t('git.settings.apps.clientId')" autocomplete="off" />
      <input v-model="r.client_secret" type="password" class="input !py-1 font-mono text-[12.5px]" :placeholder="r.has_secret ? t('git.settings.apps.secretKept') : t('git.settings.apps.secret')" :aria-label="t('git.settings.apps.secret')" autocomplete="off" />
    </div>
    <div class="mt-3 flex justify-between gap-2">
      <button type="button" class="btn btn-sm" @click="rows.push({ base_url: '', client_id: '', client_secret: '', has_secret: false, kind: '' })"><Plus class="h-4 w-4" />{{ t('git.settings.apps.add') }}</button>
      <button type="button" class="btn-primary btn-sm" :disabled="busy" @click="save">{{ t('git.settings.apps.save') }}</button>
    </div>
  </div>
</template>

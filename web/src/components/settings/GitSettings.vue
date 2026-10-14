<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n, I18nT } from 'vue-i18n'
import { GitBranch, Trash2, ExternalLink, TriangleAlert } from 'lucide-vue-next'
import Confirm from '@/components/ui/Confirm.vue'
import { gitApi, type GitAccount } from '@/api/planner'
import { useToastStore } from '@/stores/toast'

// Settings → Git: the caller's accounts on Gitea / Forgejo servers, each a
// personal access token checked against the server when added (and sealed
// on ours). Connecting the same server again replaces its token.
const toast = useToastStore()
const { t } = useI18n()

const accounts = ref<GitAccount[]>([])
const servers = ref<string[]>([])
const base = ref('')
const token = ref('')
const busy = ref(false)
const removing = ref<GitAccount | null>(null)

onMounted(async () => {
  try {
    const r = await gitApi.accounts()
    accounts.value = r.accounts
    servers.value = r.servers
    if (!base.value && r.servers.length === 1 && !r.accounts.length) base.value = r.servers[0]
  } catch (e: any) {
    toast.error(e.message)
  }
})

// Where to make a token: the server's own page, once its address is typed.
const tokenPage = computed(() => {
  try {
    const u = new URL(base.value.trim())
    return /^https?:$/.test(u.protocol) ? `${u.origin}${u.pathname.replace(/\/+$/, '')}/user/settings/applications` : ''
  } catch {
    return ''
  }
})
const canConnect = computed(() => !busy.value && !!base.value.trim() && !!token.value.trim())

async function connect() {
  if (!canConnect.value) return
  busy.value = true
  try {
    const a = await gitApi.connect(base.value.trim(), token.value.trim())
    accounts.value = [...accounts.value.filter((x) => x.id !== a.id), a].sort((x, y) => x.base_url.localeCompare(y.base_url))
    token.value = ''
    base.value = ''
    toast.success(t('git.settings.connected', { server: host(a.base_url), login: a.login }))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function disconnect() {
  const a = removing.value
  if (!a) return
  try {
    await gitApi.disconnect(a.id)
    accounts.value = accounts.value.filter((x) => x.id !== a.id)
    removing.value = null
    toast.success(t('git.settings.disconnected'))
  } catch (e: any) {
    toast.error(e.message)
  }
}

function host(u: string) {
  return u.replace(/^https?:\/\//, '')
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="text-[12.5px] text-text-3">{{ t('git.settings.hint') }}</p>

    <div v-if="accounts.length" class="group">
      <div v-for="a in accounts" :key="a.id" class="group-row">
        <span class="tile h-7 w-7" style="--tile: #f05032"><GitBranch class="h-4 w-4" /></span>
        <div class="min-w-0 flex-1">
          <a :href="a.base_url" target="_blank" rel="noopener noreferrer" class="block truncate text-[13.5px] font-medium hover:underline">{{ host(a.base_url) }}</a>
          <div class="truncate text-[12px] text-text-3">{{ t('git.settings.as', { login: a.login }) }}</div>
          <div v-if="a.last_error" class="flex items-center gap-1 text-[12px] text-crit-text"><TriangleAlert class="h-3 w-3" />{{ a.last_error }}</div>
        </div>
        <button class="icon-btn !text-danger" :title="t('git.settings.disconnect')" :aria-label="t('git.settings.disconnect')" @click="removing = a">
          <Trash2 class="h-4 w-4" />
        </button>
      </div>
    </div>
    <p v-else class="text-[13px] text-text-3">{{ t('git.settings.none') }}</p>

    <form class="flex flex-col gap-3 rounded-xl bg-fill-2 p-4" @submit.prevent="connect">
      <label class="block">
        <span class="mb-1 block text-[12.5px] font-medium text-text-2">{{ t('git.settings.server') }}</span>
        <input v-model="base" class="input" type="url" list="git-servers" :placeholder="servers[0] || t('git.settings.serverPlaceholder')" autocomplete="url" />
        <datalist id="git-servers">
          <option v-for="s in servers" :key="s" :value="s" />
        </datalist>
      </label>
      <label class="block">
        <span class="mb-1 block text-[12.5px] font-medium text-text-2">{{ t('git.settings.token') }}</span>
        <input v-model="token" class="input font-mono" type="password" autocomplete="off" spellcheck="false" />
        <I18nT keypath="git.settings.tokenHint" tag="span" scope="global" class="mt-1 block text-[12px] leading-snug text-text-3">
          <template #link>
            <a v-if="tokenPage" :href="tokenPage" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-0.5 text-accent-text hover:underline">{{ t('git.settings.tokenPage') }}<ExternalLink class="h-3 w-3" /></a>
            <span v-else>{{ t('git.settings.tokenPageGeneric') }}</span>
          </template>
        </I18nT>
      </label>
      <div class="flex justify-end">
        <button type="submit" class="btn-primary" :disabled="!canConnect">{{ t('git.settings.connect') }}</button>
      </div>
    </form>
  </div>

  <Confirm
    :open="!!removing"
    :title="t('git.settings.disconnectTitle', { server: removing ? host(removing.base_url) : '' })"
    :message="t('git.settings.disconnectBody')"
    :confirm-label="t('git.settings.disconnect')"
    danger
    @confirm="disconnect"
    @close="removing = null"
  />
</template>

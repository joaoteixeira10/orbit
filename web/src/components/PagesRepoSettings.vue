<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { GitBranch, RefreshCw } from '@lucide/vue'
import { pagesRepoApi, type PagesRepo, type PagesRepoAccount } from '@/lib/shareApi'
import { useToastStore } from '@/stores/toast'
import { fmtDate } from '@/lib/format'
import Toggle from '@/components/ui/Toggle.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'
import { NativeSelect } from '@/components/shadcn/native-select'

// Workspace settings → Publish → Git (owners): the workspace's pages copied
// into a folder of a git repository as Markdown files, one way — written as
// the owner's own account on that server (Settings → Git).
const props = defineProps<{ wsId: string }>()
const { t } = useI18n()
const toast = useToastStore()

const repo = ref<PagesRepo | null>(null)
const accounts = ref<PagesRepoAccount[]>([])
const loaded = ref(false)
const host = ref('')
const path = ref('')
const branch = ref('main')
const folder = ref('')
const auto = ref(true)
const saving = ref(false)
const copying = ref(false)
const stopping = ref(false)

function take(r: { repo: PagesRepo | null; accounts: PagesRepoAccount[] }) {
  repo.value = r.repo
  accounts.value = r.accounts
  host.value = r.repo?.host ?? r.accounts[0]?.base_url ?? ''
  path.value = r.repo ? `${r.repo.owner}/${r.repo.repo}` : ''
  branch.value = r.repo?.branch || 'main'
  folder.value = r.repo?.folder ?? ''
  auto.value = r.repo?.auto ?? true
}
onMounted(async () => {
  try {
    take(await pagesRepoApi.get(props.wsId))
  } catch (e: any) {
    toast.error(e.message)
  }
  loaded.value = true
})
const repoUrl = computed(() => (repo.value ? `${repo.value.host}/${repo.value.owner}/${repo.value.repo}` : ''))

async function save() {
  if (saving.value || !host.value || !path.value.trim()) return
  saving.value = true
  try {
    take(await pagesRepoApi.save(props.wsId, { host: host.value, repo: path.value.trim(), branch: branch.value.trim(), folder: folder.value.trim(), auto: auto.value }))
    toast.success(t('pagesRepo.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
async function copyNow() {
  if (copying.value) return
  copying.value = true
  try {
    take(await pagesRepoApi.copy(props.wsId))
    toast.success(t('pagesRepo.copied'))
  } catch (e: any) {
    toast.error(e.message)
    try {
      take(await pagesRepoApi.get(props.wsId))
    } catch {
      /* as it was */
    }
  } finally {
    copying.value = false
  }
}
async function stop() {
  stopping.value = false
  try {
    await pagesRepoApi.remove(props.wsId)
    take({ repo: null, accounts: accounts.value })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="space-y-4" data-testid="pages-repo">
    <div class="flex items-center gap-2"><GitBranch class="h-4 w-4 text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('pagesRepo.title') }}</h3></div>
    <p class="text-[13px] leading-relaxed text-text-2">{{ t('pagesRepo.intro') }}</p>
    <template v-if="loaded">
      <p v-if="!accounts.length" class="text-[13px] text-text-3">{{ t('pagesRepo.noAccount') }}</p>
      <template v-else>
        <div v-if="repo" class="rounded-xl border border-border px-3 py-2.5 text-[13px]">
          <div class="flex items-center gap-2">
            <a :href="repoUrl" target="_blank" rel="noopener" class="min-w-0 flex-1 truncate text-accent hover:underline">{{ repo.owner }}/{{ repo.repo }} · {{ repo.branch }}{{ repo.folder ? ` · ${repo.folder}/` : '' }}</a>
            <Button variant="ghost" size="sm" :disabled="copying" data-testid="pages-repo-copy" @click="copyNow"><RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': copying }" />{{ t('pagesRepo.copyNow') }}</Button>
          </div>
          <p class="mt-1 text-[12px] text-text-3">{{ repo.copied_at ? t('pagesRepo.copiedAt', { when: fmtDate(repo.copied_at) }) : t('pagesRepo.never') }}</p>
          <p v-if="repo.last_error" class="mt-1 text-[12px] text-crit-text">{{ repo.last_error }}</p>
        </div>
        <label class="block">
          <span class="text-[13px] font-semibold">{{ t('pagesRepo.server') }}</span>
          <NativeSelect v-model="host" class="mt-1.5 w-full" data-testid="pages-repo-host">
            <option v-for="a in accounts" :key="a.base_url" :value="a.base_url">{{ a.base_url }} ({{ a.login }})</option>
          </NativeSelect>
        </label>
        <label class="block">
          <span class="text-[13px] font-semibold">{{ t('pagesRepo.repo') }}</span>
          <Input v-model="path" class="mt-1.5" placeholder="owner/name" spellcheck="false" autocapitalize="off" data-testid="pages-repo-repo" />
        </label>
        <div class="grid grid-cols-2 gap-3">
          <label class="block">
            <span class="text-[13px] font-semibold">{{ t('pagesRepo.branch') }}</span>
            <Input v-model="branch" class="mt-1.5" placeholder="main" spellcheck="false" autocapitalize="off" />
          </label>
          <label class="block">
            <span class="text-[13px] font-semibold">{{ t('pagesRepo.folder') }}</span>
            <Input v-model="folder" class="mt-1.5" placeholder="docs" spellcheck="false" autocapitalize="off" />
          </label>
        </div>
        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-[13px] font-semibold">{{ t('pagesRepo.auto') }}</div>
            <div class="text-[12px] text-text-3">{{ t('pagesRepo.autoHint') }}</div>
          </div>
          <Toggle v-model="auto" />
        </div>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" :disabled="saving || !path.trim()" data-testid="pages-repo-save" @click="save">{{ t('common.save') }}</Button>
          <Button v-if="repo" variant="ghost" size="sm" class="text-danger!" @click="stopping = true">{{ t('pagesRepo.stop') }}</Button>
        </div>
      </template>
    </template>
    <Confirm :open="stopping" :title="t('pagesRepo.stopTitle')" :message="t('pagesRepo.stopBody')" :confirm-label="t('pagesRepo.stop')" danger @confirm="stop" @close="stopping = false" />
  </div>
</template>

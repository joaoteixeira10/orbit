<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { GitBranch, Search, Lock, Check, LoaderCircle } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import { gitApi, type GitAccount, type GitBoards, type GitLink, type GitRepo, type TaskList } from '@/api/planner'
import { useTasksStore } from '@/stores/tasks'
import { useToastStore } from '@/stores/toast'

// A new task list that holds a repository's issues: pick the account, the
// repository, and how its issues are laid out — open / closed, labels as
// columns, or one of its project boards. The server makes the columns and
// brings the issues in.
const props = defineProps<{ open: boolean; ws: string }>()
const emit = defineEmits<{ close: []; saved: [list: TaskList] }>()
const tasks = useTasksStore()
const toast = useToastStore()
const { t } = useI18n()

const accounts = ref<GitAccount[] | null>(null)
const accountId = ref('')
const query = ref('')
const repos = ref<GitRepo[]>([])
const searching = ref(false)
const repo = ref<GitRepo | null>(null)
const boards = ref<GitBoards | null>(null)
const mode = ref<GitLink['mode']>('issues')
const labels = ref<string[]>([]) // picked, in order
const projectId = ref(0)
const name = ref('')
const busy = ref(false)

watch(
  () => props.open,
  async (o) => {
    if (!o) return
    repo.value = null
    boards.value = null
    query.value = ''
    mode.value = 'issues'
    labels.value = []
    name.value = ''
    busy.value = false
    try {
      const r = await gitApi.accounts()
      accounts.value = r.accounts
      if (!r.accounts.some((a) => a.id === accountId.value)) accountId.value = r.accounts[0]?.id ?? ''
    } catch (e: any) {
      accounts.value = []
      toast.error(e.message)
    }
  },
  { immediate: true },
)

// Repositories as the search is typed.
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
watch([query, accountId], () => {
  clearTimeout(timer)
  if (!accountId.value || repo.value) return
  timer = setTimeout(search, 250)
}, { immediate: true })
async function search() {
  const mine = ++seq
  searching.value = true
  try {
    const r = await gitApi.repos(accountId.value, query.value.trim())
    if (mine === seq) repos.value = r
  } catch (e: any) {
    if (mine === seq) toast.error(e.message)
  } finally {
    if (mine === seq) searching.value = false
  }
}

async function choose(r: GitRepo) {
  repo.value = r
  name.value = r.name
  boards.value = null
  try {
    boards.value = await gitApi.boards(accountId.value, r.owner.login, r.name)
    projectId.value = boards.value.projects[0]?.id ?? 0
  } catch (e: any) {
    toast.error(e.message)
  }
}
function change() {
  repo.value = null
  boards.value = null
  labels.value = []
  mode.value = 'issues'
  void search()
}

function toggleLabel(n: string) {
  labels.value = labels.value.includes(n) ? labels.value.filter((x) => x !== n) : [...labels.value, n]
}
const projectsOk = computed(() => !!boards.value?.supported && boards.value.projects.length > 0)

const canCreate = computed(() => {
  if (busy.value || !repo.value || !name.value.trim()) return false
  if (mode.value === 'labels') return labels.value.length > 0
  if (mode.value === 'project') return projectsOk.value && !!projectId.value
  return true
})

async function create() {
  if (!canCreate.value || !repo.value) return
  busy.value = true
  try {
    const l = await gitApi.createList(props.ws, {
      account_id: accountId.value,
      owner: repo.value.owner.login,
      repo: repo.value.name,
      mode: mode.value,
      labels: mode.value === 'labels' ? labels.value : undefined,
      project_id: mode.value === 'project' ? projectId.value : undefined,
      name: name.value.trim(),
    })
    await tasks.load(props.ws)
    toast.success(t('git.list.created', { repo: repo.value.full_name }))
    emit('saved', l)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

// GitHub's project boards have no REST API (only GraphQL): labels there.
const github = computed(() => accounts.value?.find((a) => a.id === accountId.value)?.kind === 'github')
const modes = computed(() => [
  { key: 'issues' as const, label: t('git.list.modeIssues'), hint: t('git.list.modeIssuesHint') },
  { key: 'labels' as const, label: t('git.list.modeLabels'), hint: t('git.list.modeLabelsHint') },
  ...(github.value ? [] : [{ key: 'project' as const, label: t('git.list.modeProject'), hint: t('git.list.modeProjectHint') }]),
])
</script>

<template>
  <Overlay :open="open" :title="t('git.list.title')" size="md" @close="emit('close')">
    <div v-if="accounts === null" class="grid place-items-center py-10 text-text-3"><LoaderCircle class="h-5 w-5 animate-spin" /></div>

    <!-- No account yet -->
    <div v-else-if="!accounts.length" class="flex flex-col items-center gap-3 py-6 text-center">
      <span class="tile h-12 w-12" style="--tile: #f05032"><GitBranch class="h-6 w-6" /></span>
      <p class="max-w-[320px] text-[14px] text-text-2">{{ t('git.list.needAccount') }}</p>
      <RouterLink :to="{ name: 'settings', hash: '#git' }" class="btn-primary" @click="emit('close')">{{ t('git.list.openSettings') }}</RouterLink>
    </div>

    <div v-else class="space-y-5">
      <label v-if="accounts.length > 1" class="block">
        <span class="cap mb-1.5 block px-1">{{ t('git.list.account') }}</span>
        <select v-model="accountId" class="input" :disabled="!!repo">
          <option v-for="a in accounts" :key="a.id" :value="a.id">{{ a.login }} · {{ a.base_url.replace(/^https?:\/\//, '') }}</option>
        </select>
      </label>

      <!-- Repository -->
      <section>
        <h3 class="cap mb-1.5 px-1">{{ t('git.list.repo') }}</h3>
        <div v-if="repo" class="group">
          <div class="group-row">
            <span class="tile h-7 w-7" style="--tile: #f05032"><GitBranch class="h-4 w-4" /></span>
            <div class="min-w-0 flex-1">
              <div class="truncate text-[14px] font-medium">{{ repo.full_name }}</div>
              <div v-if="repo.description" class="truncate text-[12px] text-text-3">{{ repo.description }}</div>
            </div>
            <button type="button" class="btn-plain" @click="change">{{ t('git.list.change') }}</button>
          </div>
        </div>
        <template v-else>
          <div class="search-box">
            <Search class="h-4 w-4 text-text-3" />
            <input v-model="query" class="min-w-0 flex-1 bg-transparent text-[14px] outline-none" :placeholder="t('git.list.searchRepo')" :aria-label="t('git.list.searchRepo')" autofocus />
            <LoaderCircle v-if="searching" class="h-4 w-4 animate-spin text-text-3" />
          </div>
          <div v-if="repos.length" class="group mt-2 max-h-[264px] overflow-y-auto">
            <button v-for="r in repos" :key="r.id" type="button" class="group-row w-full text-left hover:bg-fill" @click="choose(r)">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5 truncate text-[14px]">{{ r.full_name }}<Lock v-if="r.private" class="h-3 w-3 shrink-0 text-text-3" /></div>
                <div v-if="r.description" class="truncate text-[12px] text-text-3">{{ r.description }}</div>
              </div>
            </button>
          </div>
          <p v-else-if="!searching" class="mt-2 px-1 text-[13px] text-text-3">{{ t('git.list.noRepos') }}</p>
        </template>
      </section>

      <!-- Layout -->
      <section v-if="repo">
        <h3 class="cap mb-1.5 px-1">{{ t('git.list.layout') }}</h3>
        <div class="group">
          <button
            v-for="m in modes"
            :key="m.key"
            type="button"
            class="group-row w-full !items-start text-left"
            :aria-pressed="mode === m.key"
            @click="mode = m.key"
          >
            <span class="radio mt-0.5" :class="{ on: mode === m.key }"><Check class="h-3 w-3" :stroke-width="3" /></span>
            <span class="min-w-0 flex-1">
              <span class="block text-[14px]">{{ m.label }}</span>
              <span class="block text-[12px] leading-snug text-text-3">{{ m.hint }}</span>
            </span>
          </button>
        </div>
        <p v-if="github" class="mt-2 px-1 text-[12px] leading-snug text-text-3">{{ t('git.list.githubNote') }}</p>

        <div v-if="mode === 'labels' && boards" class="mt-3">
          <p class="mb-1.5 px-1 text-[12.5px] text-text-3">{{ t('git.list.pickLabels') }}</p>
          <div class="flex flex-wrap gap-1.5 px-1">
            <button
              v-for="l in boards.labels"
              :key="l.id"
              type="button"
              class="label-chip"
              :class="{ on: labels.includes(l.name) }"
              :style="{ '--label': l.color.startsWith('#') ? l.color : `#${l.color}` }"
              :aria-pressed="labels.includes(l.name)"
              @click="toggleLabel(l.name)"
            >
              <span v-if="labels.includes(l.name)" class="order">{{ labels.indexOf(l.name) + 1 }}</span>
              <span v-else class="dot" />{{ l.name }}
            </button>
          </div>
        </div>

        <div v-if="mode === 'project' && boards" class="mt-3 px-1">
          <p v-if="!boards.supported" class="text-[12.5px] text-warn-text">{{ t('git.list.projectsUnsupported') }}</p>
          <p v-else-if="!boards.projects.length" class="text-[12.5px] text-text-3">{{ t('git.list.noProjects') }}</p>
          <label v-else class="flex items-center gap-3">
            <span class="text-[13px] text-text-2">{{ t('git.list.pickProject') }}</span>
            <select v-model.number="projectId" class="input !w-auto !py-1 text-[13px]">
              <option v-for="p in boards.projects" :key="p.id" :value="p.id">{{ p.title }}</option>
            </select>
          </label>
        </div>
      </section>

      <label v-if="repo" class="block">
        <span class="cap mb-1.5 block px-1">{{ t('git.list.name') }}</span>
        <input v-model="name" class="input" maxlength="100" @keydown.enter.prevent="create" />
      </label>
    </div>

    <template #footer>
      <button type="button" class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button v-if="accounts?.length" type="button" class="btn-primary" :disabled="!canCreate" @click="create">
        <LoaderCircle v-if="busy" class="h-4 w-4 animate-spin" />{{ t('git.list.create') }}
      </button>
    </template>
  </Overlay>
</template>

<style scoped>
.search-box {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-radius: 10px;
  background: var(--fill-2);
}
.search-box:focus-within {
  box-shadow: 0 0 0 3px var(--ring);
}
.radio {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  border: 1.5px solid var(--text-faint);
  color: transparent;
}
.radio.on {
  border-color: var(--accent);
  background: var(--accent);
  color: #fff;
}
.label-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 8px;
  border-radius: 999px;
  font-size: 13px;
  background: var(--fill-2);
  color: var(--text-2);
  transition: background var(--speed) var(--ease);
}
.label-chip:hover {
  background: var(--fill);
}
.label-chip.on {
  background: color-mix(in srgb, var(--label) 18%, transparent);
  color: var(--text);
}
.label-chip .dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--label);
}
.label-chip .order {
  display: grid;
  place-items: center;
  width: 16px;
  height: 16px;
  border-radius: 999px;
  background: var(--label);
  color: #fff;
  font-size: 10.5px;
  font-weight: 700;
}
</style>

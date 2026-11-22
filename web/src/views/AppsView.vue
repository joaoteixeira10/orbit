<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { AppWindow, ArrowLeft, ArrowRight, ExternalLink, LoaderCircle, MoreHorizontal, Pencil, Plus, RotateCw, ShieldOff, Trash2 } from '@lucide/vue'
import Popover from '@/components/ui/Popover.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { appsApi, hostOf, type AppCheck, type WorkspaceApp } from '@/lib/apps'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'

// Apps: other sites pinned in this workspace, each a tab shown full-size in
// a frame. Only the owner's allowed sites; a site that refuses to be shown
// inside another (its X-Frame-Options / frame-ancestors) is said in words,
// with a way to open it in a tab of its own. Tabs already opened stay
// loaded, so going back to one doesn't load it again.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const apps = ref<WorkspaceApp[]>([])
const sites = ref<string[]>([])
const loaded = ref(false)
async function load() {
  try {
    const [list, s] = await Promise.all([appsApi.list(ws.value), appsApi.sites()])
    apps.value = list
    sites.value = s.sites
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
const visited = reactive(new Set<string>())
watch(ws, () => {
  loaded.value = false
  visited.clear()
  void load()
}, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && e.what === 'apps') void load()
})
onUnmounted(off)

// The tab shown: the address's, else the first.
const active = computed(() => apps.value.find((a) => a.id === route.params.id) ?? apps.value[0])
function show(a: WorkspaceApp) {
  if (a.id !== active.value?.id) void router.replace({ name: 'ws-apps', params: { ws: ws.value, id: a.id } })
}

// Whether each site lets itself be shown here (asked once per address).
const checks = reactive<Record<string, { url: string; res: AppCheck | null }>>({})
function check(a: WorkspaceApp) {
  if (!a.allowed || checks[a.id]?.url === a.url) return
  checks[a.id] = { url: a.url, res: null }
  appsApi
    .check(a.id)
    .then((res) => (checks[a.id] = { url: a.url, res }))
    .catch(() => (checks[a.id] = { url: a.url, res: { frameable: 'unknown' } }))
}
watch(active, (a) => {
  if (!a) return
  visited.add(a.id)
  check(a)
}, { immediate: true })
const state = (a: WorkspaceApp): 'blocked' | 'checking' | 'refused' | 'frame' => {
  if (!a.allowed) return 'blocked'
  const c = checks[a.id]
  if (!c || c.url !== a.url || !c.res) return 'checking'
  return c.res.frameable === 'no' ? 'refused' : 'frame'
}
const shownFrames = computed(() => apps.value.filter((a) => visited.has(a.id) && state(a) === 'frame'))
const reloads = reactive<Record<string, number>>({})
function reload(a: WorkspaceApp) {
  reloads[a.id] = (reloads[a.id] ?? 0) + 1
  delete checks[a.id]
  check(a)
}

// The frame fills the window below the tabs.
const stage = ref<HTMLElement | null>(null)
const top = ref(0)
function measure() {
  if (stage.value) top.value = Math.round(stage.value.getBoundingClientRect().top + window.scrollY)
}
// Measured again when the header wraps or the first app appears (hidden,
// it has no place).
const head = ref<HTMLElement | null>(null)
let ro: ResizeObserver | null = null
onMounted(() => {
  window.addEventListener('resize', measure)
  ro = new ResizeObserver(measure)
  if (head.value) ro.observe(head.value)
  void nextTick(measure)
})
onUnmounted(() => {
  window.removeEventListener('resize', measure)
  ro?.disconnect()
})
watch([loaded, () => apps.value.length], () => void nextTick(measure))

// Adding and editing.
const editing = ref<WorkspaceApp | 'new' | null>(null)
const form = reactive({ name: '', url: '' })
const saving = ref(false)
function openAdd() {
  Object.assign(form, { name: '', url: '' })
  editing.value = 'new'
}
function openEdit(a: WorkspaceApp) {
  menu.value = null
  Object.assign(form, { name: a.name, url: a.url })
  editing.value = a
}
async function save() {
  if (!form.url.trim() || saving.value) return
  saving.value = true
  try {
    if (editing.value === 'new') {
      const a = await appsApi.create(ws.value, { name: form.name.trim(), url: form.url.trim() })
      apps.value = [...apps.value, a]
      show(a)
    } else if (editing.value) {
      const a = await appsApi.update(editing.value.id, { name: form.name.trim(), url: form.url.trim() })
      apps.value = apps.value.map((x) => (x.id === a.id ? a : x))
    }
    editing.value = null
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}

// The tab's menu: edit, move, remove.
const menu = ref<{ a: WorkspaceApp; el: HTMLElement } | null>(null)
async function move(a: WorkspaceApp, by: number) {
  menu.value = null
  const list = [...apps.value]
  const i = list.findIndex((x) => x.id === a.id)
  const j = i + by
  if (i < 0 || j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j], list[i]]
  apps.value = list
  try {
    await appsApi.order(ws.value, list.map((x) => x.id))
  } catch (e: any) {
    toast.error(e.message)
    void load()
  }
}
const removing = ref<WorkspaceApp | null>(null)
async function remove() {
  const a = removing.value
  removing.value = null
  if (!a) return
  try {
    await appsApi.remove(a.id)
    apps.value = apps.value.filter((x) => x.id !== a.id)
    visited.delete(a.id)
    if (route.params.id === a.id) void router.replace({ name: 'ws-apps', params: { ws: ws.value } })
  } catch (e: any) {
    toast.error(e.message)
  }
}

const orbit = window.location.origin
</script>

<template>
  <div class="apps-view">
    <header ref="head" class="flex flex-wrap items-center gap-2">
      <h1 class="large-title mr-2">{{ t('modules.apps') }}</h1>
      <nav v-if="apps.length" class="strip min-w-0 flex-1 overflow-x-auto max-sm:order-last max-sm:basis-full" :aria-label="t('modules.apps')">
        <div class="seg" role="tablist">
          <button
            v-for="a in apps"
            :key="a.id"
            role="tab"
            :aria-selected="a.id === active?.id"
            :class="{ on: a.id === active?.id }"
            class="inline-flex items-center gap-1.5"
            data-testid="app-tab"
            @click="show(a)"
          >
            <ShieldOff v-if="!a.allowed" class="h-3.5 w-3.5 text-text-3" />
            <span class="max-w-[180px] truncate">{{ a.name }}</span>
          </button>
        </div>
      </nav>
      <div class="flex-1" :class="{ 'sm:hidden': apps.length }" />
      <template v-if="active">
        <Button variant="ghost" size="icon" class="h-8 w-8 text-text-2" :title="t('apps.reload')" :aria-label="t('apps.reload')" @click="reload(active)"><RotateCw class="h-4 w-4" /></Button>
        <Button as-child variant="ghost" size="icon" class="h-8 w-8 text-text-2">
          <a :href="active.url" target="_blank" rel="noopener noreferrer" :title="t('apps.openNew')" :aria-label="t('apps.openNew')" data-testid="app-open-new"><ExternalLink class="h-4 w-4" /></a>
        </Button>
        <Button v-if="editable" variant="ghost" size="icon" class="h-8 w-8 text-text-2" :aria-label="t('common.more')" data-testid="app-more" @click="menu = { a: active, el: $event.currentTarget as HTMLElement }"><MoreHorizontal class="h-4 w-4" /></Button>
      </template>
      <Button v-if="editable && sites.length" variant="outline" size="sm" class="shrink-0 max-sm:px-2!" :aria-label="t('apps.add')" data-testid="app-add" @click="openAdd"><Plus class="h-4 w-4" /><span class="max-sm:hidden">{{ t('apps.add') }}</span></Button>
    </header>

    <div v-if="loaded && !apps.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #5856d6"><AppWindow class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('apps.none') }}</p>
      <p class="mt-1 max-w-[420px] text-[13px] text-text-2">{{ sites.length ? t('apps.noneHint') : t('apps.noSites') }}</p>
      <Button v-if="editable && sites.length" class="mt-4" size="sm" @click="openAdd"><Plus class="h-4 w-4" />{{ t('apps.add') }}</Button>
    </div>

    <div v-show="apps.length" ref="stage" class="stage card" :style="{ height: `calc(100dvh - ${top}px - var(--apps-bottom))` }">
      <iframe
        v-for="a in shownFrames"
        v-show="a.id === active?.id"
        :key="`${a.id}:${a.url}:${reloads[a.id] ?? 0}`"
        :src="a.url"
        :title="a.name"
        class="frame"
        sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads allow-modals"
        allow="clipboard-read; clipboard-write; fullscreen"
        data-testid="app-frame"
      />
      <template v-if="active && state(active) !== 'frame'">
        <div class="say">
          <LoaderCircle v-if="state(active) === 'checking'" class="h-6 w-6 animate-spin text-text-3" />
          <template v-else>
            <span class="tile h-12 w-12" style="--tile: #8e8e93"><ShieldOff class="h-6 w-6" /></span>
            <template v-if="state(active) === 'blocked'">
              <p class="mt-4 text-[15px] font-semibold">{{ t('apps.notAllowed') }}</p>
              <p class="mt-1 max-w-[460px] text-[13px] text-text-2">{{ t('apps.notAllowedHint', { host: hostOf(active.url) }) }}</p>
            </template>
            <template v-else-if="checks[active.id]?.res?.why === 'elsewhere'">
              <p class="mt-4 text-[15px] font-semibold" data-testid="app-refused">{{ t('apps.elsewhere', { site: checks[active.id]?.res?.site || '?' }) }}</p>
              <p class="mt-1 max-w-[460px] text-[13px] text-text-2">{{ t('apps.elsewhereHint') }}</p>
            </template>
            <template v-else>
              <p class="mt-4 text-[15px] font-semibold" data-testid="app-refused">{{ t('apps.refused', { host: hostOf(active.url) }) }}</p>
              <p class="mt-1 max-w-[460px] text-[13px] text-text-2">{{ t('apps.refusedHint', { orbit }) }}</p>
            </template>
            <Button as-child size="sm" class="mt-4">
              <a :href="active.url" target="_blank" rel="noopener noreferrer"><ExternalLink class="h-4 w-4" />{{ t('apps.openNew') }}</a>
            </Button>
          </template>
        </div>
      </template>
    </div>

    <Popover v-if="menu" :anchor="menu.el" placement="bottom-end" :width="220" @close="menu = null">
      <button class="menu-item" @click="openEdit(menu.a)"><Pencil class="h-4 w-4 text-text-2" />{{ t('apps.edit') }}</button>
      <button class="menu-item" :disabled="apps[0]?.id === menu.a.id" @click="move(menu.a, -1)"><ArrowLeft class="h-4 w-4 text-text-2" />{{ t('apps.moveLeft') }}</button>
      <button class="menu-item" :disabled="apps[apps.length - 1]?.id === menu.a.id" @click="move(menu.a, 1)"><ArrowRight class="h-4 w-4 text-text-2" />{{ t('apps.moveRight') }}</button>
      <div class="menu-sep" />
      <button class="menu-item danger" @click="removing = menu.a; menu = null"><Trash2 class="h-4 w-4" />{{ t('apps.remove') }}</button>
    </Popover>
    <Confirm :open="!!removing" :title="t('apps.removeTitle')" :message="t('apps.removeBody', { name: removing?.name ?? '' })" :confirm-label="t('apps.remove')" danger @confirm="remove" @close="removing = null" />

    <Overlay :open="!!editing" :title="editing === 'new' ? t('apps.addTitle') : t('apps.edit')" size="md" @close="editing = null">
      <form class="space-y-3" @submit.prevent="save">
        <label class="block"><span class="label">{{ t('apps.url') }}</span><Input v-model="form.url" type="url" :placeholder="sites[0] ? `${sites[0].replace('*.', '')}/` : 'https://'" data-testid="app-url" /></label>
        <label class="block"><span class="label">{{ t('apps.name') }}</span><Input v-model="form.name" maxlength="80" :placeholder="form.url ? hostOf(form.url) : t('apps.namePh')" data-testid="app-name" /></label>
        <p class="text-[12px] text-text-3">{{ t('apps.sitesHint', { sites: sites.join(', ') }) }}</p>
        <button type="submit" class="hidden" />
      </form>
      <template #footer>
        <Button variant="secondary" @click="editing = null">{{ t('common.cancel') }}</Button>
        <Button :disabled="!form.url.trim() || saving" data-testid="app-save" @click="save">{{ editing === 'new' ? t('apps.add') : t('common.save') }}</Button>
      </template>
    </Overlay>
  </div>
</template>

<style scoped>
.apps-view {
  --apps-bottom: 16px;
  padding: 20px clamp(16px, 3vw, 32px) 0;
}
@media (max-width: 639px) {
  .apps-view {
    /* Above the tab bar. */
    --apps-bottom: calc(58px + env(safe-area-inset-bottom) + 12px);
  }
}
.strip {
  scrollbar-width: none;
}
.stage {
  position: relative;
  margin-top: 14px;
  min-height: 320px;
  overflow: hidden;
  border-radius: 12px;
}
.frame {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
  background: #fff;
}
.say {
  display: flex;
  height: 100%;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  text-align: center;
}
</style>

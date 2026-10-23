<script setup lang="ts">
import { computed, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { Link2, Bookmark as BookmarkIcon, BookmarkCheck, MoreHorizontal, Pencil, RefreshCw, Trash2, LoaderCircle, Globe } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import Toggle from '@/components/ui/Toggle.vue'
import { bookmarksApi, pictureOf, forgetPicture, hostOf, type Bookmark } from '@/lib/bookmarks'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Bookmarks: paste a link to keep it — Orbit reads its title, description
// and picture. Cards open the link (a "read later" one is then read);
// filter by read later and by tag.
const route = useRoute()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)

const list = ref<Bookmark[]>([])
const loaded = ref(false)
const pics = reactive<Record<string, string>>({})
async function load() {
  try {
    list.value = await bookmarksApi.list(ws.value)
    for (const b of list.value) if (b.has_image && !pics[b.id]) void pictureOf(b.id).then((u) => (pics[b.id] = u)).catch(() => {})
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, () => {
  loaded.value = false
  void load()
}, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === ws.value && e.what === 'bookmarks') void load()
})
onUnmounted(off)

// Adding.
const link = ref('')
const later = ref(false)
const adding = ref(false)
async function add() {
  const url = link.value.trim()
  if (!url || adding.value) return
  adding.value = true
  try {
    const b = await bookmarksApi.create(ws.value, { url, read_later: later.value })
    link.value = ''
    list.value = [b, ...list.value.filter((x) => x.id !== b.id)]
    if (b.has_image) void pictureOf(b.id).then((u) => (pics[b.id] = u)).catch(() => {})
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    adding.value = false
  }
}

// Filters.
const filter = ref<'all' | 'unread'>('all')
const tag = ref('')
const tags = computed(() => [...new Set(list.value.flatMap((b) => b.tags))].sort((a, b) => a.localeCompare(b)))
const shown = computed(() =>
  list.value.filter((b) => (filter.value === 'all' || (b.read_later && !b.read_at)) && (!tag.value || b.tags.includes(tag.value))),
)

function put(b: Bookmark) {
  const i = list.value.findIndex((x) => x.id === b.id)
  if (i >= 0) list.value[i] = b
}
async function opened(b: Bookmark) {
  if (b.read_later && !b.read_at && editable.value) put(await bookmarksApi.update(b.id, { read: true }).catch(() => b))
}
async function toggleLater(b: Bookmark) {
  try {
    put(await bookmarksApi.update(b.id, b.read_later && !b.read_at ? { read: true } : { read_later: true }))
  } catch (e: any) {
    toast.error(e.message)
  }
}

// The ⋯ menu.
const menu = ref<{ b: Bookmark; el: HTMLElement } | null>(null)
async function refresh(b: Bookmark) {
  menu.value = null
  try {
    const r = await bookmarksApi.refresh(b.id)
    put(r)
    delete pics[b.id]
    forgetPicture(b.id)
    if (r.has_image) void pictureOf(r.id).then((u) => (pics[r.id] = u)).catch(() => {})
  } catch (e: any) {
    toast.error(e.message)
  }
}
const removing = ref<Bookmark | null>(null)
async function remove() {
  const b = removing.value
  removing.value = null
  if (!b) return
  try {
    await bookmarksApi.remove(b.id)
    list.value = list.value.filter((x) => x.id !== b.id)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// Editing.
const editing = ref<Bookmark | null>(null)
const form = reactive({ title: '', description: '', tags: '', notes: '', read_later: false })
function edit(b: Bookmark) {
  menu.value = null
  editing.value = b
  Object.assign(form, { title: b.title, description: b.description, tags: b.tags.join(', '), notes: b.notes, read_later: b.read_later })
}
async function saveEdit() {
  const b = editing.value
  if (!b) return
  try {
    put(await bookmarksApi.update(b.id, { title: form.title, description: form.description, notes: form.notes, read_later: form.read_later, tags: form.tags.split(',').map((x) => x.trim()).filter(Boolean) }))
    editing.value = null
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="bookmarks-view">
    <header class="mb-5">
      <h1 class="large-title">{{ t('modules.bookmarks') }}</h1>
      <p class="subhead mt-0.5">{{ workspace?.name }} · {{ list.length }}</p>
    </header>

    <form v-if="editable" class="add card mb-5 flex flex-wrap items-center gap-3 px-4 py-2.5" @submit.prevent="add">
      <Link2 class="h-4 w-4 shrink-0 text-text-3" />
      <input v-model="link" class="min-w-[200px] flex-1 bg-transparent py-1 text-[15px] outline-none placeholder:text-text-faint" :placeholder="t('bookmarks.add')" :disabled="adding" data-testid="bookmark-url" />
      <label class="flex items-center gap-2 text-[13px] text-text-2">{{ t('bookmarks.readLater') }}<Toggle v-model="later" :aria-label="t('bookmarks.readLater')" /></label>
      <button class="btn-primary btn-sm" :disabled="!link.trim() || adding" data-testid="bookmark-add">
        <LoaderCircle v-if="adding" class="h-4 w-4 animate-spin" />{{ adding ? t('bookmarks.adding') : t('bookmarks.save') }}
      </button>
    </form>

    <div v-if="list.length" class="mb-4 flex flex-wrap items-center gap-2">
      <div class="seg" role="radiogroup">
        <button :class="{ on: filter === 'all' }" @click="filter = 'all'">{{ t('bookmarks.all') }}</button>
        <button :class="{ on: filter === 'unread' }" @click="filter = 'unread'">{{ t('bookmarks.unread') }}</button>
      </div>
      <button v-for="g in tags" :key="g" class="chip-tag" :class="{ on: tag === g }" @click="tag = tag === g ? '' : g">#{{ g }}</button>
    </div>

    <div v-if="loaded && !list.length" class="flex flex-col items-center py-16 text-center">
      <span class="tile h-14 w-14" style="--tile: #ff9500"><BookmarkIcon class="h-7 w-7" /></span>
      <p class="mt-4 text-[15px] font-semibold">{{ t('bookmarks.none') }}</p>
      <p class="mt-1 max-w-[360px] text-[13px] text-text-2">{{ t('bookmarks.noneHint') }}</p>
    </div>
    <p v-else-if="loaded && !shown.length" class="footnote py-8 text-center">{{ t('bookmarks.noneHere') }}</p>

    <div class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-4">
      <div v-for="b in shown" :key="b.id" class="card bm" data-testid="bookmark">
        <a :href="b.url" target="_blank" rel="noopener noreferrer" class="block" @click="opened(b)">
          <div class="pic">
            <img v-if="pics[b.id]" :src="pics[b.id]" alt="" />
            <Globe v-else class="h-8 w-8 text-text-faint" :stroke-width="1.5" />
          </div>
          <div class="px-3.5 pt-3">
            <div class="line-clamp-2 text-[14.5px] font-semibold leading-snug">{{ b.title || b.url }}</div>
            <p v-if="b.description" class="mt-1 line-clamp-2 text-[12.5px] text-text-2">{{ b.description }}</p>
          </div>
        </a>
        <div class="flex items-center gap-1.5 px-3.5 pb-2.5 pt-2">
          <span class="min-w-0 flex-1 truncate text-[12px] text-text-3">{{ b.site_name || hostOf(b.url) }}<template v-for="g in b.tags" :key="g"> · #{{ g }}</template></span>
          <button
            v-if="editable"
            class="icon-btn h-7 w-7"
            :class="{ later: b.read_later && !b.read_at }"
            :title="b.read_later && !b.read_at ? t('bookmarks.markRead') : t('bookmarks.markUnread')"
            :aria-label="b.read_later && !b.read_at ? t('bookmarks.markRead') : t('bookmarks.markUnread')"
            data-testid="bookmark-later"
            @click="toggleLater(b)"
          >
            <BookmarkIcon v-if="b.read_later && !b.read_at" class="h-4 w-4" fill="currentColor" />
            <BookmarkCheck v-else class="h-4 w-4" />
          </button>
          <button v-if="editable" class="icon-btn h-7 w-7" :aria-label="t('common.more')" @click="menu = { b, el: $event.currentTarget as HTMLElement }"><MoreHorizontal class="h-4 w-4" /></button>
        </div>
      </div>
    </div>

    <Popover v-if="menu" :anchor="menu.el" placement="bottom-end" :width="220" @close="menu = null">
      <button class="menu-item" @click="edit(menu.b)"><Pencil class="h-4 w-4 text-text-2" />{{ t('bookmarks.edit') }}</button>
      <button class="menu-item" @click="refresh(menu.b)"><RefreshCw class="h-4 w-4 text-text-2" />{{ t('bookmarks.refresh') }}</button>
      <div class="menu-sep" />
      <button class="menu-item danger" @click="removing = menu.b; menu = null"><Trash2 class="h-4 w-4" />{{ t('bookmarks.delete') }}</button>
    </Popover>
    <Confirm :open="!!removing" :title="t('bookmarks.deleteTitle')" :message="removing?.title || removing?.url || ''" :confirm-label="t('common.delete')" danger @confirm="remove" @close="removing = null" />

    <Overlay :open="!!editing" :title="t('bookmarks.edit')" size="md" @close="editing = null">
      <form class="space-y-3" @submit.prevent="saveEdit">
        <label class="block"><span class="label">{{ t('bookmarks.title') }}</span><input v-model="form.title" class="input" maxlength="300" /></label>
        <label class="block"><span class="label">{{ t('bookmarks.description') }}</span><textarea v-model="form.description" class="input" rows="2" maxlength="700" /></label>
        <label class="block"><span class="label">{{ t('bookmarks.tags') }}</span><input v-model="form.tags" class="input" :placeholder="t('bookmarks.tagsPh')" /></label>
        <label class="block"><span class="label">{{ t('bookmarks.notes') }}</span><textarea v-model="form.notes" class="input" rows="3" maxlength="20000" /></label>
        <label class="flex items-center justify-between text-[14px]">{{ t('bookmarks.readLater') }}<Toggle v-model="form.read_later" :aria-label="t('bookmarks.readLater')" /></label>
        <button type="submit" class="hidden" />
      </form>
      <template #footer>
        <button class="btn" @click="editing = null">{{ t('common.cancel') }}</button>
        <button class="btn-primary" @click="saveEdit">{{ t('common.save') }}</button>
      </template>
    </Overlay>
  </div>
</template>

<style scoped>
.bookmarks-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
.add {
  border-radius: 14px;
}
.bm {
  overflow: hidden;
}
.pic {
  display: grid;
  place-items: center;
  height: 140px;
  overflow: hidden;
  background: var(--fill-2);
}
.pic img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.later {
  color: var(--accent);
}
.chip-tag {
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--fill);
  font-size: 12.5px;
  color: var(--text-2);
}
.chip-tag.on {
  background: var(--accent);
  color: var(--accent-fg, #fff);
}
</style>

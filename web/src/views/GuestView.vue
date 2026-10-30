<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { FileText, Pencil, Users } from 'lucide-vue-next'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import CodeArea from '@/components/editor/CodeArea.vue'
import { guestApi, type GuestPage, type GuestShare } from '@/lib/shareApi'
import { ApiError } from '@/lib/api'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'

// /shared/<link>: a page someone opened to anyone with the link — no
// account. It reads like the page in Orbit (with what it shows in place);
// the pages inside it, when the link opens them too, are listed beside it.
// An edit link adds Edit: the page's Markdown, saved with Save (someone
// else's newer save wins, and says so).
const route = useRoute()
const router = useRouter()
const branding = useBrandingStore()
const { t } = useI18n()

const token = computed(() => String(route.params.token ?? ''))
const share = ref<GuestShare | null>(null)
const page = ref<GuestPage | null>(null)
const gone = ref('')
async function open() {
  try {
    share.value = await guestApi.open(token.value)
    await show(typeof route.query.page === 'string' ? route.query.page : share.value.root)
  } catch (e: any) {
    gone.value = e.message
  }
}
onMounted(() => {
  // Not for search engines.
  const m = Object.assign(document.createElement('meta'), { name: 'robots', content: 'noindex, nofollow' })
  document.head.append(m)
  void open()
})
async function show(id: string) {
  if (!share.value) return
  editing.value = false
  try {
    page.value = id === share.value.root && !page.value ? share.value.page : await guestApi.page(token.value, id)
    document.title = page.value.title || t('pages.untitled')
    void router.replace({ query: id === share.value.root ? {} : { page: id } })
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 404) note.value = t('guest.notShared')
    else note.value = e.message
  }
}
watch(
  () => route.query.page,
  (p) => {
    if (share.value && typeof p === 'string' && p !== page.value?.id) void show(p)
  },
)

// The pages inside, as a tree.
const tree = computed(() => {
  const all = share.value?.pages ?? []
  const out: { p: GuestPage; depth: number }[] = []
  const walk = (parent: string | null, depth: number) => {
    for (const p of all.filter((x) => (x.parent_id ?? null) === parent)) {
      out.push({ p, depth })
      walk(p.id, depth + 1)
    }
  }
  const root = all.find((p) => p.id === share.value?.root)
  if (root) {
    out.push({ p: root, depth: 0 })
    walk(root.id, 1)
  }
  return out
})

// ---- Editing (an edit link) ----
const editing = ref(false)
const draftTitle = ref('')
const draft = ref('')
const saving = ref(false)
const note = ref('')
function startEdit() {
  if (!page.value) return
  draftTitle.value = page.value.title
  draft.value = page.value.content ?? ''
  note.value = ''
  editing.value = true
}
async function save() {
  if (!page.value) return
  saving.value = true
  try {
    page.value = await guestApi.save(token.value, page.value.id, { title: draftTitle.value, content: draft.value, version: page.value.version ?? 0 })
    editing.value = false
    note.value = t('guest.saved')
  } catch (e: any) {
    if (e instanceof ApiError && e.status === 409 && e.body?.page) {
      page.value = e.body.page
      note.value = t('guest.conflict')
    } else note.value = e.message
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="guest">
    <header class="g-top">
      <img :src="branding.logoMode === 'upload' ? branding.logoDataURL : DEFAULT_MARK" alt="" class="h-7 w-7 rounded-lg" />
      <span class="min-w-0 flex-1 truncate text-[13.5px] font-semibold">{{ share?.workspace }}</span>
      <span class="pill"><Users class="h-3 w-3" />{{ share?.can_edit ? t('guest.canEdit') : t('guest.readOnly') }}</span>
    </header>

    <p v-if="gone" class="py-24 text-center text-[14px] text-text-3" data-testid="guest-gone">{{ gone }}</p>
    <div v-else-if="share && page" class="g-body" :class="{ 'with-tree': tree.length > 1 }">
      <nav v-if="tree.length > 1" class="g-tree" :aria-label="t('guest.pages')">
        <button
          v-for="x in tree"
          :key="x.p.id"
          type="button"
          class="g-row"
          :class="{ on: x.p.id === page.id }"
          :style="{ paddingLeft: `${10 + x.depth * 14}px` }"
          data-testid="guest-tree-page"
          @click="show(x.p.id)"
        >
          <span v-if="x.p.icon" class="w-4 text-center">{{ x.p.icon }}</span>
          <FileText v-else class="h-4 w-4 shrink-0 text-text-3" />
          <span class="truncate">{{ x.p.title || t('pages.untitled') }}</span>
        </button>
      </nav>
      <main class="g-page">
        <div class="mb-4 flex items-start gap-3">
          <h1 v-if="!editing" class="min-w-0 flex-1 text-[32px] font-bold leading-tight" data-testid="guest-title">
            <span v-if="page.icon" class="mr-2">{{ page.icon }}</span>{{ page.title || t('pages.untitled') }}
          </h1>
          <input v-else v-model="draftTitle" class="min-w-0 flex-1 rounded-lg bg-fill-2 px-3 py-1.5 text-[26px] font-bold outline-none" maxlength="300" data-testid="guest-title-input" />
          <template v-if="share.can_edit">
            <button v-if="!editing" type="button" class="btn btn-sm shrink-0" data-testid="guest-edit" @click="startEdit"><Pencil class="h-3.5 w-3.5" />{{ t('editor.edit') }}</button>
            <template v-else>
              <button type="button" class="btn btn-sm shrink-0" @click="editing = false">{{ t('common.cancel') }}</button>
              <button type="button" class="btn-primary btn-sm shrink-0" :disabled="saving" data-testid="guest-save" @click="save">{{ t('common.save') }}</button>
            </template>
          </template>
        </div>
        <p v-if="note" class="mb-3 rounded-lg bg-fill-2 px-3 py-2 text-[12.5px] text-text-2" role="status" data-testid="guest-note">{{ note }}</p>
        <CodeArea v-if="editing" v-model="draft" markdown :label="t('guest.text')" testid="guest-text" />
        <MarkdownView v-else-if="page.content?.trim()" :content="page.content" :editable="false" :guest="token" @page="show" />
        <p v-else class="text-[14px] text-text-3">{{ t('guest.empty') }}</p>
      </main>
    </div>
    <footer class="g-foot">{{ t('guest.foot', { app: branding.appName }) }}</footer>
  </div>
</template>

<style scoped>
.guest {
  min-height: 100dvh;
  background: var(--bg);
  color: var(--text);
}
.g-top {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px clamp(14px, 3vw, 28px);
  border-bottom: 0.5px solid var(--border);
  background: color-mix(in srgb, var(--bg) 88%, transparent);
  backdrop-filter: blur(12px);
}
.g-body {
  display: grid;
  max-width: 1120px;
  margin: 0 auto;
  padding: 28px clamp(14px, 3vw, 28px) 60px;
  grid-template-columns: minmax(0, 1fr);
  gap: 28px;
}
.g-body.with-tree {
  grid-template-columns: 220px minmax(0, 1fr);
}
@media (max-width: 760px) {
  .g-body.with-tree {
    grid-template-columns: minmax(0, 1fr);
  }
}
.g-tree {
  display: flex;
  align-self: start;
  flex-direction: column;
  gap: 1px;
  position: sticky;
  top: 70px;
}
.g-row {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 10px;
  border-radius: 8px;
  font-size: 13.5px;
  text-align: left;
}
.g-row:hover {
  background: var(--fill-2);
}
.g-row.on {
  background: var(--fill);
  font-weight: 600;
}
.g-page {
  max-width: 760px;
  min-width: 0;
}
.g-foot {
  padding: 20px;
  text-align: center;
  font-size: 12px;
  color: var(--text-3);
}
</style>

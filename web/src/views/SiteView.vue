<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { FileText, Menu, Search, X } from '@lucide/vue'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { siteApi, type GuestPage, type PublicSite, type SiteHit } from '@/lib/shareApi'
import { ApiError } from '@/lib/api'
import { fmtDate } from '@/lib/format'
import { DEFAULT_MARK, useBrandingStore } from '@/stores/branding'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'

// A workspace published as a website: the pages its owners picked and what's
// under them, read-only, open to anyone. At /site/<name> (base) or on its own
// domain (base ""). The pages down the side, the one open in the middle, and
// a search across them all; on a phone the pages open from the menu button.
const props = defineProps<{ slug: string; base: string; page?: string }>()
const { t } = useI18n()
const router = useRouter()
const branding = useBrandingStore()

const site = ref<PublicSite | null>(null)
const current = ref<GuestPage | null>(null)
const gone = ref('')
const missing = ref(false)
const navOpen = ref(false)

const pageId = computed(() => props.page || site.value?.home || '')
const href = (id: string) => (id === site.value?.home ? props.base || '/' : `${props.base}/p/${id}`)

// The pages as a tree, in the site's order, each with its depth.
const tree = computed(() => {
  const pages = site.value?.pages ?? []
  const depth = new Map<string, number>()
  return pages.map((p) => {
    const d = p.parent_id && depth.has(p.parent_id) ? depth.get(p.parent_id)! + 1 : 0
    depth.set(p.id, d)
    return { p, depth: d }
  })
})

async function open() {
  try {
    site.value = await siteApi.open(props.slug)
  } catch (e: any) {
    gone.value = e instanceof ApiError && e.status === 404 ? t('site.none') : e.message
  }
}
async function show(id: string) {
  missing.value = false
  if (!id) {
    current.value = null
    return
  }
  try {
    current.value = await siteApi.page(props.slug, id)
  } catch {
    current.value = null
    missing.value = true
  }
}
watch(
  () => [site.value?.slug, pageId.value] as const,
  ([s, id]) => {
    if (s) void show(id)
    navOpen.value = false
  },
)
watch([() => current.value?.title, () => site.value?.title], () => {
  const name = site.value?.title ?? ''
  const title = current.value?.title
  document.title = title && title !== name ? `${title} · ${name}` : name
})
onMounted(open)

// A link to another page of the site, from inside a page.
function go(id: string) {
  if (site.value?.pages.some((p) => p.id === id)) void router.push(href(id))
}

// ---- Search ----
const q = ref('')
const hits = ref<SiteHit[] | null>(null)
let timer: ReturnType<typeof setTimeout> | null = null
watch(q, (v) => {
  if (timer) clearTimeout(timer)
  if (v.trim().length < 2) {
    hits.value = null
    return
  }
  timer = setTimeout(async () => {
    try {
      hits.value = await siteApi.search(props.slug, v.trim())
    } catch {
      hits.value = []
    }
  }, 250)
})
function pick(h: SiteHit) {
  q.value = ''
  hits.value = null
  void router.push(href(h.id))
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    q.value = ''
    navOpen.value = false
  }
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => {
  window.removeEventListener('keydown', onKey)
  if (timer) clearTimeout(timer)
})
const logo = computed(() => (branding.logoMode === 'theme' ? branding.crestUrl : branding.logoMode === 'upload' ? branding.logoDataURL : DEFAULT_MARK))
</script>

<template>
  <div class="site">
    <header class="s-top">
      <Button variant="ghost" size="icon" class="s-menu text-text-2" :aria-label="t('site.pages')" :aria-expanded="navOpen" @click="navOpen = !navOpen"><Menu class="h-5 w-5" /></Button>
      <RouterLink :to="base || '/'" class="flex min-w-0 items-center gap-2.5">
        <img :src="logo" alt="" class="h-7 w-7 shrink-0 rounded-lg" />
        <span class="truncate text-[14.5px] font-semibold" data-testid="site-title">{{ site?.title }}</span>
      </RouterLink>
      <div v-if="site" class="s-search">
        <Search class="pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2 text-text-3" />
        <Input v-model="q" type="search" class="h-8 pl-8" :placeholder="t('site.search')" :aria-label="t('site.search')" data-testid="site-search" @keydown.enter="hits?.[0] && pick(hits[0])" />
        <div v-if="hits" class="s-hits" role="listbox" data-testid="site-hits">
          <p v-if="!hits.length" class="px-3 py-2.5 text-[13px] text-text-3">{{ t('site.noHits') }}</p>
          <button v-for="h in hits" :key="h.id" type="button" role="option" class="s-hit" @click="pick(h)">
            <span class="block truncate text-[13.5px] font-medium">{{ h.icon ? h.icon + ' ' : '' }}{{ h.title || t('pages.untitled') }}</span>
            <span v-if="h.snippet" class="line-clamp-2 block text-[12px] text-text-3">{{ h.snippet }}</span>
          </button>
        </div>
      </div>
    </header>

    <p v-if="gone" class="py-24 text-center text-[14px] text-text-3" data-testid="site-gone">{{ gone }}</p>
    <div v-else-if="site" class="s-body">
      <nav class="s-nav" :class="{ open: navOpen }" :aria-label="t('site.pages')">
        <div class="mb-2 flex items-center justify-between px-2 md:hidden">
          <span class="cap">{{ t('site.pages') }}</span>
          <Button variant="ghost" size="icon" class="h-7 w-7 text-text-2" :aria-label="t('common.close')" @click="navOpen = false"><X class="h-4 w-4" /></Button>
        </div>
        <RouterLink
          v-for="x in tree"
          :key="x.p.id"
          :to="href(x.p.id)"
          class="s-row"
          :class="{ on: x.p.id === pageId }"
          :style="{ paddingLeft: `${10 + x.depth * 14}px` }"
          data-testid="site-nav-page"
        >
          <span v-if="x.p.icon" class="w-4 shrink-0 text-center">{{ x.p.icon }}</span>
          <FileText v-else class="h-4 w-4 shrink-0 text-text-3" />
          <span class="truncate">{{ x.p.title || t('pages.untitled') }}</span>
        </RouterLink>
        <p v-if="!tree.length" class="px-2 text-[13px] text-text-3">{{ t('site.empty') }}</p>
      </nav>
      <div v-if="navOpen" class="s-scrim md:hidden" @click="navOpen = false" />
      <main class="s-page">
        <template v-if="current">
          <h1 class="text-[32px] leading-tight font-bold" data-testid="site-page-title"><span v-if="current.icon" class="mr-2">{{ current.icon }}</span>{{ current.title || t('pages.untitled') }}</h1>
          <p class="mt-1 mb-6 text-[12.5px] text-text-3">{{ t('site.updated', { date: fmtDate(current.updated_at) }) }}</p>
          <MarkdownView v-if="current.content?.trim()" :content="current.content" :editable="false" :guest="`site/${slug}`" data-testid="site-page" @page="go" />
        </template>
        <p v-else-if="missing" class="py-16 text-center text-[14px] text-text-3">{{ t('site.missing') }}</p>
      </main>
    </div>
  </div>
</template>

<style scoped>
.site {
  min-height: 100dvh;
  background: var(--bg);
  color: var(--text);
}
.s-top {
  position: sticky;
  top: 0;
  z-index: 3;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px clamp(16px, 3vw, 28px);
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--bg) 88%, transparent);
  backdrop-filter: blur(12px);
}
.s-menu {
  display: none;
}
.s-search {
  position: relative;
  width: min(320px, 40vw);
  margin-left: auto;
}
.s-hits {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  width: min(420px, calc(100vw - 32px));
  max-height: 60vh;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 12px;
  background: var(--surface);
  box-shadow: var(--shadow-lg, 0 12px 32px rgb(0 0 0 / 0.18));
}
.s-hit {
  display: block;
  width: 100%;
  padding: 7px 10px;
  border-radius: 8px;
  text-align: left;
}
.s-hit:hover,
.s-hit:focus-visible {
  background: var(--fill-2);
}
.s-body {
  display: grid;
  max-width: 1120px;
  margin: 0 auto;
  padding: 28px clamp(16px, 3vw, 28px) 60px;
  grid-template-columns: 240px minmax(0, 1fr);
  gap: 32px;
}
.s-nav {
  display: flex;
  align-self: start;
  flex-direction: column;
  gap: 1px;
  position: sticky;
  top: 74px;
  max-height: calc(100dvh - 96px);
  overflow-y: auto;
}
.s-row {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 5px 10px;
  border-radius: 8px;
  font-size: 13.5px;
}
.s-row:hover {
  background: var(--fill-2);
}
.s-row.on {
  background: var(--fill);
  font-weight: 600;
}
.s-page {
  max-width: 760px;
  min-width: 0;
}
.s-scrim {
  position: fixed;
  inset: 0;
  z-index: 4;
  background: rgb(0 0 0 / 0.3);
}
@media (max-width: 767px) {
  .s-menu {
    display: inline-flex;
    margin-left: -8px;
  }
  .s-search {
    width: auto;
    flex: 1;
    min-width: 0;
  }
  .s-body {
    grid-template-columns: minmax(0, 1fr);
  }
  .s-nav {
    display: none;
  }
  .s-nav.open {
    display: flex;
    position: fixed;
    top: 0;
    bottom: 0;
    left: 0;
    z-index: 5;
    width: min(300px, 85vw);
    max-height: none;
    padding: 12px 8px;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }
}
</style>

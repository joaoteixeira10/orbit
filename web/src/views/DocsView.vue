<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Search } from 'lucide-vue-next'
import MarkdownView from '@/components/editor/MarkdownView.vue'
import { guidesApi, type Guide, type GuideSection } from '@/api'
import { useToastStore } from '@/stores/toast'

// Orbit's own guides, as this version has them (built into it): the list,
// one guide to read, and a search across them all — beside the API
// reference. Links between guides stay on this page.
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const toast = useToastStore()

const guides = ref<Guide[]>([])
const slug = computed(() => (typeof route.params.slug === 'string' && route.params.slug) || guides.value[0]?.slug || '')
const markdown = ref('')
const title = ref('')
const missing = ref(false)
const body = ref<HTMLElement | null>(null)

void guidesApi
  .list()
  .then((g) => (guides.value = g))
  .catch((e) => toast.error(e.message))

watch(
  slug,
  async (s) => {
    if (!s) return
    try {
      const g = await guidesApi.get(s)
      markdown.value = g.markdown
      title.value = g.title
      missing.value = false
      await nextTick()
      scrollToHash()
    } catch {
      markdown.value = ''
      missing.value = true
    }
  },
  { immediate: true },
)
watch(() => route.hash, () => nextTick(scrollToHash))

// The part a link points to (headings carry data-anchor), else the top —
// once the guide is drawn (a few frames after its text arrives).
function scrollToHash(tries = 20) {
  const a = decodeURIComponent(route.hash.replace(/^#/, ''))
  if (!a) return body.value?.scrollTo({ top: 0 })
  const el = body.value?.querySelector(`[data-anchor="${CSS.escape(a)}"]`)
  if (el) el.closest('h1, h2, h3, h4')?.scrollIntoView({ block: 'start' })
  else if (tries > 0) requestAnimationFrame(() => scrollToHash(tries - 1))
}

// Links to a part of a guide stay here (they'd open a new tab).
function onClick(e: MouseEvent) {
  const a = (e.target as HTMLElement).closest('a') as HTMLAnchorElement | null
  const href = a?.getAttribute('href') ?? ''
  if (!a || !(href.startsWith('/docs/') || href.startsWith('#'))) return
  e.preventDefault()
  e.stopPropagation()
  if (href.startsWith('#')) router.replace({ hash: href })
  else {
    const [path, hash] = href.split('#')
    router.push({ name: 'docs', params: { slug: path.replace('/docs/', '') }, hash: hash ? `#${hash}` : '' })
  }
}

// ---- Search across the guides ----
const q = ref('')
const hits = ref<GuideSection[]>([])
const searching = computed(() => q.value.trim().length >= 2)
let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
watch(q, (v) => {
  clearTimeout(timer)
  if (v.trim().length < 2) return (hits.value = [])
  timer = setTimeout(async () => {
    const my = ++seq
    try {
      const r = await guidesApi.search(v.trim())
      if (my === seq) hits.value = r
    } catch (e: any) {
      toast.error(e.message)
    }
  }, 200)
})
// A hit's words around what was searched for, without Markdown's marks.
function gist(text: string): string {
  const s = text
    .replace(/^\s*\|?\s*:?-{3,}.*$/gm, ' ') // a table's ruler
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/[*`>#]/g, '')
    .replace(/\|/g, ' · ')
    .replace(/\s+/g, ' ')
    .trim()
  const low = s.toLowerCase()
  const at = Math.min(...q.value.toLowerCase().split(/\s+/).filter((w) => w.length > 2).map((w) => low.indexOf(w)).filter((i) => i >= 0), Infinity)
  const from = Number.isFinite(at) ? Math.max(0, at - 60) : 0
  const cut = s.slice(from, from + 240)
  return (from > 0 ? '…' : '') + cut + (from + 240 < s.length ? '…' : '')
}
function openHit(h: GuideSection) {
  q.value = ''
  router.push({ name: 'docs', params: { slug: h.slug }, hash: h.anchor ? `#${h.anchor}` : '' })
}
</script>

<template>
  <div class="docs-page">
    <aside class="docs-side">
      <h1 class="title-2 mb-3 px-2">{{ t('docs.title') }}</h1>
      <label class="search mb-3">
        <Search class="h-4 w-4 shrink-0" aria-hidden="true" />
        <input v-model="q" type="search" :placeholder="t('docs.search')" :aria-label="t('docs.search')" autocomplete="off" data-testid="docs-search" />
      </label>
      <nav :aria-label="t('docs.title')">
        <RouterLink v-for="g in guides" :key="g.slug" :to="{ name: 'docs', params: { slug: g.slug } }" class="docs-link" :class="{ on: g.slug === slug && !searching }" @click="q = ''">
          {{ g.title }}
        </RouterLink>
      </nav>
      <a class="docs-link mt-3" href="/api/docs" target="_blank" rel="noopener">{{ t('nav.apiDocs') }} ↗</a>
    </aside>

    <main ref="body" class="docs-main">
      <template v-if="searching">
        <p class="mb-3 text-[13px] text-text-3">{{ hits.length ? t('docs.found', { n: hits.length }) : t('docs.none') }}</p>
        <button v-for="h in hits" :key="`${h.slug}#${h.anchor}`" type="button" class="hit" data-testid="docs-hit" @click="openHit(h)">
          <span class="text-[12px] text-text-3">{{ h.guide }}</span>
          <b class="block text-[15px]">{{ h.heading }}</b>
          <span class="block text-[13px] text-text-2">{{ gist(h.text) }}</span>
        </button>
      </template>
      <p v-else-if="missing" class="text-text-3">{{ t('docs.missing') }}</p>
      <!-- capture: before the view's own handling of links (a new tab) -->
      <div v-else-if="markdown" class="docs-body" data-testid="docs-guide" @click.capture="onClick">
        <MarkdownView :content="markdown" :editable="false" />
      </div>
    </main>
  </div>
</template>

<style scoped>
.docs-page {
  display: flex;
  height: 100%;
  min-height: 0;
}
.docs-side {
  flex: none;
  width: 250px;
  padding: 28px 14px;
  overflow-y: auto;
  border-right: 0.5px solid var(--border);
}
.docs-side .search {
  display: flex;
  align-items: center;
  gap: 8px;
}
.docs-link {
  display: block;
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 14px;
  color: var(--text-2);
}
.docs-link:hover {
  background: var(--fill-2);
  color: var(--text);
}
.docs-link.on {
  background: var(--accent-bg);
  color: var(--accent-text);
  font-weight: 550;
}
.docs-main {
  flex: 1;
  min-width: 0;
  padding: 32px clamp(16px, 4vw, 48px) 80px;
  overflow-y: auto;
}
.docs-body {
  max-width: 820px;
}
.docs-body :deep(h1),
.docs-body :deep(h2),
.docs-body :deep(h3),
.docs-body :deep(h4) {
  scroll-margin-top: 16px;
}
.hit {
  display: block;
  width: 100%;
  max-width: 820px;
  margin-bottom: 8px;
  padding: 12px 14px;
  border-radius: 12px;
  background: var(--fill-2);
  text-align: left;
}
.hit:hover {
  background: var(--fill);
}
@media (max-width: 760px) {
  .docs-page {
    flex-direction: column;
    height: auto;
  }
  .docs-side {
    width: auto;
    border-right: 0;
    border-bottom: 0.5px solid var(--border);
    padding: 20px 12px 12px;
  }
}
</style>

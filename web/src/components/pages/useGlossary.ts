import { computed, ref, watch, type Ref } from 'vue'
import { pagesApi } from '@/api/planner'
import { termsOf, type Term } from '@/lib/glossary'
import { usePagesStore } from '@/stores/pages'

// useGlossary is the terms of a workspace's glossary pages (but the page
// being read: a glossary doesn't explain itself), read again when one changes.
const cache = new Map<string, { at: string; terms: Term[] }>()
export function useGlossary(ws: Ref<string>, pageId: Ref<string>) {
  const pages = usePagesStore()
  const terms = ref<Term[]>([])
  const glossaries = computed(() => (pages.byWs[ws.value] ?? []).filter((p) => p.glossary && p.id !== pageId.value))
  watch(
    () => glossaries.value.map((p) => p.id + p.updated_at).join(),
    async () => {
      const out: Term[] = []
      for (const p of glossaries.value) {
        let hit = cache.get(p.id)
        if (!hit || hit.at !== p.updated_at) {
          try {
            const full = await pagesApi.get(p.id)
            hit = { at: p.updated_at, terms: termsOf(full.content, p.id, p.title) }
            cache.set(p.id, hit)
          } catch {
            continue
          }
        }
        out.push(...hit.terms)
      }
      terms.value = out
    },
    { immediate: true },
  )
  return terms
}

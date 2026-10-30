<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { FileText } from 'lucide-vue-next'
import { usePagesStore } from '@/stores/pages'

// ::pagetree — the pages inside this one (and theirs), as they are now.
const route = useRoute()
const pages = usePagesStore()
const { t } = useI18n()
const ws = computed(() => String(route.params.ws ?? ''))
const pageId = computed(() => String(route.params.page ?? ''))
const rows = computed(() => {
  const all = pages.byWs[ws.value] ?? []
  const out: { id: string; title: string; icon: string; depth: number }[] = []
  const walk = (parent: string, depth: number) => {
    if (depth > 5) return
    for (const p of all.filter((x) => x.parent_id === parent).sort((a, b) => a.rank - b.rank)) {
      out.push({ id: p.id, title: p.title, icon: p.icon, depth })
      walk(p.id, depth + 1)
    }
  }
  walk(pageId.value, 0)
  return out
})
</script>

<template>
  <div class="subpage-list" data-testid="pagetree">
    <p v-if="!rows.length" class="text-[13px] text-text-3">{{ t('blocks.noSubpages') }}</p>
    <RouterLink v-for="r in rows" :key="r.id" :to="{ name: 'ws-page', params: { ws, page: r.id } }" class="sp-row" :style="{ paddingLeft: `${6 + r.depth * 18}px` }">
      <span v-if="r.icon" class="w-4 text-center">{{ r.icon }}</span>
      <FileText v-else class="h-4 w-4 shrink-0 text-text-3" />
      <span class="truncate">{{ r.title || t('pages.untitled') }}</span>
    </RouterLink>
  </div>
</template>

<style scoped>
.subpage-list {
  margin: 0.5em 0;
  padding: 6px;
  border-radius: 10px;
  box-shadow: 0 0 0 0.5px var(--border);
}
.sp-row {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 4px 6px;
  border-radius: 7px;
  font-size: 15px;
  color: var(--text);
  text-decoration: none;
}
.sp-row:hover {
  background: var(--fill-2);
}
</style>

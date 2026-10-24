<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Table2 } from 'lucide-vue-next'
import DbCell from './DbCell.vue'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { withFormulas, type DBOption, type PropValue } from '@/lib/database'
import { todayStr } from '@/lib/dates'

// A row's values, on its own page: each property of its database, edited
// in place as in the table.
const props = defineProps<{ ws: string; pageId: string; dbId: string; editable: boolean }>()
const { t } = useI18n()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const toast = useToastStore()

const db = computed(() => pages.find(props.dbId))
const members = computed(() => wss.members[props.ws] ?? [])
// This row with its formulas worked out (over its siblings, for columns).
const row = computed(() => {
  const d = db.value?.db
  const siblings = (pages.byWs[props.ws] ?? []).filter((p) => p.parent_id === props.dbId && !p.trashed_at).map((p) => ({ id: p.id, title: p.title, rank: p.rank, props: p.props ?? {} }))
  if (!d) return siblings.find((r) => r.id === props.pageId)
  const names = new Map(members.value.map((m) => [m.user_id, m.display_name || m.username]))
  return withFormulas(d, siblings, names, todayStr()).find((r) => r.id === props.pageId)
})

async function set(prop: string, v: PropValue | null) {
  try {
    await pages.setProp(props.pageId, prop, v)
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function addOption(propId: string, o: DBOption) {
  const d = db.value?.db
  if (!d) return
  const next = JSON.parse(JSON.stringify(d))
  const p = next.props.find((x: { id: string }) => x.id === propId)
  if (!p) return
  p.options = [...(p.options ?? []), o]
  try {
    await pages.saveDB(props.dbId, next)
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div v-if="db?.db && db.db.props.length" class="row-props" data-testid="row-props">
    <RouterLink :to="{ name: 'ws-page', params: { ws, page: dbId } }" class="row-db"><Table2 class="h-3.5 w-3.5" />{{ db.title || t('pages.untitled') }}</RouterLink>
    <div v-for="p in db.db.props" :key="p.id" class="row-prop">
      <span class="row-label" :title="p.name">{{ p.name }}</span>
      <div class="min-w-0 flex-1">
        <DbCell :prop="p" :value="row?.props?.[p.id]" :editable="editable" :members="members" @set="(v) => set(p.id, v)" @add-option="(o) => addOption(p.id, o)" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.row-props {
  margin: -6px 0 18px;
  padding: 8px 0 10px;
  border-bottom: 0.5px solid var(--border);
}
.row-db {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 4px;
  font-size: 12px;
  color: var(--text-3);
}
.row-db:hover {
  color: var(--accent-text);
}
.row-prop {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 32px;
}
.row-label {
  flex: 0 0 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--text-3);
}
</style>

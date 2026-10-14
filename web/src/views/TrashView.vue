<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Trash2, RotateCcw, FileText } from 'lucide-vue-next'
import Confirm from '@/components/ui/Confirm.vue'
import { pagesApi, type PageSummary } from '@/api/planner'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { relDay } from '@/lib/dates'

// A workspace's trash: pages wait here 30 days before they are deleted for
// good. Only the pages trashed directly are listed (their sub-pages come
// back with them).
const route = useRoute()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const editable = computed(() => !!workspace.value && workspace.value.role !== 'viewer' && !workspace.value.archived_at && !auth.privileges.read_only)
const items = ref<PageSummary[]>([])
const loaded = ref(false)

async function load() {
  try {
    items.value = await pagesApi.trash(ws.value)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, load)
onMounted(load)

// Top-level trashed pages: their parent isn't in the trash with them.
const top = computed(() => {
  const ids = new Map(items.value.map((p) => [p.id, p]))
  return items.value.filter((p) => !p.parent_id || ids.get(p.parent_id)?.trashed_at !== p.trashed_at)
})
const inside = (p: PageSummary) => items.value.filter((c) => c.parent_id === p.id).length

async function restore(p: PageSummary) {
  try {
    await pagesApi.restore(p.id)
    toast.success(t('pages.restored'))
    await Promise.all([load(), pages.load(ws.value)])
  } catch (e: any) {
    toast.error(e.message)
  }
}
const confirmFor = ref<PageSummary | null>(null)
async function destroy() {
  if (!confirmFor.value) return
  try {
    await pagesApi.destroy(confirmFor.value.id)
    confirmFor.value = null
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="trash">
    <h1 class="large-title">{{ t('pages.trash') }}</h1>
    <p class="subhead mb-6 mt-1">{{ t('pages.trashHint') }}</p>
    <div v-if="top.length" class="group">
      <div v-for="p in top" :key="p.id" class="group-row">
        <span class="w-6 text-center text-[17px]"><template v-if="p.icon">{{ p.icon }}</template><FileText v-else class="mx-auto h-4 w-4 text-text-3" /></span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14.5px]">{{ p.title || t('pages.untitled') }}<span v-if="inside(p)" class="text-text-3"> +{{ inside(p) }}</span></span>
          <span class="block text-[12px] text-text-3">{{ t('pages.inTrash', { when: relDay(p.trashed_at!.slice(0, 10)).toLowerCase() }) }}</span>
        </span>
        <template v-if="editable">
          <button class="btn btn-sm" @click="restore(p)"><RotateCcw class="h-3.5 w-3.5" />{{ t('pages.restore') }}</button>
          <button class="icon-btn !text-danger" :title="t('pages.deleteForever')" @click="confirmFor = p"><Trash2 class="h-4 w-4" /></button>
        </template>
      </div>
    </div>
    <div v-else-if="loaded" class="py-20 text-center">
      <Trash2 class="mx-auto mb-3 h-10 w-10 text-text-faint" :stroke-width="1.5" />
      <p class="title-3">{{ t('pages.trashEmpty') }}</p>
    </div>
    <Confirm
      :open="!!confirmFor"
      :title="t('pages.deleteForeverTitle', { name: confirmFor?.title || t('pages.untitled') })"
      :message="t('pages.deleteForeverBody')"
      :confirm-label="t('pages.deleteForever')"
      danger
      @confirm="destroy"
      @close="confirmFor = null"
    />
  </div>
</template>

<style scoped>
.trash {
  padding: 32px clamp(16px, 4vw, 48px) 64px;
  max-width: 820px;
}
</style>

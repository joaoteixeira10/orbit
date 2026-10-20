<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronRight, Plus, MoreHorizontal, FileText } from 'lucide-vue-next'
import { usePagesStore, type PageNode } from '@/stores/pages'
import { useSortable, type SortMove } from '@/lib/sortable'
import PageMenu from './PageMenu.vue'

// One level of a workspace's page tree in the sidebar. Rows are dragged to
// reorder within their level; "Move to…" in a row's menu changes its parent.
const props = withDefaults(defineProps<{ ws: string; nodes: PageNode[]; depth?: number; parent?: string | null; canEdit: boolean }>(), {
  depth: 0,
  parent: null,
})
const emit = defineEmits<{ add: [parent: string | null]; move: [id: string, parent: string | null, after: string] }>()
const pages = usePagesStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const activeId = computed(() => (route.name === 'ws-page' ? String(route.params.page) : ''))
const menuFor = ref('')
const menuAnchor = ref<HTMLElement | null>(null)
function openMenu(id: string, e: MouseEvent) {
  menuFor.value = id
  menuAnchor.value = e.currentTarget as HTMLElement
}

const list = ref<HTMLElement | null>(null)
useSortable(
  list,
  (m: SortMove) => emit('move', m.id, props.parent, m.beforeId || '^'),
  { group: { name: `pages-${props.ws}`, pull: false, put: false }, handle: '.tree-row', disabled: !props.canEdit, fallbackOnBody: true, swapThreshold: 0.6 },
)

function go(n: PageNode) {
  router.push({ name: 'ws-page', params: { ws: props.ws, page: n.id } })
}
</script>

<template>
  <div ref="list" :data-group="parent ?? 'root'">
    <div v-for="n in nodes" :key="n.id" :data-id="n.id">
      <div
        class="tree-row side-item group/row !h-[28px]"
        :class="{ on: activeId === n.id }"
        :style="{ paddingLeft: 6 + depth * 14 + 'px' }"
        @click="go(n)"
      >
        <button
          class="grid h-4 w-4 shrink-0 place-items-center rounded text-text-3 hover:bg-fill"
          :class="{ invisible: !n.children.length }"
          :aria-label="t('pages.expand')"
          @click.stop="pages.toggle(n.id)"
        >
          <ChevronRight class="h-3 w-3 !text-text-3 transition-transform" :class="{ 'rotate-90': pages.open.has(n.id) }" :stroke-width="2.5" />
        </button>
        <span v-if="n.icon" class="w-4 shrink-0 text-center text-[13px] leading-none">{{ n.icon }}</span>
        <FileText v-else class="h-3.5 w-3.5 shrink-0 !text-text-3" />
        <span class="min-w-0 flex-1 truncate">{{ n.title || t('pages.untitled') }}</span>
        <!-- Shown on hover — and while this row's menu is open, which is
             anchored to its ⋯ (hidden, the menu would lose its place). -->
        <span v-if="canEdit" class="ml-auto hidden shrink-0 items-center gap-0.5 group-hover/row:flex" :class="{ '!flex': menuFor === n.id }">
          <button class="grid h-5 w-5 place-items-center rounded hover:bg-fill" :title="t('pages.addSub')" @click.stop="emit('add', n.id)">
            <Plus class="h-3.5 w-3.5 !text-text-2" />
          </button>
          <button class="grid h-5 w-5 place-items-center rounded hover:bg-fill" :title="t('common.more')" @click.stop="openMenu(n.id, $event)">
            <MoreHorizontal class="h-3.5 w-3.5 !text-text-2" />
          </button>
        </span>
      </div>
      <PageTree
        v-if="n.children.length && pages.open.has(n.id)"
        :ws="ws"
        :nodes="n.children"
        :depth="depth + 1"
        :parent="n.id"
        :can-edit="canEdit"
        @add="(p) => emit('add', p)"
        @move="(id, p, a) => emit('move', id, p, a)"
      />
    </div>
    <PageMenu v-if="menuFor" :page-id="menuFor" :ws="ws" :anchor="menuAnchor" @close="menuFor = ''" />
  </div>
</template>

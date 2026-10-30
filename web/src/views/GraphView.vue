<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { Search, X } from 'lucide-vue-next'
import GraphCanvas from '@/components/graph/GraphCanvas.vue'
import { useFillHeight } from '@/components/calendar/useFillHeight'
import { KIND_COLOR, NODE_KINDS, loadGraph, neighbourhood, only, routeOf, type Graph, type GraphNode, type NodeKind } from '@/lib/graph'
import { onLiveEvent } from '@/lib/live'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// A workspace's graph: everything in it and how it connects. Kinds can be
// hidden (their dots and links), a name found (the rest dims); ?focus=
// shows only what's near one thing (1–3 links), with a way to see it all.
const route = useRoute()
const router = useRouter()
const wss = useWorkspacesStore()
const toast = useToastStore()
const { t } = useI18n()

const ws = computed(() => String(route.params.ws))
const workspace = computed(() => wss.byId(ws.value))
const graph = ref<Graph>({ nodes: [], edges: [] })
const loaded = ref(false)
async function load(fresh = false) {
  try {
    graph.value = await loadGraph(ws.value, fresh)
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    loaded.value = true
  }
}
watch(ws, () => void load(), { immediate: true })
let timer: ReturnType<typeof setTimeout> | undefined
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) !== ws.value) return
  clearTimeout(timer)
  timer = setTimeout(() => void load(true), 800)
})
onUnmounted(() => (off(), clearTimeout(timer)))

const KEY = 'orbit_graph_hidden'
const hidden = ref<Set<NodeKind>>(new Set((() => {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? '[]') as NodeKind[]
  } catch {
    return []
  }
})()))
function toggle(k: NodeKind) {
  const s = new Set(hidden.value)
  if (s.has(k)) s.delete(k)
  else s.add(k)
  hidden.value = s
  try {
    localStorage.setItem(KEY, JSON.stringify([...s]))
  } catch {
    /* private mode */
  }
}
const counts = computed(() => {
  const c: Partial<Record<NodeKind, number>> = {}
  for (const n of graph.value.nodes) c[n.kind] = (c[n.kind] ?? 0) + 1
  return c
})
const present = computed(() => NODE_KINDS.filter((k) => counts.value[k]))

const focus = computed(() => (typeof route.query.focus === 'string' && graph.value.nodes.some((n) => n.id === route.query.focus) ? route.query.focus : ''))
const depth = computed(() => Math.min(3, Math.max(1, Number(route.query.depth) || 2)))
const focusNode = computed(() => graph.value.nodes.find((n) => n.id === focus.value))
const shown = computed(() => {
  const g = focus.value ? neighbourhood(graph.value, focus.value, depth.value) : graph.value
  return only(g, new Set(NODE_KINDS.filter((k) => !hidden.value.has(k) || k === focusNode.value?.kind)))
})
const setDepth = (d: number) => router.replace({ query: { ...route.query, depth: String(d) } })
const showAll = () => router.replace({ query: {} })
const search = ref('')

function open(n: GraphNode) {
  const to = routeOf(ws.value, n)
  if (to) void router.push(to)
}

const fill = ref<HTMLElement | null>(null)
useFillHeight(fill, 420)
</script>

<template>
  <div class="graph-view">
    <header class="mb-4 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('graph.title') }}</h1>
        <p class="subhead mt-0.5">{{ workspace?.name }}</p>
      </div>
      <label class="relative w-60 max-w-full">
        <Search class="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-3" />
        <input v-model="search" class="field !pl-8" :placeholder="t('graph.find')" data-testid="graph-search" />
      </label>
    </header>
    <div class="mb-3 flex flex-wrap items-center gap-1.5">
      <button
        v-for="k in present"
        :key="k"
        type="button"
        class="chip"
        :class="{ off: hidden.has(k) }"
        :aria-pressed="!hidden.has(k)"
        :data-testid="`graph-kind-${k}`"
        @click="toggle(k)"
      >
        <span class="dot" :style="{ background: KIND_COLOR[k] }" />{{ t(`graph.kind.${k}`) }}<span class="text-text-3">{{ counts[k] }}</span>
      </button>
      <template v-if="focusNode">
        <span class="mx-1 h-4 w-px bg-border" />
        <span class="text-[12.5px] text-text-2">{{ t('graph.near', { name: focusNode.title || t('graph.untitled') }) }}</span>
        <div class="seg" role="group">
          <button v-for="d in [1, 2, 3]" :key="d" type="button" :class="{ on: depth === d }" @click="setDepth(d)">{{ d }}</button>
        </div>
        <button type="button" class="btn btn-sm" data-testid="graph-all" @click="showAll"><X class="h-3.5 w-3.5" />{{ t('graph.all') }}</button>
      </template>
    </div>
    <div ref="fill" class="graph-frame">
      <p v-if="loaded && !graph.nodes.length" class="py-24 text-center text-[13.5px] text-text-3">{{ t('graph.empty') }}</p>
      <GraphCanvas v-else-if="loaded" :graph="shown" :focus="focus || undefined" :search="search" @open="open" />
    </div>
  </div>
</template>

<style scoped>
.graph-view {
  padding: 32px clamp(16px, 4vw, 48px) 0;
}
.graph-frame {
  overflow: hidden;
  border-radius: 14px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border);
}
.chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 10px;
  border-radius: 999px;
  background: var(--fill-2);
  font-size: 12.5px;
}
.chip.off {
  opacity: 0.45;
}
.chip .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
</style>

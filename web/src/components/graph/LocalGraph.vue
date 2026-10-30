<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Waypoints } from 'lucide-vue-next'
import GraphCanvas from './GraphCanvas.vue'
import { loadGraph, neighbourhood, routeOf, type Graph, type GraphNode } from '@/lib/graph'
import { onLiveEvent } from '@/lib/live'

// The small graph beside a thing: what it links to and what links to it,
// 1–3 links deep (remembered), and the way to the workspace's whole graph.
const props = defineProps<{ ws: string; id: string }>()
const { t } = useI18n()
const router = useRouter()

const all = ref<Graph | null>(null)
async function load(fresh = false) {
  try {
    all.value = await loadGraph(props.ws, fresh)
  } catch {
    all.value = null
  }
}
watch(() => props.ws, () => void load(), { immediate: true })
let timer: ReturnType<typeof setTimeout> | undefined
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) !== props.ws) return
  clearTimeout(timer)
  timer = setTimeout(() => void load(true), 1500)
})
onUnmounted(() => (off(), clearTimeout(timer)))

const depth = ref(Number(localStorage.getItem('orbit_local_graph_depth')) || 1)
watch(depth, (d) => {
  try {
    localStorage.setItem('orbit_local_graph_depth', String(d))
  } catch {
    /* private mode */
  }
})
const near = computed(() => (all.value ? neighbourhood(all.value, props.id, depth.value) : null))
const lonely = computed(() => !!near.value && near.value.nodes.length <= 1)
function open(n: GraphNode) {
  if (n.id === props.id) return
  const to = routeOf(props.ws, n)
  if (to) void router.push(to)
}
</script>

<template>
  <section v-if="near && near.nodes.length" class="local-graph" data-testid="local-graph">
    <div class="lg-head">
      <Waypoints class="h-4 w-4 text-text-3" />
      <h3 class="flex-1 text-[13px] font-semibold">{{ t('graph.local') }}</h3>
      <div class="seg" role="group" :aria-label="t('graph.depth')">
        <button v-for="d in [1, 2, 3]" :key="d" type="button" :class="{ on: depth === d }" :aria-pressed="depth === d" @click="depth = d">{{ d }}</button>
      </div>
      <RouterLink :to="{ name: 'ws-graph', params: { ws }, query: { focus: id, depth: String(depth) } }" class="btn-plain !py-0.5 text-[12.5px]" data-testid="local-graph-open">{{ t('graph.openAll') }}</RouterLink>
    </div>
    <p v-if="lonely" class="px-3 pb-3 text-[12.5px] text-text-3">{{ t('graph.lonely') }}</p>
    <div v-else class="lg-canvas"><GraphCanvas :graph="near" :focus="id" compact @open="open" /></div>
  </section>
</template>

<style scoped>
.local-graph {
  margin-top: 16px;
  overflow: hidden;
  border-radius: 12px;
  background: var(--surface);
  box-shadow: 0 0 0 0.5px var(--border);
}
.lg-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
}
.lg-head .seg button {
  padding: 1px 9px;
  font-size: 12px;
}
.lg-canvas {
  height: 240px;
  border-top: 0.5px solid var(--line-soft, var(--border));
}
</style>

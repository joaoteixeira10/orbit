<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import Avatar from './ui/Avatar.vue'
import { activityApi, type ActivityLine } from '@/api/planner'
import { onLiveEvent } from '@/lib/live'
import { locale } from '@/lib/dates'

// A workspace's activity, newest first: a person's run of the same thing
// within two hours reads as one line ("Ana completed 3 tasks"), with what
// they were (the first few), each a link.
const props = defineProps<{ ws: string }>()
const { t } = useI18n()
const router = useRouter()

const lines = ref<ActivityLine[]>([])
const more = ref(true)

async function load() {
  try {
    const l = await activityApi.list(props.ws)
    lines.value = l
    more.value = l.length === 60
  } catch {
    lines.value = []
  }
}
async function loadMore() {
  const last = lines.value[lines.value.length - 1]
  if (!last) return
  const l = await activityApi.list(props.ws, last.created_at).catch(() => [] as ActivityLine[])
  lines.value = [...lines.value, ...l]
  more.value = l.length === 60
}
watch(() => props.ws, load, { immediate: true })
const off = onLiveEvent('ws', (e) => {
  if (String(e.ws) === props.ws && ['tasks', 'pages', 'notes', 'events', 'comments', 'members'].includes(String(e.what))) void load()
})
onUnmounted(off)

interface Group {
  key: string
  actor: string
  avatar: string
  verb: ActivityLine['verb']
  at: string
  items: ActivityLine[]
}
const groups = computed<Group[]>(() => {
  const out: Group[] = []
  for (const l of lines.value) {
    const g = out[out.length - 1]
    if (g && g.actor === l.actor_name && g.verb === l.verb && new Date(g.items[g.items.length - 1].created_at).getTime() - new Date(l.created_at).getTime() < 2 * 3600_000) {
      g.items.push(l)
    } else {
      out.push({ key: l.id, actor: l.actor_name, avatar: l.actor_avatar, verb: l.verb, at: l.created_at, items: [l] })
    }
  }
  return out
})

function when(iso: string) {
  const m = Math.floor((Date.now() - new Date(iso).getTime()) / 60000)
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto', style: 'short' })
  if (m < 60) return rtf.format(-Math.max(m, 0), 'minute')
  if (m < 1440) return rtf.format(-Math.floor(m / 60), 'hour')
  return new Intl.DateTimeFormat(locale(), { day: 'numeric', month: 'short' }).format(new Date(iso))
}
function open(l: ActivityLine) {
  if (l.link) router.push(l.link)
}
</script>

<template>
  <section class="widget card p-5" data-testid="ws-feed">
    <h2 class="widget-title" style="color: #34c759">{{ t('feed.title') }}</h2>
    <ol class="space-y-3">
      <li v-for="g in groups" :key="g.key" class="flex gap-2.5">
        <Avatar :name="g.actor || '?'" :src="g.avatar" :size="24" class="mt-0.5" />
        <div class="min-w-0 flex-1">
          <div class="flex items-baseline gap-2">
            <span class="min-w-0 flex-1 text-[13.5px]">{{ t(`feed.${g.verb}`, { actor: g.actor || '?', n: g.items.length }, g.items.length) }}</span>
            <span class="shrink-0 text-[11.5px] text-text-3">{{ when(g.at) }}</span>
          </div>
          <div v-if="g.verb !== 'member_joined'" class="mt-0.5 flex flex-wrap gap-x-2 text-[12.5px]">
            <button v-for="l in g.items.slice(0, 3)" :key="l.id" class="max-w-full truncate text-left text-accent-text hover:underline" @click="open(l)">{{ l.title || '…' }}</button>
            <span v-if="g.items.length > 3" class="text-text-3">{{ t('feed.andMore', { n: g.items.length - 3 }) }}</span>
          </div>
        </div>
      </li>
    </ol>
    <p v-if="!groups.length" class="empty-line">{{ t('feed.none') }}</p>
    <button v-else-if="more" class="btn-plain mt-3" @click="loadMore">{{ t('feed.more') }}</button>
  </section>
</template>

<style scoped>
/* As the overview's other cards. */
.widget-title {
  margin-bottom: 8px;
  font: 700 20px var(--font-display);
  letter-spacing: -0.015em;
}
.empty-line {
  padding: 10px 0;
  font-size: 14px;
  color: var(--text-3);
}
</style>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Bell, AlarmClock, UserCheck, Users, Sparkles, Sun, CheckCheck, BellOff, Clock } from 'lucide-vue-next'
import RemindPicker from '@/components/ui/RemindPicker.vue'
import { useToastStore } from '@/stores/toast'
import { fmtDate, fmtTime } from '@/lib/dates'
import Popover from '@/components/ui/Popover.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import { useInboxStore } from '@/stores/inbox'
import { useWorkspacesStore } from '@/stores/workspaces'
import { renderNotification } from '@/lib/inbox'
import { ago } from '@/components/notes/noteModel'
import { locale } from '@/lib/dates'
import type { InboxItem } from '@/api/planner'

// The bell: how many notifications are unread, and a panel with the latest
// (new ones first). A click opens what it's about and marks it read.
withDefaults(defineProps<{ placement?: 'right-start' | 'bottom-end' }>(), { placement: 'right-start' })
const inbox = useInboxStore()
const wss = useWorkspacesStore()
const router = useRouter()
const { t } = useI18n()

const anchor = ref<HTMLElement | null>(null)
const open = ref(false)
onMounted(() => void inbox.load().catch(() => {}))

const kindIcon = { reminder: AlarmClock, assigned: UserCheck, workspace: Users, activity: Sparkles, digest: Sun }
const kindColor = { reminder: '#FF9500', assigned: '#007AFF', workspace: '#34C759', activity: '#AF52DE', digest: '#FFCC00' }

const now = ref(Date.now())
function toggle() {
  open.value = !open.value
  now.value = Date.now()
  if (open.value && !inbox.loaded) void inbox.load().catch(() => {})
}
const fresh = computed(() => inbox.items.filter((n) => !n.read))
const earlier = computed(() => inbox.items.filter((n) => n.read))
const when = (n: InboxItem) => ago(n.updated_at, locale(), now.value)

function go(n: InboxItem) {
  open.value = false
  void inbox.markRead([n.id])
  if (n.link) void router.push(n.link)
}
const badge = computed(() => (inbox.unread > 99 ? '99+' : String(inbox.unread)))

// Snooze: "remind me again" in an hour, tomorrow…
const toast = useToastStore()
const snoozing = ref<{ id: string; el: HTMLElement } | null>(null)
async function snooze(at: string) {
  const s = snoozing.value
  snoozing.value = null
  if (!s || !at) return
  try {
    await inbox.snooze(s.id, at)
    const d = new Date(at)
    toast.info(t('snooze.done', { when: `${fmtDate(d, { weekday: 'short' })} ${fmtTime(d)}` }))
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <button
    ref="anchor"
    class="icon-btn relative h-7 w-7 shrink-0"
    :title="t('inbox.open')"
    :aria-label="inbox.unread ? `${t('inbox.open')} — ${t('inbox.unread', { n: inbox.unread })}` : t('inbox.open')"
    :aria-expanded="open"
    @click="toggle"
  >
    <Bell class="h-[17px] w-[17px]" />
    <span v-if="inbox.unread" class="badge">{{ badge }}</span>
  </button>

  <Popover v-if="open" :anchor="anchor" :placement="placement" :width="380" @close="open = false">
    <div class="flex items-center gap-2 border-b border-line-soft px-4 py-3">
      <h2 class="flex-1 text-[15px] font-semibold">{{ t('inbox.title') }}</h2>
      <button v-if="inbox.unread" class="btn-plain !text-[13px]" @click="inbox.markAll()"><CheckCheck class="h-4 w-4" />{{ t('inbox.markAll') }}</button>
    </div>
    <div class="max-h-[min(70vh,560px)] overflow-y-auto overscroll-contain py-1">
      <div v-if="!inbox.items.length" class="px-6 py-10 text-center">
        <BellOff class="mx-auto mb-3 h-8 w-8 text-text-faint" :stroke-width="1.5" />
        <p class="text-[14px] font-semibold">{{ t('inbox.empty') }}</p>
        <p class="mt-1 text-[12.5px] text-text-3">{{ t('inbox.emptyHint') }}</p>
      </div>
      <template v-for="[label, list] in [[t('inbox.new'), fresh], [t('inbox.earlier'), earlier]] as const" :key="label">
        <template v-if="list.length">
          <div class="cap px-4 pb-1 pt-2.5">{{ label }}</div>
          <div v-for="n in list" :key="n.id" class="item" :class="{ unread: !n.read }" role="button" tabindex="0" @click="go(n)" @keydown.enter="go(n)">
            <span class="kind" :style="{ background: kindColor[n.kind] }"><component :is="kindIcon[n.kind]" class="h-[15px] w-[15px]" :stroke-width="2.3" /></span>
            <span class="min-w-0 flex-1">
              <span class="block text-[13.5px] leading-snug" :class="n.read ? 'font-medium' : 'font-semibold'">{{ renderNotification(n).title }}</span>
              <span class="body">{{ renderNotification(n).body }}</span>
              <span class="mt-1 flex items-center gap-1.5 text-[11.5px] text-text-3">
                <WsIcon v-if="wss.byId(n.workspace_id)" :icon="wss.byId(n.workspace_id)!.icon" :color="wss.byId(n.workspace_id)!.color" :size="14" />
                {{ when(n) }}
              </span>
            </span>
            <span v-if="!n.read" class="dot" />
            <button class="snooze icon-btn h-7 w-7" :title="t('snooze.action')" :aria-label="t('snooze.action')" @click.stop="snoozing = { id: n.id, el: $event.currentTarget as HTMLElement }">
              <Clock class="h-4 w-4" />
            </button>
          </div>
        </template>
      </template>
      <div v-if="inbox.more" class="px-4 py-2 text-center">
        <button class="btn-plain !text-[13px]" @click="inbox.loadMore()">{{ t('inbox.more') }}</button>
      </div>
    </div>
  </Popover>
  <Popover v-if="snoozing" :anchor="snoozing.el" placement="bottom-end" :width="260" @close="snoozing = null">
    <RemindPicker snooze @pick="snooze" />
  </Popover>
</template>

<style scoped>
.badge {
  position: absolute;
  top: -3px;
  right: -4px;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--crit);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  line-height: 16px;
  text-align: center;
  box-shadow: 0 0 0 2px var(--side-bg, var(--bg));
}
.item {
  display: flex;
  align-items: flex-start;
  gap: 11px;
  width: calc(100% - 12px);
  margin: 0 6px;
  padding: 9px 10px;
  border-radius: 10px;
  text-align: left;
}
.item:hover {
  background: var(--surface-hover);
}
.item {
  position: relative;
  cursor: pointer;
}
.item .snooze {
  position: absolute;
  right: 8px;
  bottom: 6px;
  opacity: 0;
}
.item:hover .snooze,
.item:focus-within .snooze {
  opacity: 1;
}
@media (hover: none) {
  .item .snooze {
    opacity: 0.6;
  }
}
.kind {
  display: grid;
  flex: none;
  place-items: center;
  width: 28px;
  height: 28px;
  margin-top: 1px;
  border-radius: 8px;
  color: #fff;
}
.body {
  display: -webkit-box;
  margin-top: 1px;
  overflow: hidden;
  font-size: 12.5px;
  line-height: 1.4;
  color: var(--text-2);
  white-space: pre-line;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
}
.dot {
  flex: none;
  width: 8px;
  height: 8px;
  margin-top: 6px;
  border-radius: 999px;
  background: var(--accent);
}
</style>

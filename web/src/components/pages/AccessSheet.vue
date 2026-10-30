<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Lock, Users } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Avatar from '@/components/ui/Avatar.vue'
import { pagesApi, type Page } from '@/api/planner'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// Who can see a page — and what's under it: everyone in the workspace, or
// only some of its people (you always). It's never wider than a page above
// it: under a restricted page, only those both let in see it.
const props = defineProps<{ open: boolean; page: Page; ws: string }>()
const emit = defineEmits<{ close: []; changed: [p: Page] }>()
const { t } = useI18n()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()

const only = ref(false)
const chosen = ref<Set<string>>(new Set())
watch(
  () => props.open,
  (o) => {
    if (!o) return
    only.value = !!props.page.restricted
    chosen.value = new Set(props.page.allowed_users ?? [])
    if (!wss.members[props.ws]) void wss.loadMembers(props.ws).catch(() => undefined)
  },
  { immediate: true },
)
const me = computed(() => auth.user?.id ?? '')
const people = computed(() => (wss.members[props.ws] ?? []).map((m) => ({ id: m.user_id, name: m.display_name || m.username, role: m.role })))
// A page above it that's restricted (it narrows this one too).
const above = computed(() => {
  const all = pages.byWs[props.ws] ?? []
  let cur = all.find((p) => p.id === props.page.id)?.parent_id
  const seen = new Set<string>()
  while (cur && !seen.has(cur)) {
    seen.add(cur)
    const p = all.find((x) => x.id === cur)
    if (p?.restricted) return p
    cur = p?.parent_id ?? null
  }
  return null
})
function flip(id: string) {
  const s = new Set(chosen.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  chosen.value = s
}
const busy = ref(false)
async function save() {
  busy.value = true
  try {
    const p = await pagesApi.setAccess(props.page.id, { restricted: only.value, user_ids: [...chosen.value] })
    pages.patch({ id: p.id, restricted: p.restricted })
    emit('changed', p)
    emit('close')
    toast.success(only.value ? t('access.nowOnly', { n: (p.allowed_users ?? []).length }) : t('access.nowAll'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Overlay :open="open" :title="t('access.title')" @close="emit('close')">
    <p class="mb-3 text-[13px] leading-relaxed text-text-2">{{ t('access.lead') }}</p>
    <div class="flex flex-col gap-2">
      <label class="choice" :class="{ on: !only }">
        <input v-model="only" type="radio" :value="false" name="access" data-testid="access-all" />
        <Users class="h-4 w-4 text-text-2" />
        <span><b>{{ t('access.everyone') }}</b><br /><small>{{ t('access.everyoneHint') }}</small></span>
      </label>
      <label class="choice" :class="{ on: only }">
        <input v-model="only" type="radio" :value="true" name="access" data-testid="access-only" />
        <Lock class="h-4 w-4 text-text-2" />
        <span><b>{{ t('access.only') }}</b><br /><small>{{ t('access.onlyHint') }}</small></span>
      </label>
    </div>
    <div v-if="only" class="mt-3 flex flex-col divide-y divide-border rounded-xl border border-border">
      <label v-for="p in people" :key="p.id" class="flex items-center gap-2.5 px-3 py-2 text-[13.5px]">
        <input type="checkbox" :checked="p.id === me || chosen.has(p.id)" :disabled="p.id === me" :data-testid="`access-person-${p.name}`" @change="flip(p.id)" />
        <Avatar :name="p.name" :size="22" />
        <span class="min-w-0 flex-1 truncate">{{ p.name }}<span v-if="p.id === me" class="text-text-3"> · {{ t('access.you') }}</span></span>
        <span class="text-[11.5px] text-text-3">{{ t(`roles.${p.role}`) }}</span>
      </label>
    </div>
    <p v-if="above" class="mt-3 rounded-lg bg-fill-2 px-3 py-2 text-[12px] text-text-2">{{ t('access.above', { name: above.title || t('pages.untitled') }) }}</p>
    <template #footer>
      <button type="button" class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button type="button" class="btn-primary" :disabled="busy" data-testid="access-save" @click="save">{{ t('common.save') }}</button>
    </template>
  </Overlay>
</template>

<style scoped>
.choice {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 13.5px;
  cursor: pointer;
}
.choice.on {
  box-shadow: 0 0 0 2px var(--accent);
}
.choice input {
  margin-top: 3px;
}
.choice small {
  color: var(--text-3);
  font-size: 12px;
}
</style>

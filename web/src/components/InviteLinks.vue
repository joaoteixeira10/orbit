<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Link2, Trash2 } from 'lucide-vue-next'
import CopyButton from './ui/CopyButton.vue'
import { invitesApi, type Invite } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { fmtDate } from '@/lib/dates'

// A workspace's invite links (its owners): make one — a role, how long it
// lasts, how many people can use it — copy it (shown once), revoke it.
const props = defineProps<{ wsId: string }>()
const { t } = useI18n()
const toast = useToastStore()

const links = ref<Invite[]>([])
const role = ref<'editor' | 'viewer'>('editor')
const days = ref(7)
const uses = ref(1)
const fresh = ref('') // the link just made, until the sheet closes
const busy = ref(false)

async function load() {
  try {
    links.value = await invitesApi.list(props.wsId)
  } catch {
    links.value = []
  }
}
watch(() => props.wsId, () => ((fresh.value = ''), load()), { immediate: true })

async function make() {
  busy.value = true
  try {
    const r = await invitesApi.create(props.wsId, { role: role.value, days: days.value, max_uses: uses.value })
    fresh.value = location.origin + r.path
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
async function revoke(inv: Invite) {
  try {
    await invitesApi.revoke(props.wsId, inv.id)
    links.value = links.value.filter((x) => x.id !== inv.id)
    toast.success(t('invites.revoked'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <section class="space-y-2">
    <h3 class="subhead px-1">{{ t('invites.title') }}</h3>
    <div class="group">
      <div class="group-row flex-wrap gap-y-2">
        <label class="inline-flex items-center gap-2 text-[13px] text-text-2">
          {{ t('invites.role') }}
          <select v-model="role" class="input !w-auto !py-1 text-[13px]">
            <option value="editor">{{ t('roles.editor') }}</option>
            <option value="viewer">{{ t('roles.viewer') }}</option>
          </select>
        </label>
        <label class="inline-flex items-center gap-2 text-[13px] text-text-2">
          {{ t('invites.days') }}
          <select v-model.number="days" class="input !w-auto !py-1 text-[13px]">
            <option v-for="n in [1, 7, 14, 30]" :key="n" :value="n">{{ t('invites.daysN', { n }, n) }}</option>
          </select>
        </label>
        <label class="inline-flex items-center gap-2 text-[13px] text-text-2">
          {{ t('invites.uses') }}
          <select v-model.number="uses" class="input !w-auto !py-1 text-[13px]">
            <option v-for="n in [1, 5, 10, 25, 100]" :key="n" :value="n">{{ t('invites.usesN', { n }, n) }}</option>
          </select>
        </label>
        <button class="btn ml-auto" :disabled="busy" @click="make"><Link2 class="h-4 w-4" />{{ t('invites.make') }}</button>
      </div>
      <div v-if="fresh" class="group-row flex-col !items-stretch gap-2">
        <span class="text-[12.5px] text-text-2">{{ t('invites.created') }}</span>
        <div class="flex items-center gap-2">
          <input class="input flex-1 font-mono text-[12.5px]" :value="fresh" readonly data-testid="invite-url" @focus="($event.target as HTMLInputElement).select()" />
          <CopyButton :value="fresh" :title="t('invites.copy')" />
        </div>
      </div>
      <div v-for="inv in links" :key="inv.id" class="group-row">
        <Link2 class="h-4 w-4 shrink-0 text-text-3" />
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14px]"><span class="font-mono text-[12.5px] text-text-3">/join/{{ inv.prefix }}…</span> · {{ t(`roles.${inv.role}`) }}</span>
          <span class="block truncate text-[12px] text-text-3">
            {{ t('invites.used', { uses: inv.uses, max: inv.max_uses }) }} · {{ t('invites.expires', { date: fmtDate(new Date(inv.expires_at)) }) }}
          </span>
        </span>
        <button class="icon-btn !text-danger" :title="t('invites.revoke')" :aria-label="t('invites.revoke')" @click="revoke(inv)"><Trash2 class="h-4 w-4" /></button>
      </div>
      <div v-if="!links.length && !fresh" class="group-row text-[13px] text-text-3">{{ t('invites.none') }}</div>
    </div>
    <p class="footnote px-1">{{ t('invites.hint') }}</p>
  </section>
</template>

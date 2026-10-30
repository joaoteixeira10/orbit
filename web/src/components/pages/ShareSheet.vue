<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Link2, Plus, Trash2 } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Toggle from '@/components/ui/Toggle.vue'
import CopyButton from '@/components/ui/CopyButton.vue'
import { shareApi, type ShareLink } from '@/lib/shareApi'
import { useToastStore } from '@/stores/toast'

// A page's guest links: each opens the page (and, if chosen, the pages
// inside it) to anyone who has it, without an account — to read, or also to
// edit; for good, or until a day. Taking one back stops it at once.
const props = defineProps<{ open: boolean; pageId: string; editable: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const toast = useToastStore()

const links = ref<ShareLink[]>([])
const loading = ref(false)
watch(
  () => props.open,
  async (o) => {
    if (!o) return
    loading.value = true
    try {
      links.value = await shareApi.links(props.pageId)
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)
async function make() {
  try {
    links.value = [...links.value, await shareApi.createLink(props.pageId, { mode: 'read', subpages: true })]
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function change(l: ShareLink, b: { mode?: 'read' | 'edit'; subpages?: boolean; expires_in_days?: number }) {
  try {
    const x = await shareApi.updateLink(l.id, b)
    links.value = links.value.map((y) => (y.id === l.id ? x : y))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function remove(l: ShareLink) {
  try {
    await shareApi.removeLink(l.id)
    links.value = links.value.filter((y) => y.id !== l.id)
    toast.success(t('share.removed'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
const until = (iso?: string) => (iso ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(iso)) : '')
</script>

<template>
  <Overlay :open="open" :title="t('share.title')" size="lg" @close="emit('close')">
    <p class="mb-4 text-[13px] leading-relaxed text-text-2">{{ t('share.lead') }}</p>
    <p v-if="loading" class="py-4 text-center text-[13px] text-text-3">{{ t('common.loading') }}</p>
    <p v-else-if="!links.length" class="mb-3 rounded-xl bg-fill-2 px-4 py-3 text-[13px] text-text-3">{{ t('share.none') }}</p>
    <div v-for="l in links" :key="l.id" class="share-row" data-testid="share-link">
      <div class="flex items-center gap-2">
        <Link2 class="h-4 w-4 shrink-0 text-text-3" />
        <input class="field min-w-0 flex-1 font-mono !text-[12px]" :value="l.url" readonly data-testid="share-url" @focus="($event.target as HTMLInputElement).select()" />
        <CopyButton :value="l.url" />
        <button v-if="editable" type="button" class="icon-btn h-8 w-8 text-text-3" :aria-label="t('share.remove')" data-testid="share-remove" @click="remove(l)"><Trash2 class="h-4 w-4" /></button>
      </div>
      <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-[12.5px]">
        <div class="seg" role="group">
          <button type="button" :class="{ on: l.mode === 'read' }" :disabled="!editable" @click="change(l, { mode: 'read' })">{{ t('share.read') }}</button>
          <button type="button" :class="{ on: l.mode === 'edit' }" :disabled="!editable" data-testid="share-edit" @click="change(l, { mode: 'edit' })">{{ t('share.edit') }}</button>
        </div>
        <label class="flex items-center gap-2"><Toggle :model-value="l.subpages" :disabled="!editable" @update:model-value="(v: boolean) => change(l, { subpages: v })" />{{ t('share.subpages') }}</label>
        <label class="flex items-center gap-2 text-text-2">
          {{ t('share.until') }}
          <select class="input !w-auto !py-1 text-[12.5px]" :disabled="!editable" @change="change(l, { expires_in_days: Number(($event.target as HTMLSelectElement).value) })">
            <option value="0" :selected="!l.expires_at">{{ t('share.forever') }}</option>
            <option value="1">{{ t('share.days', { n: 1 }, 1) }}</option>
            <option value="7">{{ t('share.days', { n: 7 }, 7) }}</option>
            <option value="30">{{ t('share.days', { n: 30 }, 30) }}</option>
            <option v-if="l.expires_at" value="-1" selected disabled>{{ until(l.expires_at) }}</option>
          </select>
        </label>
      </div>
    </div>
    <template #footer>
      <button v-if="editable" type="button" class="btn-primary" data-testid="share-new" @click="make"><Plus class="h-4 w-4" />{{ t('share.new') }}</button>
    </template>
  </Overlay>
</template>

<style scoped>
.share-row {
  margin-bottom: 10px;
  padding: 12px;
  border-radius: 12px;
  box-shadow: 0 0 0 0.5px var(--border);
}
</style>

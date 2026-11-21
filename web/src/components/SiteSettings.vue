<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink, FileText, Lock } from 'lucide-vue-next'
import { siteAdminApi, type SiteSettings } from '@/lib/shareApi'
import { usePagesStore } from '@/stores/pages'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import Toggle from '@/components/ui/Toggle.vue'
import Checkbox from '@/components/ui/Checkbox.vue'
import CopyButton from '@/components/ui/CopyButton.vue'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'

// Workspace settings → Website (owners): publish some of the workspace's
// pages as a read-only website — which top-level pages (each with what's
// under it), its name in the address, a domain of its own, and whether
// search engines may list it. Restricted and locked pages never go on it.
const props = defineProps<{ wsId: string; wsName: string }>()
const { t } = useI18n()
const pages = usePagesStore()
const auth = useAuthStore()
const toast = useToastStore()

const site = ref<SiteSettings | null>(null)
const loaded = ref(false)
const published = ref(false)
const slug = ref('')
const title = ref('')
const domain = ref('')
const picked = ref(new Set<string>())
const indexed = ref(false)
const saving = ref(false)

const off = computed(() => auth.features.public_sites === false)
const tops = computed(() => pages.tree(props.wsId))
const origin = window.location.origin
const url = computed(() => `${origin}/site/${site.value?.slug ?? slug.value}`)
const domainUrl = computed(() => (site.value?.domain ? `https://${site.value.domain}` : ''))

onMounted(async () => {
  if (!pages.byWs[props.wsId]) await pages.load(props.wsId).catch(() => {})
  try {
    const r = await siteAdminApi.get(props.wsId)
    fill(r.site, r.suggested_slug)
  } catch (e: any) {
    toast.error(e.message)
  }
  loaded.value = true
})
function fill(s: SiteSettings | null, suggested = '') {
  site.value = s
  published.value = s?.published ?? false
  slug.value = s?.slug ?? suggested
  title.value = s?.title ?? ''
  domain.value = s?.domain ?? ''
  picked.value = new Set(s?.page_ids ?? [])
  indexed.value = s?.indexed ?? false
}
function toggle(id: string, on: boolean) {
  const next = new Set(picked.value)
  if (on) next.add(id)
  else next.delete(id)
  picked.value = next
}
async function save() {
  if (saving.value) return
  saving.value = true
  try {
    const s = await siteAdminApi.save(props.wsId, {
      published: published.value,
      slug: slug.value.trim().toLowerCase(),
      title: title.value,
      domain: domain.value,
      // In the sidebar's order.
      page_ids: tops.value.filter((p) => picked.value.has(p.id)).map((p) => p.id),
      indexed: indexed.value,
    })
    fill(s)
    toast.success(s.published ? t('site.admin.live') : t('site.admin.saved'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="space-y-5" data-testid="site-settings">
    <p v-if="off" class="text-[13px] text-text-3">{{ t('site.admin.off') }}</p>
    <template v-else-if="loaded">
      <p class="text-[13px] leading-relaxed text-text-2">{{ t('site.admin.intro') }}</p>

      <div class="flex items-center justify-between gap-4">
        <div>
          <div class="text-[14px] font-semibold">{{ t('site.admin.publish') }}</div>
          <div class="text-[12px] text-text-3">{{ t('site.admin.publishHint') }}</div>
        </div>
        <Toggle v-model="published" data-testid="site-published" />
      </div>

      <div v-if="site?.published" class="rounded-xl border border-border px-3 py-2.5 text-[13px]">
        <div class="flex items-center gap-2">
          <a :href="url" target="_blank" rel="noopener" class="min-w-0 flex-1 truncate text-accent hover:underline" data-testid="site-url">{{ url }}</a>
          <CopyButton :value="url" />
          <a :href="url" target="_blank" rel="noopener" class="text-text-3 hover:text-text" :aria-label="t('site.admin.open')"><ExternalLink class="h-4 w-4" /></a>
        </div>
        <div v-if="domainUrl" class="mt-1 flex items-center gap-2">
          <a :href="domainUrl" target="_blank" rel="noopener" class="min-w-0 flex-1 truncate text-accent hover:underline">{{ domainUrl }}</a>
          <CopyButton :value="domainUrl" />
        </div>
      </div>

      <label class="block">
        <span class="text-[13px] font-semibold">{{ t('site.admin.name') }}</span>
        <div class="mt-1.5 flex items-center gap-1.5">
          <span class="shrink-0 text-[13px] text-text-3">/site/</span>
          <Input v-model="slug" maxlength="40" spellcheck="false" autocapitalize="off" data-testid="site-slug" />
        </div>
        <span class="mt-1 block text-[12px] text-text-3">{{ t('site.admin.nameHint') }}</span>
      </label>

      <label class="block">
        <span class="text-[13px] font-semibold">{{ t('site.admin.title') }}</span>
        <Input v-model="title" class="mt-1.5" maxlength="200" :placeholder="wsName" data-testid="site-title-input" />
      </label>

      <div>
        <div class="text-[13px] font-semibold">{{ t('site.admin.pages') }}</div>
        <div class="mb-2 text-[12px] text-text-3">{{ t('site.admin.pagesHint') }}</div>
        <div class="max-h-64 overflow-y-auto rounded-xl border border-border p-1.5">
          <p v-if="!tops.length" class="px-2 py-1.5 text-[13px] text-text-3">{{ t('site.admin.noPages') }}</p>
          <div v-for="p in tops" :key="p.id" class="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13.5px]" :class="{ 'opacity-50': p.restricted }">
            <Checkbox
              :model-value="picked.has(p.id) && !p.restricted"
              :disabled="p.restricted"
              :aria-label="p.title || t('pages.untitled')"
              :data-testid="`site-page-${p.id}`"
              @update:model-value="(v: boolean) => toggle(p.id, v)"
            />
            <span v-if="p.icon" class="w-4 shrink-0 text-center">{{ p.icon }}</span>
            <FileText v-else class="h-4 w-4 shrink-0 text-text-3" />
            <span class="min-w-0 flex-1 truncate">{{ p.title || t('pages.untitled') }}</span>
            <span v-if="p.restricted" class="flex shrink-0 items-center gap-1 text-[12px] text-text-3"><Lock class="h-3 w-3" />{{ t('site.admin.restricted') }}</span>
          </div>
        </div>
      </div>

      <label class="block">
        <span class="text-[13px] font-semibold">{{ t('site.admin.domain') }}</span>
        <Input v-model="domain" class="mt-1.5" placeholder="docs.example.org" spellcheck="false" autocapitalize="off" data-testid="site-domain" />
        <span class="mt-1 block text-[12px] text-text-3">{{ t('site.admin.domainHint') }}</span>
      </label>

      <div class="flex items-center justify-between gap-4">
        <div>
          <div class="text-[13px] font-semibold">{{ t('site.admin.indexed') }}</div>
          <div class="text-[12px] text-text-3">{{ t('site.admin.indexedHint') }}</div>
        </div>
        <Toggle v-model="indexed" data-testid="site-indexed" />
      </div>

      <div><Button size="sm" :disabled="saving" data-testid="site-save" @click="save">{{ t('common.save') }}</Button></div>
    </template>
  </div>
</template>

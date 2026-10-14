<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Star, StarOff, Copy, Link, FolderInput, Trash2, ChevronLeft, FileText, CornerDownRight } from 'lucide-vue-next'
import Popover from './ui/Popover.vue'
import { pagesApi } from '@/api/planner'
import { usePagesStore } from '@/stores/pages'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// A page's actions: favourite, duplicate, copy link, move to another parent,
// move to the trash.
const props = defineProps<{ pageId: string; ws: string; anchor: HTMLElement | null }>()
const emit = defineEmits<{ close: [] }>()
const pages = usePagesStore()
const wss = useWorkspacesStore()
const toast = useToastStore()
const router = useRouter()
const { t } = useI18n()

const page = computed(() => pages.find(props.pageId))
const picking = ref(false)

// Pages it can move under: not itself or its descendants.
const targets = computed(() => {
  const all = pages.byWs[props.ws] ?? []
  const banned = new Set([props.pageId])
  let grew = true
  while (grew) {
    grew = false
    for (const p of all) {
      if (p.parent_id && banned.has(p.parent_id) && !banned.has(p.id)) {
        banned.add(p.id)
        grew = true
      }
    }
  }
  return all.filter((p) => !banned.has(p.id)).sort((a, b) => a.title.localeCompare(b.title))
})

async function run(fn: () => Promise<unknown>) {
  try {
    await fn()
  } catch (e: any) {
    toast.error(e.message)
  }
  emit('close')
}
const favorite = () =>
  run(async () => {
    const on = !page.value?.favorite
    await pagesApi.favorite(props.pageId, on)
    pages.patch({ id: props.pageId, favorite: on })
    await wss.loadFavorites()
  })
const duplicate = () =>
  run(async () => {
    const p = await pagesApi.duplicate(props.pageId)
    await pages.load(props.ws)
    router.push({ name: 'ws-page', params: { ws: props.ws, page: p.id } })
  })
const copyLink = () =>
  run(async () => {
    await navigator.clipboard.writeText(`${location.origin}/w/${props.ws}/p/${props.pageId}`)
    toast.success(t('pages.linkCopied'))
  })
const moveTo = (parent: string | null) =>
  run(async () => {
    await pagesApi.move(props.pageId, parent, '')
    if (parent) pages.toggle(parent, true)
    await pages.load(props.ws)
  })
const trash = () =>
  run(async () => {
    await pagesApi.trashPage(props.pageId)
    await pages.load(props.ws)
    await wss.loadFavorites()
    toast.success(t('pages.trashed'))
    if (router.currentRoute.value.params.page === props.pageId) router.push({ name: 'ws-pages', params: { ws: props.ws } })
  })
</script>

<template>
  <Popover :anchor="anchor" :width="240" placement="right-start" @close="emit('close')">
    <template v-if="!picking">
      <button class="menu-item" @click="favorite">
        <component :is="page?.favorite ? StarOff : Star" class="h-4 w-4 text-text-2" />
        {{ page?.favorite ? t('pages.unfavorite') : t('pages.favorite') }}
      </button>
      <button class="menu-item" @click="duplicate"><Copy class="h-4 w-4 text-text-2" />{{ t('pages.duplicate') }}</button>
      <button class="menu-item" @click="copyLink"><Link class="h-4 w-4 text-text-2" />{{ t('pages.copyLink') }}</button>
      <button class="menu-item" @click="picking = true"><FolderInput class="h-4 w-4 text-text-2" />{{ t('pages.moveTo') }}</button>
      <div class="menu-sep" />
      <button class="menu-item danger" @click="trash"><Trash2 class="h-4 w-4" />{{ t('pages.toTrash') }}</button>
    </template>
    <template v-else>
      <button class="menu-item text-text-2" @click="picking = false"><ChevronLeft class="h-4 w-4" />{{ t('pages.moveTo') }}</button>
      <div class="menu-sep" />
      <div class="max-h-72 overflow-y-auto">
        <button class="menu-item" @click="moveTo(null)"><CornerDownRight class="h-4 w-4 text-text-2" />{{ t('pages.topLevel') }}</button>
        <button v-for="p in targets" :key="p.id" class="menu-item" @click="moveTo(p.id)">
          <span v-if="p.icon" class="w-4 text-center">{{ p.icon }}</span>
          <FileText v-else class="h-4 w-4 text-text-3" />
          <span class="truncate">{{ p.title || t('pages.untitled') }}</span>
        </button>
      </div>
    </template>
  </Popover>
</template>

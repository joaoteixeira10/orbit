<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { notesApi } from '@/api/planner'
import { useToastStore } from '@/stores/toast'
import { shareNote } from '@/lib/share'

// /share: what another app shared (title, text, url) is saved as a Quick
// Note, which then opens. Android's share sheet comes here through the web
// manifest's share_target; an iPhone shortcut can open the same address.
const route = useRoute()
const router = useRouter()
const toast = useToastStore()
const { t } = useI18n()
const failed = ref('')

const q = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '')

onMounted(async () => {
  const note = shareNote({ title: q('title'), text: q('text'), url: q('url') })
  if (!note) {
    await router.replace({ name: 'my-notes' })
    return
  }
  try {
    const n = await notesApi.create('me', note)
    toast.success(t('notes.mine.saved'))
    await router.replace({ name: 'my-notes', query: { note: n.id } })
  } catch (e: any) {
    failed.value = e.message
  }
})
</script>

<template>
  <div class="mx-auto max-w-md px-4 py-16 text-center" data-testid="share">
    <template v-if="failed">
      <h1 class="title-3">{{ t('notes.mine.shareFailed') }}</h1>
      <p class="mt-1.5 text-[13px] text-text-2">{{ failed }}</p>
      <RouterLink class="btn mt-5" :to="{ name: 'my-notes' }">{{ t('notes.mine.title') }}</RouterLink>
    </template>
    <p v-else class="text-[14px] text-text-2">{{ t('notes.mine.sharing') }}</p>
  </div>
</template>

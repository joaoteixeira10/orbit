<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Brush, ExternalLink } from 'lucide-vue-next'
import { drawingsApi, previewSrc, type Drawing } from '@/components/draw/drawing'
import { guestApi } from '@/lib/shareApi'

// A whiteboard shown in a page (a link to it alone on its line): its
// picture, and a way to open it. guest: a guest link's secret.
const props = defineProps<{ ws: string; id: string; guest?: string }>()
const { t } = useI18n()
const router = useRouter()

const doc = ref<Drawing | null>(null)
const failed = ref(false)
watch(
  () => props.id,
  async (id) => {
    try {
      doc.value = props.guest ? await guestApi.embed<Drawing>(props.guest, 'board', id) : await drawingsApi.get(id)
    } catch {
      failed.value = true
    }
  },
  { immediate: true },
)
const open = () => router.push({ name: 'ws-whiteboard', params: { ws: props.ws, id: props.id } })
</script>

<template>
  <div class="board-embed" data-testid="whiteboard-embed">
    <div class="be-head">
      <Brush class="h-4 w-4 shrink-0 text-[#af52de]" />
      <span class="min-w-0 flex-1 truncate font-medium">{{ failed ? t('embed.boardNotFound') : doc?.title || t('draw.untitled') }}</span>
      <button v-if="!failed && !guest" type="button" class="btn-plain !py-0.5 text-[12.5px]" @click="open"><ExternalLink class="h-3.5 w-3.5" />{{ t('diagrams.open') }}</button>
    </div>
    <div v-if="doc" class="be-pic">
      <img v-if="doc.preview" :src="previewSrc(doc.preview)" :alt="doc.title" />
      <span v-else class="text-[13px] text-[#8e8e93]">{{ t('embed.emptyBoard') }}</span>
    </div>
  </div>
</template>

<style scoped>
.board-embed {
  margin: 12px 0;
  overflow: hidden;
  border: 0.5px solid var(--border);
  border-radius: 12px;
}
.be-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  font-size: 13px;
  background: var(--fill-2);
}
.be-pic {
  display: flex;
  min-height: 120px;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: #fff;
}
.be-pic img {
  max-width: 100%;
  max-height: 520px;
}
</style>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ExternalLink, Workflow } from 'lucide-vue-next'
import { diagramsApi, type DiagramDoc } from '@/lib/diagramsApi'
import { svgUrl } from '@/lib/drawio'

// A diagram shown in a page (a link to it alone on its line): its picture,
// and a way to open it.
const props = defineProps<{ ws: string; id: string }>()
const { t } = useI18n()
const router = useRouter()

const doc = ref<DiagramDoc | null>(null)
const failed = ref(false)
watch(
  () => props.id,
  async (id) => {
    try {
      doc.value = await diagramsApi.get(id)
    } catch {
      failed.value = true
    }
  },
  { immediate: true },
)
const open = () => router.push({ name: 'ws-diagram', params: { ws: props.ws, id: props.id } })
</script>

<template>
  <div class="diagram-embed" data-testid="diagram-embed">
    <div class="de-head">
      <Workflow class="h-4 w-4 shrink-0 text-text-3" />
      <span class="min-w-0 flex-1 truncate font-medium">{{ failed ? t('diagrams.notFound') : doc?.title || t('diagrams.untitled') }}</span>
      <button v-if="!failed" type="button" class="btn-plain !py-0.5 text-[12.5px]" @click="open"><ExternalLink class="h-3.5 w-3.5" />{{ t('diagrams.open') }}</button>
    </div>
    <button v-if="doc" type="button" class="de-pic" :aria-label="t('diagrams.open')" @click="open">
      <img v-if="doc.preview" :src="svgUrl(doc.preview)" :alt="doc.title" />
      <span v-else class="text-[13px] text-[#8e8e93]">{{ t('diagrams.noPicture') }}</span>
    </button>
  </div>
</template>

<style scoped>
.diagram-embed {
  margin: 12px 0;
  overflow: hidden;
  border: 0.5px solid var(--border);
  border-radius: 12px;
}
.de-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  font-size: 13px;
  background: var(--fill-2);
}
.de-pic {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: #fff;
}
.de-pic img {
  max-width: 100%;
  max-height: 520px;
}
</style>

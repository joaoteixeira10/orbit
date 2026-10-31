<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Brush, Upload } from 'lucide-vue-next'
import FileBrowser from '@/components/files/FileBrowser.vue'
import { drawingsApi } from '@/components/draw/drawing'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// My files: one's own drive, outside any workspace — Quick Draw sketches,
// files from the computer, and Quick Notes' attachments, in folders of
// one's own. Only its owner sees it.
const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const toast = useToastStore()
const editable = !auth.privileges.read_only

const browser = ref<InstanceType<typeof FileBrowser> | null>(null)
const rawEl = ref<HTMLInputElement | null>(null)
function uploadRaw(e: Event) {
  const list = [...((e.target as HTMLInputElement).files ?? [])]
  ;(e.target as HTMLInputElement).value = ''
  void browser.value?.addFiles(list)
}
// A new sketch, in the folder shown.
async function newSketch() {
  try {
    const d = await drawingsApi.make('me', { title: '', folder_id: browser.value?.here ?? '' })
    await router.push({ name: 'my-drawing', params: { id: d.id } })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="files-view">
    <header class="mb-6 flex flex-wrap items-end gap-x-4 gap-y-3">
      <div class="min-w-0 flex-1">
        <h1 class="large-title">{{ t('myFiles.title') }}</h1>
        <p class="subhead mt-0.5">{{ t('myFiles.sub') }}</p>
      </div>
      <template v-if="editable">
        <input ref="rawEl" type="file" class="hidden" multiple data-testid="files-add-input" @change="uploadRaw" />
        <button class="btn" :title="t('drive.addFilesHint')" data-testid="files-add" @click="rawEl?.click()"><Upload class="h-4 w-4" />{{ t('drive.addFiles') }}</button>
        <button class="btn-primary" data-testid="my-files-sketch" @click="newSketch"><Brush class="h-4 w-4" />{{ t('draw.newSketch') }}</button>
      </template>
    </header>
    <FileBrowser ref="browser" ws="me" :kinds="['board', 'file']" :editable="editable" :root-name="t('myFiles.title')">
      <template #empty>{{ t('myFiles.empty') }}</template>
    </FileBrowser>
  </div>
</template>

<style scoped>
.files-view {
  padding: 32px clamp(16px, 4vw, 48px) 96px;
}
</style>

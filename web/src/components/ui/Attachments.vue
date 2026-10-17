<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Paperclip, X, FileText, LoaderCircle } from 'lucide-vue-next'
import { downloadUpload, fileSize, uploadBlobUrl, uploadFile, uploadsApi, type Upload, type UploadTarget } from '@/lib/uploads'
import { useToastStore } from '@/stores/toast'

// The files on a task or a note: images as thumbnails, other files as rows;
// add (button, or drop files on it), open / download, remove. Given the
// files already (a note's, from its list) or loading them itself (a task's).
const props = defineProps<{ target: UploadTarget; targetId: string; editable: boolean; initial?: Upload[]; compact?: boolean }>()
const emit = defineEmits<{ change: [files: Upload[]] }>()
const { t } = useI18n()
const toast = useToastStore()

const files = ref<Upload[]>(props.initial ?? [])
const thumbs = ref<Record<string, string>>({})
const busy = ref(0)
const over = ref(false)
const input = ref<HTMLInputElement | null>(null)

function loadThumbs() {
  for (const f of files.value) {
    if (f.image && !thumbs.value[f.id]) void uploadBlobUrl(f.url).then((u) => (thumbs.value = { ...thumbs.value, [f.id]: u })).catch(() => {})
  }
}
async function load() {
  if (props.initial) {
    files.value = props.initial
  } else {
    try {
      files.value = await uploadsApi.list(props.target, props.targetId)
    } catch {
      files.value = []
    }
  }
  loadThumbs()
}
onMounted(load)
watch(() => [props.targetId, props.initial] as const, load)

async function add(list: FileList | File[] | null) {
  for (const f of [...(list ?? [])]) {
    busy.value++
    try {
      files.value = [...files.value, await uploadFile(props.target, props.targetId, f)]
      emit('change', files.value)
      loadThumbs()
    } catch (e: any) {
      toast.error(e?.message ?? String(e))
    } finally {
      busy.value--
    }
  }
}
async function remove(f: Upload) {
  try {
    await uploadsApi.remove(f.id)
    files.value = files.value.filter((x) => x.id !== f.id)
    emit('change', files.value)
  } catch (e: any) {
    toast.error(e?.message ?? String(e))
  }
}
// Images open in a new tab (from their blob: URL); other files download.
async function open(f: Upload) {
  if (f.image) {
    const url = thumbs.value[f.id] ?? (await uploadBlobUrl(f.url))
    window.open(url, '_blank', 'noopener')
  } else {
    void downloadUpload(f.url, f.name).catch((e) => toast.error(e?.message ?? String(e)))
  }
}
function onDrop(e: DragEvent) {
  over.value = false
  if (!props.editable) return
  e.preventDefault()
  void add(e.dataTransfer?.files ?? null)
}
defineExpose({ pick: () => input.value?.click() })
</script>

<template>
  <div
    class="attachments"
    :class="{ over, compact }"
    @dragover.prevent="editable && (over = true)"
    @dragleave="over = false"
    @drop="onDrop"
  >
    <div v-if="files.some((f) => f.image)" class="thumbs">
      <div v-for="f in files.filter((x) => x.image)" :key="f.id" class="thumb">
        <button type="button" class="thumb-img" :title="f.name" @click="open(f)">
          <img v-if="thumbs[f.id]" :src="thumbs[f.id]" :alt="f.name" />
        </button>
        <button v-if="editable" type="button" class="thumb-x" :title="t('files.remove')" :aria-label="t('files.remove')" @click="remove(f)"><X class="h-3 w-3" /></button>
      </div>
    </div>
    <div v-for="f in files.filter((x) => !x.image)" :key="f.id" class="file-row">
      <FileText class="h-4 w-4 shrink-0 text-text-3" />
      <button type="button" class="min-w-0 flex-1 truncate text-left text-[13.5px] hover:underline" @click="open(f)">{{ f.name }}</button>
      <span class="shrink-0 text-[12px] text-text-3">{{ fileSize(f.size) }}</span>
      <button v-if="editable" type="button" class="icon-btn h-6 w-6" :title="t('files.remove')" :aria-label="t('files.remove')" @click="remove(f)"><X class="h-3.5 w-3.5" /></button>
    </div>
    <div v-if="editable && !compact" class="add-row">
      <button type="button" class="btn-plain !px-1" @click="input?.click()">
        <LoaderCircle v-if="busy" class="h-4 w-4 animate-spin" /><Paperclip v-else class="h-4 w-4" />{{ t('files.add') }}
      </button>
      <span class="text-[12px] text-text-3">{{ t('files.drop') }}</span>
    </div>
    <input ref="input" type="file" multiple class="hidden" @change="add(($event.target as HTMLInputElement).files); ($event.target as HTMLInputElement).value = ''" />
  </div>
</template>

<style scoped>
.attachments {
  border-radius: 12px;
  transition: box-shadow var(--speed);
}
.attachments.over {
  box-shadow: 0 0 0 2px var(--accent);
}
.thumbs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 6px 2px;
}
.thumb {
  position: relative;
}
.thumb-img {
  display: block;
  width: 84px;
  height: 84px;
  border-radius: 10px;
  overflow: hidden;
  background: var(--fill-2);
  box-shadow: 0 0 0 0.5px var(--border);
}
.compact .thumb-img {
  width: 100%;
  height: auto;
  max-height: 160px;
}
.compact .thumbs {
  display: block;
  padding: 0;
}
.compact .thumb + .thumb {
  margin-top: 6px;
}
.thumb-img img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.thumb-x {
  position: absolute;
  top: -6px;
  right: -6px;
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-2);
  box-shadow: var(--shadow-sm), 0 0 0 0.5px var(--border-2);
}
.file-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
}
.add-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 4px 2px;
}
</style>

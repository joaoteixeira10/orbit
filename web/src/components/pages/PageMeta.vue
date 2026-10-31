<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Calendar, CheckSquare, Hash, Link as LinkIcon, Plus, Type, X } from 'lucide-vue-next'
import Popover from '@/components/ui/Popover.vue'
import { pagesApi } from '@/api/planner'
import type { PageField } from '@/lib/markdown'
import { useToastStore } from '@/stores/toast'

// Any page's tags and properties, under its title: tags as chips (a click
// lists the workspace's pages with it), properties as rows — a name and a
// value of its kind (text, number, date, checkbox, link). Each change saves
// at once (no version: it's not the page's text).
const props = defineProps<{ ws: string; pageId: string; tags: string[]; fields: PageField[]; editable: boolean }>()
const emit = defineEmits<{ saved: [b: { tags?: string[]; fields?: PageField[] }] }>()
const { t } = useI18n()
const router = useRouter()
const toast = useToastStore()

const tags = ref<string[]>([...props.tags])
const fields = ref<PageField[]>(props.fields.map((f) => ({ ...f })))
watch(
  () => [props.tags, props.fields] as const,
  ([tg, fs]) => {
    tags.value = [...tg]
    fields.value = fs.map((f) => ({ ...f }))
  },
)

async function save(b: { tags?: string[]; fields?: PageField[] }) {
  try {
    await pagesApi.update(props.pageId, b)
    emit('saved', b)
  } catch (e: any) {
    toast.error(e.message)
  }
}

// ---- Tags ----
const adding = ref(false)
const tagText = ref('')
const tagEl = ref<HTMLInputElement | null>(null)
const known = ref<string[]>([])
async function startTag() {
  adding.value = true
  tagText.value = ''
  await nextTick()
  tagEl.value?.focus()
  known.value = (await pagesApi.tags(props.ws).catch(() => [])).map((x) => x.tag)
}
const suggestions = computed(() => {
  const q = tagText.value.trim().replace(/^#/, '').toLowerCase()
  const have = new Set(tags.value.map((x) => x.toLowerCase()))
  return known.value.filter((x) => !have.has(x.toLowerCase()) && (!q || x.toLowerCase().includes(q))).slice(0, 6)
})
function addTag(raw = tagText.value) {
  const tag = raw.trim().replace(/^#/, '').replace(/\s+/g, ' ')
  tagText.value = ''
  if (!tag || tags.value.some((x) => x.toLowerCase() === tag.toLowerCase())) return
  tags.value = [...tags.value, tag]
  void save({ tags: tags.value })
}
function removeTag(tag: string) {
  tags.value = tags.value.filter((x) => x !== tag)
  void save({ tags: tags.value })
}
function onTagKey(e: KeyboardEvent) {
  if (e.key === 'Enter' || e.key === ',') {
    e.preventDefault()
    addTag()
  } else if (e.key === 'Escape') adding.value = false
  else if (e.key === 'Backspace' && !tagText.value && tags.value.length) removeTag(tags.value[tags.value.length - 1])
}
// Leaving the field adds what's typed; it closes a moment later, so a click
// on what's beside it (which moves as it closes) still lands.
function onTagBlur() {
  addTag()
  setTimeout(() => (adding.value = false), 150)
}
const openTag = (tag: string) => router.push({ name: 'ws-pages', params: { ws: props.ws }, query: { tag } })

// ---- Properties ----
const KINDS: { type: PageField['type']; icon: unknown }[] = [
  { type: 'text', icon: Type },
  { type: 'number', icon: Hash },
  { type: 'date', icon: Calendar },
  { type: 'checkbox', icon: CheckSquare },
  { type: 'url', icon: LinkIcon },
]
const iconOf = (type: PageField['type']) => KINDS.find((k) => k.type === type)?.icon ?? Type
const newOpen = ref(false)
const newEl = ref<HTMLElement | null>(null)
const newName = ref('')
const newType = ref<PageField['type']>('text')
const newNameEl = ref<HTMLInputElement | null>(null)
async function startField() {
  newName.value = ''
  newType.value = 'text'
  newOpen.value = true
  await nextTick()
  newNameEl.value?.focus()
}
function addField() {
  const key = newName.value.trim()
  if (!key) return
  if (fields.value.some((f) => f.key.toLowerCase() === key.toLowerCase())) {
    toast.error(t('meta.twice', { name: key }))
    return
  }
  newOpen.value = false
  fields.value = [...fields.value, { key, type: newType.value, value: newType.value === 'checkbox' ? false : null }]
  void save({ fields: fields.value })
}
function setValue(i: number, raw: string | boolean) {
  const f = fields.value[i]
  let value: PageField['value'] = raw === '' ? null : raw
  if (f.type === 'number' && typeof raw === 'string') value = raw === '' ? null : Number(raw)
  if (f.type === 'number' && typeof value === 'number' && !Number.isFinite(value)) return
  if (value === f.value) return
  fields.value = fields.value.map((x, j) => (j === i ? { ...x, value } : x))
  void save({ fields: fields.value })
}
function removeField(i: number) {
  fields.value = fields.value.filter((_, j) => j !== i)
  void save({ fields: fields.value })
}
const shown = computed(() => props.editable || tags.value.length || fields.value.length)
</script>

<template>
  <div v-if="shown" class="page-meta" data-testid="page-meta">
    <div v-if="tags.length || editable" class="tags">
      <span v-for="tag in tags" :key="tag" class="tag" data-testid="page-tag">
        <button type="button" class="tag-name" :title="t('meta.withTag', { tag })" @click="openTag(tag)">#{{ tag }}</button>
        <button v-if="editable" type="button" class="tag-x" :aria-label="t('meta.removeTag', { tag })" @click="removeTag(tag)"><X class="h-3 w-3" /></button>
      </span>
      <span v-if="adding" class="tag-input">
        <input
          ref="tagEl"
          v-model="tagText"
          maxlength="40"
          :placeholder="t('meta.tagPh')"
          :aria-label="t('meta.addTag')"
          data-testid="tag-input"
          @keydown="onTagKey"
          @blur="onTagBlur"
        />
        <span v-if="suggestions.length" class="suggest">
          <button v-for="s in suggestions" :key="s" type="button" data-testid="tag-suggestion" @mousedown.prevent="addTag(s)">#{{ s }}</button>
        </span>
      </span>
      <button v-else-if="editable" type="button" class="meta-add" data-testid="add-tag" @click="startTag"><Plus class="h-3 w-3" />{{ t('meta.addTag') }}</button>
      <button v-if="editable" ref="newEl" type="button" class="meta-add" data-testid="add-field" @click="startField"><Plus class="h-3 w-3" />{{ t('meta.addField') }}</button>
    </div>

    <div v-if="fields.length" class="fields">
      <div v-for="(f, i) in fields" :key="f.key" class="field-row" data-testid="page-field">
        <span class="field-key"><component :is="iconOf(f.type)" class="h-3.5 w-3.5 shrink-0" />{{ f.key }}</span>
        <span class="field-value">
          <input v-if="f.type === 'checkbox'" type="checkbox" :checked="!!f.value" :disabled="!editable" :aria-label="f.key" @change="setValue(i, ($event.target as HTMLInputElement).checked)" />
          <template v-else-if="editable">
            <input
              class="field-input"
              :type="f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : f.type === 'url' ? 'url' : 'text'"
              :value="f.value ?? ''"
              :placeholder="t('meta.empty')"
              :aria-label="f.key"
              data-testid="field-value"
              @change="setValue(i, ($event.target as HTMLInputElement).value)"
              @keydown.enter="($event.target as HTMLInputElement).blur()"
            />
            <a v-if="f.type === 'url' && f.value" :href="String(f.value)" target="_blank" rel="noopener noreferrer" class="field-open" :aria-label="t('meta.open')"><LinkIcon class="h-3.5 w-3.5" /></a>
          </template>
          <a v-else-if="f.type === 'url' && f.value" :href="String(f.value)" target="_blank" rel="noopener noreferrer" class="text-accent">{{ f.value }}</a>
          <span v-else class="read">{{ f.value ?? '—' }}</span>
        </span>
        <button v-if="editable" type="button" class="icon-btn h-6 w-6 shrink-0 opacity-60" :aria-label="t('meta.removeField', { name: f.key })" @click="removeField(i)"><X class="h-3.5 w-3.5" /></button>
      </div>
    </div>

    <Popover v-if="newOpen" :anchor="newEl" :width="260" placement="bottom-start" @close="newOpen = false">
      <form class="flex flex-col gap-2 p-2.5" @submit.prevent="addField">
        <input ref="newNameEl" v-model="newName" class="field !py-1.5 text-[13.5px]" maxlength="60" :placeholder="t('meta.fieldName')" :aria-label="t('meta.fieldName')" data-testid="field-name" />
        <div class="kinds" role="radiogroup" :aria-label="t('meta.kind')">
          <button v-for="k in KINDS" :key="k.type" type="button" role="radio" :aria-checked="newType === k.type" :class="{ on: newType === k.type }" :data-testid="`field-kind-${k.type}`" @click="newType = k.type">
            <component :is="k.icon" class="h-3.5 w-3.5" />{{ t(`meta.kinds.${k.type}`) }}
          </button>
        </div>
        <button type="submit" class="btn-primary btn-sm self-end" :disabled="!newName.trim()" data-testid="field-add">{{ t('meta.add') }}</button>
      </form>
    </Popover>
  </div>
</template>

<style scoped>
.page-meta {
  margin: 4px 0 18px;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.tag {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ws, var(--accent)) 12%, transparent);
  color: var(--ws, var(--accent));
  font-size: 12.5px;
  font-weight: 500;
}
.tag-name {
  padding: 2px 4px 2px 10px;
}
.tag-name:hover {
  text-decoration: underline;
}
.tag-x {
  display: grid;
  place-items: center;
  width: 20px;
  height: 20px;
  margin-right: 2px;
  border-radius: 999px;
  opacity: 0.7;
}
.tag-x:hover {
  opacity: 1;
  background: color-mix(in srgb, currentColor 14%, transparent);
}
.tag-input {
  position: relative;
}
.tag-input input {
  width: 150px;
  padding: 2px 10px;
  border-radius: 999px;
  background: var(--fill-2);
  font-size: 12.5px;
  outline: none;
}
.suggest {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 20;
  display: flex;
  min-width: 160px;
  flex-direction: column;
  padding: 4px;
  border-radius: 10px;
  background: var(--surface);
  box-shadow: var(--shadow-lg, 0 8px 24px rgba(0, 0, 0, 0.15)), 0 0 0 0.5px var(--border);
}
.suggest button {
  padding: 4px 8px;
  border-radius: 6px;
  text-align: left;
  font-size: 13px;
}
.suggest button:hover {
  background: var(--fill-2);
}
.meta-add {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 2px 8px;
  border-radius: 999px;
  color: var(--text-3);
  font-size: 12.5px;
}
.meta-add:hover {
  background: var(--fill-2);
  color: var(--text-2);
}
.fields {
  display: grid;
  grid-template-columns: minmax(110px, max-content) minmax(0, 1fr) auto;
  gap: 2px 12px;
  margin-top: 10px;
  max-width: 560px;
}
.field-row {
  display: contents;
}
.field-key {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  color: var(--text-3);
  font-size: 13px;
}
.field-value {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  font-size: 13.5px;
}
.field-input {
  width: 100%;
  min-width: 0;
  padding: 3px 6px;
  border-radius: 6px;
  background: transparent;
  outline: none;
}
.field-input:hover,
.field-input:focus {
  background: var(--fill-2);
}
.field-open {
  color: var(--text-3);
}
.read {
  overflow-wrap: anywhere;
}
.kinds {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
.kinds button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 999px;
  background: var(--fill-2);
  font-size: 12px;
  color: var(--text-2);
}
.kinds button.on {
  background: var(--accent);
  color: var(--accent-fg);
}
</style>

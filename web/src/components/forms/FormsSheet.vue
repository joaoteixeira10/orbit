<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ClipboardList, ExternalLink, Plus, Trash2 } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Toggle from '@/components/ui/Toggle.vue'
import CopyButton from '@/components/ui/CopyButton.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { shareApi, type FormDef, type FormField } from '@/lib/shareApi'
import type { Database } from '@/lib/database'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'

// A database's forms: each asks for some of its properties (and the row's
// name), with your own words and help, some required; it takes answers
// until closed; a public one has a link anyone can answer. Every answer is
// a new row.
const props = defineProps<{ open: boolean; ws: string; pageId: string; db: Database; editable: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const auth = useAuthStore()
const toast = useToastStore()

const forms = ref<FormDef[]>([])
const editing = ref<FormDef | null>(null)
watch(
  () => props.open,
  async (o) => {
    if (!o) return
    editing.value = null
    try {
      forms.value = await shareApi.forms(props.pageId)
    } catch (e: any) {
      toast.error(e.message)
    }
  },
  { immediate: true },
)

// Every question it could ask: the name, then the properties (not formulas).
const choices = computed(() => [
  { prop: 'title', name: t('db.name') },
  ...props.db.props.filter((p) => p.type !== 'formula').map((p) => ({ prop: p.id, name: p.name })),
])
const fieldOf = (prop: string) => editing.value?.fields.find((f) => f.prop === prop)
function include(prop: string, on: boolean) {
  const f = editing.value
  if (!f) return
  if (on && !fieldOf(prop)) {
    // In the database's order.
    const order = choices.value.map((c) => c.prop)
    f.fields = [...f.fields, { prop }].sort((a, b) => order.indexOf(a.prop) - order.indexOf(b.prop))
  } else if (!on) f.fields = f.fields.filter((x) => x.prop !== prop)
}
function setField(prop: string, patch: Partial<FormField>) {
  const x = fieldOf(prop)
  if (x) Object.assign(x, patch)
}

async function make() {
  try {
    const f = await shareApi.createForm(props.pageId, {})
    forms.value = [...forms.value, f]
    editing.value = JSON.parse(JSON.stringify(f))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function save(patch: Partial<FormDef> = {}) {
  const f = editing.value
  if (!f) return
  try {
    const x = await shareApi.updateForm(f.id, { title: f.title, intro: f.intro, thanks: f.thanks, fields: f.fields, open: f.open, public: f.public, ...patch })
    forms.value = forms.value.map((y) => (y.id === x.id ? x : y))
    editing.value = JSON.parse(JSON.stringify(x))
    toast.success(t('forms.saved'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
const removing = ref(false)
async function remove() {
  const f = editing.value
  removing.value = false
  if (!f) return
  try {
    await shareApi.removeForm(f.id)
    forms.value = forms.value.filter((y) => y.id !== f.id)
    editing.value = null
  } catch (e: any) {
    toast.error(e.message)
  }
}
const canPublic = computed(() => auth.features.public_forms !== false)
</script>

<template>
  <Overlay :open="open" :title="t('forms.title')" size="lg" @close="emit('close')">
    <!-- The forms -->
    <template v-if="!editing">
      <p class="mb-4 text-[13px] leading-relaxed text-text-2">{{ t('forms.lead') }}</p>
      <p v-if="!forms.length" class="mb-3 rounded-xl bg-fill-2 px-4 py-3 text-[13px] text-text-3">{{ t('forms.none') }}</p>
      <div v-for="f in forms" :key="f.id" class="form-row" data-testid="form-row">
        <ClipboardList class="h-4 w-4 shrink-0 text-text-3" />
        <button type="button" class="min-w-0 flex-1 truncate text-left font-medium" @click="editing = JSON.parse(JSON.stringify(f))">{{ f.title || t('forms.untitled') }}</button>
        <span class="shrink-0 text-[12px] text-text-3">{{ t('forms.answers', { n: f.answers }, f.answers) }}<template v-if="f.public"> · {{ t('forms.public') }}</template><template v-if="!f.open"> · {{ t('forms.closedShort') }}</template></span>
        <RouterLink :to="{ name: 'ws-form', params: { ws, id: f.id } }" class="icon-btn h-8 w-8" :title="t('forms.openIt')" :aria-label="t('forms.openIt')"><ExternalLink class="h-4 w-4" /></RouterLink>
      </div>
    </template>

    <!-- One form -->
    <div v-else class="flex flex-col gap-4" data-testid="form-editor">
      <button type="button" class="btn-plain -ml-2 self-start" @click="editing = null"><ChevronLeft class="h-4 w-4" />{{ t('forms.all') }}</button>
      <label class="block">
        <span class="label">{{ t('forms.formTitle') }}</span>
        <input v-model="editing.title" class="input" maxlength="300" :disabled="!editable" data-testid="form-edit-title" />
      </label>
      <label class="block">
        <span class="label">{{ t('forms.intro') }}</span>
        <textarea v-model="editing.intro" class="input" rows="2" maxlength="5000" :disabled="!editable" />
      </label>
      <div>
        <span class="label">{{ t('forms.questions') }}</span>
        <div class="flex flex-col divide-y divide-border rounded-xl border border-border">
          <div v-for="c in choices" :key="c.prop" class="q-row" :class="{ off: !fieldOf(c.prop) }">
            <input type="checkbox" :checked="!!fieldOf(c.prop)" :disabled="!editable" :aria-label="c.name" :data-testid="`form-include-${c.name}`" @change="include(c.prop, ($event.target as HTMLInputElement).checked)" />
            <div class="min-w-0 flex-1">
              <div class="text-[13px] font-medium">{{ c.name }}</div>
              <template v-if="fieldOf(c.prop)">
                <input class="field mt-1 !py-1 text-[12.5px]" :value="fieldOf(c.prop)!.label" :placeholder="t('forms.askAs', { name: c.name })" maxlength="200" :disabled="!editable" @input="setField(c.prop, { label: ($event.target as HTMLInputElement).value })" />
                <input class="field mt-1 !py-1 text-[12.5px]" :value="fieldOf(c.prop)!.help" :placeholder="t('forms.help')" maxlength="500" :disabled="!editable" @input="setField(c.prop, { help: ($event.target as HTMLInputElement).value })" />
              </template>
            </div>
            <label v-if="fieldOf(c.prop)" class="flex shrink-0 items-center gap-1.5 text-[12px] text-text-2">
              <Toggle :model-value="!!fieldOf(c.prop)!.required" :disabled="!editable" @update:model-value="(v: boolean) => setField(c.prop, { required: v })" />{{ t('forms.required') }}
            </label>
          </div>
        </div>
      </div>
      <label class="block">
        <span class="label">{{ t('forms.thanksLabel') }}</span>
        <input v-model="editing.thanks" class="input" maxlength="500" :placeholder="t('forms.thanks')" :disabled="!editable" />
      </label>
      <div class="flex flex-wrap items-center gap-x-6 gap-y-3 text-[13px]">
        <label class="flex items-center gap-2"><Toggle v-model="editing.open" :disabled="!editable" />{{ t('forms.taking') }}</label>
        <label class="flex items-center gap-2" :title="canPublic ? '' : t('forms.publicOff')"><Toggle v-model="editing.public" :disabled="!editable || (!canPublic && !editing.public)" data-testid="form-public" />{{ t('forms.makePublic') }}</label>
      </div>
      <div v-if="editing.url" class="flex items-center gap-2 rounded-lg bg-fill-2 px-3 py-2">
        <code class="min-w-0 flex-1 truncate font-mono text-[12px]" data-testid="form-url">{{ editing.url }}</code>
        <CopyButton :value="editing.url" />
      </div>
      <p v-else-if="editing.public" class="text-[12px] text-text-3">{{ t('forms.saveForLink') }}</p>
    </div>

    <template #footer>
      <template v-if="editing">
        <button v-if="editable" type="button" class="btn btn-ghost mr-auto text-crit-text" @click="removing = true"><Trash2 class="h-4 w-4" />{{ t('forms.delete') }}</button>
        <RouterLink :to="{ name: 'ws-form', params: { ws, id: editing.id } }" class="btn">{{ t('forms.openIt') }}</RouterLink>
        <button v-if="editable" type="button" class="btn-primary" data-testid="form-save" @click="save()">{{ t('common.save') }}</button>
      </template>
      <button v-else-if="editable" type="button" class="btn-primary" data-testid="form-new" @click="make"><Plus class="h-4 w-4" />{{ t('forms.new') }}</button>
    </template>
    <Confirm :open="removing" :title="t('forms.deleteTitle')" :message="t('forms.deleteBody')" :confirm-label="t('forms.delete')" danger @confirm="remove" @close="removing = false" />
  </Overlay>
</template>

<style scoped>
.form-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  margin-bottom: 6px;
  border-radius: 12px;
  box-shadow: 0 0 0 0.5px var(--border);
}
.q-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
}
.q-row.off {
  opacity: 0.6;
}
.q-row input[type='checkbox'] {
  margin-top: 3px;
}
</style>

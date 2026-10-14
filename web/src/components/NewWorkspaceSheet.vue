<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, Check } from 'lucide-vue-next'
import Overlay from './ui/Overlay.vue'
import Toggle from './ui/Toggle.vue'
import WsIcon from './ui/WsIcon.vue'
import IconColorPicker from './ui/IconColorPicker.vue'
import { ICONS } from '@/lib/icons'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'
import { currentLocale } from '@/i18n'
import type { Template, ModuleKey } from '@/api/planner'

// "New workspace": pick a template (School, Work, Personal, Shared, Blank),
// then make it yours — name, colour, icon and which modules it has.
const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()
const wss = useWorkspacesStore()
const toast = useToastStore()
const router = useRouter()
const { t } = useI18n()

const step = ref<1 | 2>(1)
const tpl = ref<Template | null>(null)
const name = ref('')
const icon = ref('')
const color = ref('#007AFF')
const modules = ref<ModuleKey[]>([])
const starter = ref(true)
const busy = ref(false)

watch(
  () => props.open,
  async (o) => {
    if (!o) return
    step.value = 1
    tpl.value = null
    await wss.loadCatalog().catch(() => {})
  },
)

function choose(t0: Template) {
  tpl.value = t0
  name.value = t0.kind === 'custom' ? '' : t(`templates.${t0.kind}.name`)
  icon.value = t0.icon
  color.value = t0.color
  modules.value = [...t0.modules]
  starter.value = t0.kind !== 'custom'
  step.value = 2
}

const available = computed(() => wss.modules.filter((m) => !m.soon))
const soon = computed(() => wss.modules.filter((m) => m.soon))
function toggleModule(k: ModuleKey, on: boolean) {
  if (on && !modules.value.includes(k)) {
    // Keep the catalog's order.
    const order = wss.modules.map((m) => m.key)
    modules.value = [...modules.value, k].sort((a, b) => order.indexOf(a) - order.indexOf(b))
  } else if (!on) modules.value = modules.value.filter((m) => m !== k)
}

async function create() {
  if (!tpl.value || !name.value.trim()) return
  busy.value = true
  try {
    const ws = await wss.create({
      name: name.value.trim(), icon: icon.value, color: color.value, kind: tpl.value.kind,
      modules: modules.value, starter: starter.value, locale: currentLocale(),
    })
    emit('close')
    router.push({ name: 'ws', params: { ws: ws.id } })
    if (ws.kind === 'shared') toast.info(t('ws.inviteHint'))
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Overlay :open="open" :title="step === 1 ? t('ws.new') : ''" size="lg" @close="emit('close')">
    <!-- Step 1: template -->
    <div v-if="step === 1">
      <p class="subhead -mt-1 mb-4">{{ t('ws.newHint') }}</p>
      <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
        <button
          v-for="tp in wss.templates"
          :key="tp.kind"
          class="card group flex items-start gap-3 p-4 text-left transition hover:-translate-y-0.5 hover:shadow-soft"
          :class="{ 'sm:col-span-2': tp.kind === 'custom' }"
          @click="choose(tp)"
        >
          <WsIcon :icon="tp.icon" :color="tp.color" :size="40" />
          <span class="min-w-0">
            <span class="block text-[15px] font-semibold">{{ t(`templates.${tp.kind}.name`) }}</span>
            <span class="mt-0.5 block text-[13px] leading-snug text-text-2">{{ t(`templates.${tp.kind}.desc`) }}</span>
            <span class="mt-2 flex flex-wrap gap-1">
              <span v-for="m in tp.modules.filter((x) => x !== 'home')" :key="m" class="pill !text-[11px]">{{ t(`modules.${m}`) }}</span>
            </span>
          </span>
        </button>
      </div>
    </div>

    <!-- Step 2: make it yours -->
    <div v-else-if="tpl" class="space-y-5">
      <div class="-mt-2 flex items-center">
        <button class="btn-plain -ml-2" @click="step = 1"><ChevronLeft class="h-4 w-4" />{{ t('ws.templates') }}</button>
      </div>
      <div class="flex flex-col items-center gap-3">
        <WsIcon :icon="icon" :color="color" :size="64" :name="name" />
        <input
          v-model="name"
          class="w-full rounded-xl bg-fill-2 px-4 py-2.5 text-center text-[20px] font-semibold outline-none focus:shadow-[0_0_0_3px_var(--ring)]"
          :placeholder="t('ws.namePlaceholder')"
          maxlength="120"
          autofocus
          @keydown.enter="create"
        />
      </div>
      <div class="rounded-xl bg-fill-2 p-4">
        <IconColorPicker v-model:icon="icon" v-model:color="color" />
      </div>

      <div>
        <h3 class="cap mb-2 px-1">{{ t('ws.modules') }}</h3>
        <div class="group">
          <label v-for="m in available" :key="m.key" class="group-row cursor-pointer">
            <span class="tile h-7 w-7" :style="{ '--tile': color }"><component :is="ICONS[m.icon] ?? ICONS['sparkles']" class="h-4 w-4" :stroke-width="2.2" /></span>
            <span class="min-w-0 flex-1">
              <span class="block text-[14px]">{{ t(`modules.${m.key}`) }}</span>
              <span class="block text-[12px] text-text-3">{{ t(`modules.${m.key}Desc`) }}</span>
            </span>
            <Toggle :model-value="modules.includes(m.key)" @update:model-value="(v) => toggleModule(m.key, v)" />
          </label>
        </div>
        <div v-if="soon.length" class="mt-3 flex flex-wrap items-center gap-1.5 px-1">
          <span class="text-[12px] text-text-3">{{ t('ws.comingSoon') }}</span>
          <span v-for="m in soon" :key="m.key" class="pill !text-[11px] opacity-70">{{ t(`modules.${m.key}`) }}</span>
        </div>
      </div>

      <label v-if="tpl.kind !== 'custom'" class="group-row card cursor-pointer">
        <span class="min-w-0 flex-1">
          <span class="block text-[14px]">{{ t('ws.starter') }}</span>
          <span class="block text-[12px] text-text-3">{{ t('ws.starterHint') }}</span>
        </span>
        <Toggle v-model="starter" />
      </label>
    </div>

    <template v-if="step === 2" #footer>
      <button class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
      <button class="btn-primary" :disabled="busy || !name.trim() || !modules.length" @click="create">
        <Check class="h-4 w-4" />{{ t('ws.create') }}
      </button>
    </template>
  </Overlay>
</template>

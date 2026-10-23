<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import WsIcon from '@/components/ui/WsIcon.vue'
import IconColorPicker from '@/components/ui/IconColorPicker.vue'
import Toggle from '@/components/ui/Toggle.vue'
import { useTasksStore } from '@/stores/tasks'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { emptyFilter, type TaskFilter } from '@/lib/taskFilter'
import type { SmartList } from '@/api/planner'

// Creates or edits a smart list: a saved filter with a name, colour and icon,
// your own or shared with the workspace. Its filter is set with the Filter
// button (a new one starts from the filter on now).
const props = defineProps<{ open: boolean; ws: string; id?: string; filter?: TaskFilter }>()
const emit = defineEmits<{ close: []; saved: [l: SmartList]; deleted: [] }>()
const tasks = useTasksStore()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const { t } = useI18n()

const current = computed(() => (props.id ? tasks.smartOf(props.ws).find((l) => l.id === props.id) : undefined))
const creating = computed(() => !props.id)
const editable = computed(() => !auth.privileges.read_only && (creating.value || !!current.value?.can_edit))
// Sharing takes an editor (a viewer's lists are their own).
const canShare = computed(() => editable.value && wss.byId(props.ws)?.role !== 'viewer' && (wss.byId(props.ws)?.member_count ?? 1) > 1)

const name = ref('')
const icon = ref('star')
const color = ref('#FF9500')
const shared = ref(false)
const busy = ref(false)
watch(
  () => [props.open, props.id] as const,
  ([o]) => {
    if (!o) return
    busy.value = false
    name.value = current.value?.name ?? ''
    icon.value = current.value?.icon || 'star'
    color.value = current.value?.color || '#FF9500'
    shared.value = current.value?.shared ?? false
  },
  { immediate: true },
)

async function save() {
  if (!editable.value || !name.value.trim() || busy.value) return
  busy.value = true
  try {
    const body = { name: name.value.trim(), icon: icon.value, color: color.value, shared: shared.value }
    const l = await tasks.saveSmart(props.ws, props.id ?? '', creating.value ? { ...body, filter: JSON.parse(JSON.stringify(props.filter ?? emptyFilter())) } : body)
    emit('saved', l)
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

const confirmDelete = ref(false)
async function destroy() {
  if (!props.id) return
  try {
    await tasks.removeSmart(props.ws, props.id)
    confirmDelete.value = false
    toast.success(t('smartList.deleted'))
    emit('deleted')
    emit('close')
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open && (creating || !!current)" :title="creating ? t('smartList.new') : t('smartList.edit')" size="md" @close="emit('close')">
    <div class="space-y-5" data-testid="smart-sheet">
      <div class="flex flex-col items-center gap-3 pt-1">
        <WsIcon :icon="icon" :color="color" :size="64" :name="name" />
        <input
          v-model="name"
          class="w-full rounded-xl bg-fill-2 px-4 py-2.5 text-center text-[20px] font-semibold outline-none placeholder:text-text-faint focus:shadow-[0_0_0_3px_var(--ring)]"
          :style="{ color }"
          :placeholder="t('smartList.namePlaceholder')"
          :aria-label="t('smartList.namePlaceholder')"
          :disabled="!editable"
          maxlength="100"
          autofocus
          @keydown.enter.prevent="save"
        />
      </div>
      <div v-if="editable" class="rounded-xl bg-fill-2 p-4">
        <IconColorPicker v-model:icon="icon" v-model:color="color" compact />
      </div>
      <div class="group">
        <div v-if="canShare" class="group-row">
          <span class="flex-1 text-[14px]">{{ t('smartList.shared') }}</span>
          <Toggle v-model="shared" :aria-label="t('smartList.shared')" data-testid="smart-shared" />
        </div>
        <p class="px-4 py-2.5 text-[12.5px] leading-snug text-text-3">
          {{ !editable ? t('smartList.readOnly') : shared ? t('smartList.sharedHint') : t('smartList.personalHint') }}
          {{ t('smartList.filterHint') }}
        </p>
      </div>
    </div>

    <template #footer>
      <button v-if="!creating && editable" type="button" class="btn-ghost mr-auto !px-2.5 !text-danger" @click="confirmDelete = true">
        <Trash2 class="h-4 w-4" />{{ t('smartList.delete') }}
      </button>
      <template v-if="editable">
        <button type="button" class="btn" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button type="button" class="btn-primary" :disabled="!name.trim() || busy" data-testid="smart-save" @click="save">{{ creating ? t('smartList.create') : t('common.save') }}</button>
      </template>
      <button v-else type="button" class="btn-primary" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </Overlay>

  <Confirm
    :open="confirmDelete"
    :title="t('smartList.deleteTitle', { name: current?.name ?? '' })"
    :message="t('smartList.deleteBody')"
    :confirm-label="t('smartList.delete')"
    danger
    @confirm="destroy"
    @close="confirmDelete = false"
  />
</template>

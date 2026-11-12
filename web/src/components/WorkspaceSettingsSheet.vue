<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { GripVertical, UserPlus, Archive, ArchiveRestore, Trash2, LogOut, Download } from 'lucide-vue-next'
import Overlay from './ui/Overlay.vue'
import Toggle from './ui/Toggle.vue'
import Confirm from './ui/Confirm.vue'
import WsIcon from './ui/WsIcon.vue'
import IconColorPicker from './ui/IconColorPicker.vue'
import Avatar from './ui/Avatar.vue'
import InviteLinks from './InviteLinks.vue'
import WorkspaceGroups from './WorkspaceGroups.vue'
import { ICONS } from '@/lib/icons'
import { useSortable } from '@/lib/sortable'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useAuthStore } from '@/stores/auth'
import { useToastStore } from '@/stores/toast'
import { useAssistantStore } from '@/stores/assistant'
import { workspacesApi, type ModuleKey, type WSRole } from '@/api/planner'
import { downloadAuthed } from '@/lib/api'

// A workspace's settings: what it is (name, look, description), which
// modules it has and in which order, who is in it, and archiving / deleting.
// Only owners change anything; others see it read-only and can leave.
const props = defineProps<{ open: boolean; wsId: string; initialTab?: 'general' | 'modules' | 'members' }>()
const emit = defineEmits<{ close: [] }>()
const wss = useWorkspacesStore()
const auth = useAuthStore()
const toast = useToastStore()
const assistant = useAssistantStore()
const router = useRouter()
const { t } = useI18n()

const tab = ref<'general' | 'modules' | 'members'>('general')
const ws = computed(() => wss.byId(props.wsId))
const isOwner = computed(() => ws.value?.role === 'owner')
const members = computed(() => wss.members[props.wsId] ?? [])

const name = ref('')
const icon = ref('')
const color = ref('')
const description = ref('')
const notes = ref('')
const modules = ref<ModuleKey[]>([])

watch(
  () => props.open,
  async (o) => {
    if (!o || !ws.value) return
    tab.value = props.initialTab ?? 'general'
    name.value = ws.value.name
    icon.value = ws.value.icon
    color.value = ws.value.color
    description.value = ws.value.description
    notes.value = ws.value.assistant_notes ?? ''
    modules.value = [...ws.value.modules]
    await Promise.all([wss.loadCatalog(), wss.loadMembers(props.wsId)]).catch(() => {})
  },
)

async function save(patch: Parameters<typeof wss.update>[1]) {
  try {
    await wss.update(props.wsId, patch)
  } catch (e: any) {
    toast.error(e.message)
  }
}
let timer: ReturnType<typeof setTimeout> | null = null
// General fields save as you type (like Apple's settings), debounced.
watch([name, icon, color, description], () => {
  if (!props.open || !isOwner.value || !ws.value) return
  if (name.value === ws.value.name && icon.value === ws.value.icon && color.value === ws.value.color && description.value === ws.value.description) return
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => {
    if (name.value.trim()) void save({ name: name.value.trim(), icon: icon.value, color: color.value, description: description.value })
  }, 450)
})

// What the assistant is told about it, saved as typed too.
let notesTimer: ReturnType<typeof setTimeout> | null = null
watch(notes, (v) => {
  if (!props.open || !isOwner.value || !ws.value || v === (ws.value.assistant_notes ?? '')) return
  if (notesTimer) clearTimeout(notesTimer)
  notesTimer = setTimeout(() => void save({ assistant_notes: v }), 450)
})

// Modules: on/off and their order.
const allModules = computed(() => {
  const avail = wss.modules.filter((m) => !m.soon)
  const on = modules.value.map((k) => avail.find((m) => m.key === k)).filter(Boolean) as typeof avail
  return [...on, ...avail.filter((m) => !modules.value.includes(m.key))]
})
function toggleModule(k: ModuleKey, v: boolean) {
  const next = v ? [...modules.value, k] : modules.value.filter((m) => m !== k)
  if (!next.length) return
  modules.value = next
  void save({ modules: next })
}
const modList = ref<HTMLElement | null>(null)
useSortable(
  modList,
  (m) => {
    const ids = allModules.value.map((x) => x.key).filter((k) => modules.value.includes(k))
    const from = ids.indexOf(m.id)
    if (from < 0) return
    ids.splice(from, 1)
    const at = m.beforeId ? ids.indexOf(m.beforeId) + 1 : 0
    ids.splice(at, 0, m.id)
    modules.value = ids
    void save({ modules: ids })
  },
  { handle: '.drag' },
)

// Members.
const invite = ref('')
const inviteRole = ref<WSRole>('editor')
async function addMember() {
  if (!invite.value.trim()) return
  try {
    wss.members[props.wsId] = await workspacesApi.addMember(props.wsId, invite.value.trim(), inviteRole.value)
    invite.value = ''
    toast.success(t('ws.memberAdded'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function setRole(uid: string, role: WSRole) {
  try {
    wss.members[props.wsId] = await workspacesApi.setRole(props.wsId, uid, role)
  } catch (e: any) {
    toast.error(e.message)
    await wss.loadMembers(props.wsId)
  }
}
async function removeMember(uid: string) {
  try {
    await workspacesApi.removeMember(props.wsId, uid)
    if (uid === auth.user?.id) {
      emit('close')
      await wss.load()
      router.push({ name: 'home' })
      return
    }
    await wss.loadMembers(props.wsId)
  } catch (e: any) {
    toast.error(e.message)
  }
}

const confirmDelete = ref(false)
// The whole workspace as a file, to import on another Orbit.
async function exportWs() {
  if (!ws.value) return
  const slug = ws.value.name.replace(/[^A-Za-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'workspace'
  try {
    await downloadAuthed(`/workspaces/${ws.value.id}/transfer`, `${slug}.orbit.zip`)
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function archive(on: boolean) {
  await save({ archived: on })
  toast.success(on ? t('ws.archived') : t('ws.restored'))
}
async function destroy() {
  try {
    await wss.remove(props.wsId)
    confirmDelete.value = false
    emit('close')
    router.push({ name: 'home' })
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <Overlay :open="open && !!ws" :title="t('ws.settings')" size="lg" @close="emit('close')">
    <template v-if="ws">
      <div class="mb-5 flex justify-center">
        <div class="seg">
          <button :class="{ on: tab === 'general' }" @click="tab = 'general'">{{ t('ws.general') }}</button>
          <button :class="{ on: tab === 'modules' }" @click="tab = 'modules'">{{ t('ws.modules') }}</button>
          <button :class="{ on: tab === 'members' }" @click="tab = 'members'">{{ t('ws.members') }}</button>
        </div>
      </div>

      <!-- General -->
      <div v-if="tab === 'general'" class="space-y-5">
        <div class="flex items-center gap-4">
          <WsIcon :icon="icon" :color="color" :size="56" :name="name" />
          <div class="min-w-0 flex-1 space-y-2">
            <input v-model="name" class="input text-[16px] font-semibold" :disabled="!isOwner" maxlength="120" />
            <input v-model="description" class="input" :disabled="!isOwner" :placeholder="t('ws.descPlaceholder')" maxlength="500" />
          </div>
        </div>
        <div v-if="isOwner" class="rounded-xl bg-fill-2 p-4">
          <IconColorPicker v-model:icon="icon" v-model:color="color" />
        </div>
        <div class="group">
          <div class="group-row">
            <span class="flex-1 text-[14px]">{{ t('ws.yourRole') }}</span>
            <span class="text-[14px] text-text-2">{{ t(`roles.${ws.role}`) }}</span>
          </div>
          <div class="group-row">
            <span class="flex-1 text-[14px]">{{ t('ws.template') }}</span>
            <span class="text-[14px] text-text-2">{{ t(`templates.${ws.kind}.name`) }}</span>
          </div>
        </div>

        <!-- The built-in assistant, when the instance has it. -->
        <div v-if="assistant.status?.enabled || assistant.status?.own_allowed" data-testid="ws-assistant">
          <h3 class="subhead mb-2 px-1">{{ t('ws.assistant') }}</h3>
          <div class="group">
            <div class="group-row">
              <span class="min-w-0 flex-1">
                <span class="block text-[14px]">{{ t('ws.assistantIn') }}</span>
                <span class="block text-[12px] text-text-3">{{ t('ws.assistantInHint') }}</span>
              </span>
              <Toggle :model-value="!ws.assistant_off" :disabled="!isOwner" data-testid="ws-assistant-on" @update:model-value="(v) => save({ assistant_off: !v })" />
            </div>
          </div>
          <template v-if="!ws.assistant_off && (isOwner || notes)">
            <label class="mt-3 mb-1 block px-1 text-[13px] text-text-2" for="ws-assistant-notes">{{ t('ws.assistantNotes') }}</label>
            <textarea id="ws-assistant-notes" v-model="notes" class="input min-h-[76px] resize-y" :disabled="!isOwner" :placeholder="t('ws.assistantNotesPlaceholder')" maxlength="2000" data-testid="ws-assistant-notes" />
            <p class="footnote mt-1.5 px-1">{{ t('ws.assistantNotesHint') }}</p>
          </template>
        </div>

        <div class="space-y-2">
          <template v-if="isOwner">
            <button class="btn w-full justify-start" @click="archive(!ws.archived_at)">
              <component :is="ws.archived_at ? ArchiveRestore : Archive" class="h-4 w-4" />
              {{ ws.archived_at ? t('ws.unarchive') : t('ws.archive') }}
              <span class="ml-auto text-[12px] font-normal text-text-3">{{ ws.archived_at ? '' : t('ws.archiveHint') }}</span>
            </button>
            <button class="btn w-full justify-start" data-testid="export-ws" @click="exportWs">
              <Download class="h-4 w-4" />{{ t('ws.export') }}
              <span class="ml-auto text-[12px] font-normal text-text-3">{{ t('ws.exportHint') }}</span>
            </button>
            <button class="btn w-full justify-start !text-danger" @click="confirmDelete = true"><Trash2 class="h-4 w-4" />{{ t('ws.delete') }}</button>
          </template>
          <button v-else class="btn w-full justify-start !text-danger" @click="removeMember(auth.user!.id)"><LogOut class="h-4 w-4" />{{ t('ws.leave') }}</button>
        </div>
      </div>

      <!-- Modules -->
      <div v-else-if="tab === 'modules'">
        <p class="subhead mb-3 px-1">{{ t('ws.modulesHint') }}</p>
        <div ref="modList" class="group">
          <div v-for="m in allModules" :key="m.key" :data-id="m.key" class="group-row">
            <GripVertical v-if="isOwner && modules.includes(m.key)" class="drag h-4 w-4 cursor-grab text-text-faint" />
            <span v-else class="w-4" />
            <span class="tile h-7 w-7" :style="{ '--tile': ws.color }"><component :is="ICONS[m.icon]" class="h-4 w-4" :stroke-width="2.2" /></span>
            <span class="min-w-0 flex-1">
              <span class="block text-[14px]">{{ t(`modules.${m.key}`) }}</span>
              <span class="block text-[12px] text-text-3">{{ t(`modules.${m.key}Desc`) }}</span>
            </span>
            <Toggle :model-value="modules.includes(m.key)" :disabled="!isOwner || (modules.length === 1 && modules.includes(m.key))" @update:model-value="(v) => toggleModule(m.key, v)" />
          </div>
        </div>
        <p class="footnote mt-3 px-1">{{ t('ws.modulesOffHint') }}</p>
      </div>

      <!-- Members -->
      <div v-else class="space-y-4">
        <div v-if="isOwner" class="flex gap-2">
          <input v-model="invite" class="input flex-1" :placeholder="t('ws.invitePlaceholder')" autocomplete="off" @keydown.enter="addMember" />
          <select v-model="inviteRole" class="input !w-auto">
            <option value="editor">{{ t('roles.editor') }}</option>
            <option value="viewer">{{ t('roles.viewer') }}</option>
            <option value="owner">{{ t('roles.owner') }}</option>
          </select>
          <button class="btn-primary" :disabled="!invite.trim()" @click="addMember"><UserPlus class="h-4 w-4" /><span class="hidden sm:inline">{{ t('ws.invite') }}</span></button>
        </div>
        <div class="group">
          <div v-for="m in members" :key="m.user_id" class="group-row">
            <Avatar :name="m.display_name || m.username" :src="m.avatar_url" :size="32" />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-[14px]">{{ m.display_name || m.username }}<span v-if="m.user_id === auth.user?.id" class="text-text-3"> · {{ t('ws.you') }}</span></span>
              <span class="block truncate text-[12px] text-text-3">@{{ m.username }}<template v-if="m.groups?.length"> · {{ t('groups.via', { names: m.groups.join(', ') }) }}</template></span>
            </span>
            <select v-if="isOwner" class="input !w-auto !py-1 text-[13px]" :value="m.role" @change="setRole(m.user_id, ($event.target as HTMLSelectElement).value as WSRole)">
              <option value="owner">{{ t('roles.owner') }}</option>
              <option value="editor">{{ t('roles.editor') }}</option>
              <option value="viewer">{{ t('roles.viewer') }}</option>
            </select>
            <span v-else class="text-[13px] text-text-2">{{ t(`roles.${m.role}`) }}</span>
            <button v-if="isOwner && m.user_id !== auth.user?.id && (m.direct_role || !m.groups?.length)" class="icon-btn !text-danger" :title="t('ws.remove')" @click="removeMember(m.user_id)"><Trash2 class="h-4 w-4" /></button>
          </div>
        </div>
        <p class="footnote px-1">{{ t('ws.rolesHint') }}</p>
        <WorkspaceGroups :ws-id="wsId" :is-owner="isOwner" />
        <InviteLinks v-if="isOwner && auth.features.workspace_invites !== false" :ws-id="wsId" />
      </div>
    </template>
  </Overlay>
  <Confirm
    :open="confirmDelete"
    :title="t('ws.deleteTitle', { name: ws?.name })"
    :message="t('ws.deleteBody')"
    :confirm-label="t('ws.delete')"
    danger
    :type-to-confirm="ws?.name"
    @confirm="destroy"
    @close="confirmDelete = false"
  />
</template>

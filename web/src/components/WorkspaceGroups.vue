<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2, UsersRound } from 'lucide-vue-next'
import { groupsApi, type WorkspaceGroup } from '@/api/groups'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useToastStore } from '@/stores/toast'

// A workspace's groups (Settings → Members): everyone in a group is a member
// with the group's role (or their own, if higher). Its owners add a group,
// change its role, take it out.
const props = defineProps<{ wsId: string; isOwner: boolean }>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const toast = useToastStore()
const wss = useWorkspacesStore()

const here = ref<WorkspaceGroup[]>([])
const all = ref<WorkspaceGroup[]>([])
const pick = ref('')
const role = ref('editor')
async function load() {
  try {
    here.value = await groupsApi.ofWorkspace(props.wsId)
    if (props.isOwner) all.value = await groupsApi.names()
  } catch (e: any) {
    toast.error(e.message)
  }
}
onMounted(load)
const addable = computed(() => all.value.filter((g) => !here.value.some((h) => h.group_id === g.group_id)))

async function run(f: () => Promise<WorkspaceGroup[]>) {
  try {
    here.value = await f()
    await wss.loadMembers(props.wsId)
    emit('changed')
  } catch (e: any) {
    toast.error(e.message)
    await load()
  }
}
const add = () => pick.value && run(() => groupsApi.setInWorkspace(props.wsId, pick.value, role.value)).then(() => (pick.value = ''))
</script>

<template>
  <div v-if="isOwner || here.length" data-testid="ws-groups">
    <h3 class="subhead mb-2 px-1">{{ t('groups.title') }}</h3>
    <div class="group">
      <div v-for="g in here" :key="g.group_id" class="group-row" data-testid="ws-group">
        <span class="tile h-8 w-8" style="--tile: #5856d6"><UsersRound class="h-4 w-4" /></span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[14px]">{{ g.name }}</span>
          <span class="block text-[12px] text-text-3">{{ t('groups.peopleN', { n: g.members }, g.members) }}</span>
        </span>
        <select v-if="isOwner" class="input !w-auto !py-1 text-[13px]" :value="g.role" :aria-label="t('groups.roleIn')" @change="run(() => groupsApi.setInWorkspace(wsId, g.group_id, ($event.target as HTMLSelectElement).value))">
          <option value="owner">{{ t('roles.owner') }}</option>
          <option value="editor">{{ t('roles.editor') }}</option>
          <option value="viewer">{{ t('roles.viewer') }}</option>
        </select>
        <span v-else class="text-[13px] text-text-2">{{ t(`roles.${g.role}`) }}</span>
        <button v-if="isOwner" class="icon-btn !text-danger" :title="t('groups.takeOut')" :aria-label="t('groups.takeOut')" @click="run(() => groupsApi.removeFromWorkspace(wsId, g.group_id))"><Trash2 class="h-4 w-4" /></button>
      </div>
      <div v-if="isOwner && addable.length" class="group-row">
        <select v-model="pick" class="input min-w-0 flex-1 !py-1 text-[13px]" data-testid="ws-group-pick">
          <option value="">{{ t('groups.addOne') }}</option>
          <option v-for="g in addable" :key="g.group_id" :value="g.group_id">{{ g.name }} ({{ g.members }})</option>
        </select>
        <select v-model="role" class="input !w-auto !py-1 text-[13px]" :aria-label="t('groups.roleIn')">
          <option value="editor">{{ t('roles.editor') }}</option>
          <option value="viewer">{{ t('roles.viewer') }}</option>
          <option value="owner">{{ t('roles.owner') }}</option>
        </select>
        <button class="btn btn-sm" :disabled="!pick" data-testid="ws-group-add" @click="add">{{ t('admin.add') }}</button>
      </div>
      <p v-if="isOwner && !here.length && !addable.length" class="group-row text-[13px] text-text-3">{{ t('groups.noneYet') }}</p>
    </div>
  </div>
</template>

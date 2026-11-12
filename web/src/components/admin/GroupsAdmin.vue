<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Pencil, Trash2, UsersRound } from 'lucide-vue-next'
import Overlay from '@/components/ui/Overlay.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { groupsApi, type Group, type GroupDetail } from '@/api/groups'
import type { Role } from '@/api/types'
import { useToastStore } from '@/stores/toast'
import { useAuthStore } from '@/stores/auth'

// Admin → Groups: sets of people that carry instance roles and that
// workspaces' owners add with a role. Joining or leaving a group gives or
// takes all of it. A group that makes admins (a superuser role, or an
// owner-only privilege) is the owner's to change.
const props = defineProps<{ roles: Role[]; users: { id: string; username: string }[] }>()
const { t } = useI18n()
const toast = useToastStore()
const auth = useAuthStore()

const groups = ref<GroupDetail[]>([])
async function load() {
  try {
    const list = await groupsApi.list()
    groups.value = await Promise.all(list.map((g) => groupsApi.get(g.id)))
  } catch (e: any) {
    toast.error(e.message)
  }
}
onMounted(load)

const roleName = (id: string) => props.roles.find((r) => r.id === id)?.name ?? id
const makesAdmins = (g: Group) => g.role_ids.some((id) => props.roles.find((r) => r.id === id)?.superuser)
const canChange = (g: Group) => auth.isOwner || !makesAdmins(g)
const outsiders = (g: GroupDetail) => props.users.filter((u) => !g.people.some((p) => p.user_id === u.id))

// New / edit.
const form = reactive({ open: false, id: '', name: '', description: '', role_ids: [] as string[] })
function openForm(g?: GroupDetail) {
  Object.assign(form, { open: true, id: g?.id ?? '', name: g?.name ?? '', description: g?.description ?? '', role_ids: [...(g?.role_ids ?? [])] })
}
const busy = ref(false)
async function save() {
  busy.value = true
  try {
    const b = { name: form.name.trim(), description: form.description.trim(), role_ids: form.role_ids }
    if (form.id) await groupsApi.update(form.id, b)
    else await groupsApi.create(b)
    form.open = false
    toast.success(t('groups.saved'))
    await load()
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
const removing = ref<GroupDetail | null>(null)
async function remove() {
  const g = removing.value
  removing.value = null
  if (!g) return
  try {
    await groupsApi.remove(g.id)
    toast.info(t('groups.deleted'))
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}

const adding = reactive<Record<string, string>>({})
async function addPerson(g: GroupDetail) {
  const uid = adding[g.id]
  const u = props.users.find((x) => x.id === uid)
  if (!u) return
  try {
    replace(await groupsApi.addMember(g.id, u.username))
    adding[g.id] = ''
  } catch (e: any) {
    toast.error(e.message)
  }
}
async function removePerson(g: GroupDetail, uid: string) {
  try {
    replace(await groupsApi.removeMember(g.id, uid))
  } catch (e: any) {
    toast.error(e.message)
  }
}
function replace(d: GroupDetail) {
  const i = groups.value.findIndex((g) => g.id === d.id)
  if (i >= 0) groups.value[i] = d
}
const roleChoices = computed(() => props.roles.filter((r) => auth.isOwner || !r.superuser))
</script>

<template>
  <div data-testid="groups-admin">
    <div class="mb-3 flex items-center justify-between gap-3">
      <p class="text-[13px] text-text-2">{{ t('groups.hint') }}</p>
      <button class="btn-primary shrink-0" data-testid="group-new" @click="openForm()"><Plus class="h-4 w-4" /> {{ t('groups.new') }}</button>
    </div>
    <p v-if="!groups.length" class="card p-6 text-center text-[13px] text-text-3">{{ t('groups.none') }}</p>
    <div class="flex flex-col gap-4">
      <div v-for="g in groups" :key="g.id" class="card overflow-hidden" data-testid="group-card">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
          <div class="min-w-0">
            <h3 class="flex items-center gap-1.5 text-[14px] font-semibold">
              <UsersRound class="h-4 w-4 text-text-2" />{{ g.name }}
              <span v-if="g.source" class="rounded bg-surface-3 px-1.5 py-0.5 text-[10px] font-medium uppercase text-text-3">{{ t('groups.fromIdp') }}</span>
            </h3>
            <div v-if="g.description" class="text-[12px] text-text-3">{{ g.description }}</div>
          </div>
          <div v-if="canChange(g)" class="flex gap-1">
            <button class="btn-ghost px-2 py-1" :title="t('admin.edit')" :aria-label="t('admin.edit')" @click="openForm(g)"><Pencil class="h-4 w-4" /></button>
            <button class="btn-ghost px-2 py-1 text-crit-text" :title="t('common.delete')" :aria-label="t('common.delete')" @click="removing = g"><Trash2 class="h-4 w-4" /></button>
          </div>
        </div>
        <div class="grid md:grid-cols-3">
          <div class="min-w-0 border-border p-4 md:border-r">
            <span class="cap mb-2 block">{{ t('groups.people') }} · {{ g.people.length }}</span>
            <div class="mb-2.5 flex flex-wrap gap-1.5">
              <span v-for="p in g.people" :key="p.user_id" class="inline-flex items-center gap-0.5 rounded-full border border-border bg-surface-2 py-0.5 pl-2.5 pr-1 font-mono text-[12px]" data-testid="group-person">
                {{ p.username }}
                <button v-if="canChange(g) && !g.source" class="px-1 text-text-3 hover:text-crit-text" :title="t('admin.removeMember')" @click="removePerson(g, p.user_id)">×</button>
              </span>
              <span v-if="!g.people.length" class="text-[12px] text-text-3">{{ t('admin.noMembers') }}</span>
            </div>
            <div v-if="canChange(g) && !g.source && outsiders(g).length" class="flex gap-1.5">
              <select v-model="adding[g.id]" class="input min-w-0 flex-1 py-1" data-testid="group-add-person">
                <option value="">{{ t('admin.addMember') }}</option>
                <option v-for="u in outsiders(g)" :key="u.id" :value="u.id">{{ u.username }}</option>
              </select>
              <button class="btn-ghost" :disabled="!adding[g.id]" data-testid="group-add" @click="addPerson(g)">{{ t('admin.add') }}</button>
            </div>
          </div>
          <div class="min-w-0 border-border p-4 md:border-r">
            <span class="cap mb-2 block">{{ t('groups.roles') }}</span>
            <div class="flex flex-wrap gap-1">
              <span v-for="id in g.role_ids" :key="id" class="rounded bg-surface-3 px-1.5 py-0.5 text-[11.5px] text-text-2">{{ roleName(id) }}</span>
              <span v-if="!g.role_ids.length" class="text-[12px] text-text-3">{{ t('groups.noRoles') }}</span>
            </div>
          </div>
          <div class="min-w-0 p-4">
            <span class="cap mb-2 block">{{ t('groups.workspaces') }}</span>
            <div v-for="w in g.in" :key="w.id" class="text-[12.5px]">{{ w.name }} <span class="text-text-3">· {{ t(`roles.${w.role}`) }}</span></div>
            <p v-if="!g.in.length" class="text-[12px] text-text-3">{{ t('groups.noWorkspaces') }}</p>
          </div>
        </div>
      </div>
    </div>

    <Overlay :open="form.open" :title="form.id ? t('groups.edit') : t('groups.new')" size="sm" @close="form.open = false">
      <form class="space-y-4" @submit.prevent="save">
        <label class="block">
          <span class="mb-1 block text-[12px] font-medium text-text-2">{{ t('groups.name') }}</span>
          <input v-model="form.name" class="input w-full" maxlength="120" required data-testid="group-name" />
        </label>
        <label class="block">
          <span class="mb-1 block text-[12px] font-medium text-text-2">{{ t('groups.description') }}</span>
          <input v-model="form.description" class="input w-full" maxlength="500" />
        </label>
        <fieldset>
          <legend class="mb-1 text-[12px] font-medium text-text-2">{{ t('groups.roles') }}</legend>
          <label v-for="r in roleChoices" :key="r.id" class="flex items-center gap-2 py-1 text-[13px]">
            <input v-model="form.role_ids" type="checkbox" :value="r.id" />{{ r.name }}
          </label>
          <p class="mt-1 text-[12px] text-text-3">{{ t('groups.rolesHint') }}</p>
        </fieldset>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn" @click="form.open = false">{{ t('common.cancel') }}</button>
          <button type="submit" class="btn-primary" :disabled="busy || !form.name.trim()" data-testid="group-save">{{ t('common.save') }}</button>
        </div>
      </form>
    </Overlay>
    <Confirm :open="!!removing" :title="t('groups.deleteTitle')" :message="t('groups.deleteAsk', { name: removing?.name ?? '' })" :confirm-label="t('common.delete')" danger @confirm="remove" @close="removing = null" />
  </div>
</template>

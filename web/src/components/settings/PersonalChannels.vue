<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Trash2, Send, TriangleAlert } from 'lucide-vue-next'
import { myChannelsApi, type PersonalChannel, type PersonalChannelInput } from '@/api'
import { useToastStore } from '@/stores/toast'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'
import { NativeSelect } from '@/components/shadcn/native-select'

// A person's own notification channels: theirs alone (not the owner's
// shared ones), so they get the full text. Adding one sends it a test first;
// afterwards only a hint of it is shown (its secrets stay on the server).
const props = defineProps<{ allowed: string[] }>() // the kinds the owner allows
const emit = defineEmits<{ count: [n: number]; added: [n: number] }>()
const { t } = useI18n()
const toast = useToastStore()

const list = ref<PersonalChannel[]>([])
const adding = ref(false)
const busy = ref(false)
const form = ref<PersonalChannelInput>({ kind: 'ntfy' })
const ORDER: PersonalChannel['kind'][] = ['ntfy', 'telegram', 'discord', 'slack', 'teams', 'gotify']
const KINDS = computed(() => ORDER.filter((k) => props.allowed.includes(k)))
function startAdd() {
  if (!KINDS.value.includes(form.value.kind)) form.value = { kind: KINDS.value[0] }
  adding.value = true
}
const label = (k: string) => ({ ntfy: 'ntfy', telegram: 'Telegram', discord: 'Discord', slack: 'Slack', teams: 'Microsoft Teams', gotify: 'Gotify' })[k] ?? k

// The fields each kind needs.
const fields = computed<{ key: keyof PersonalChannelInput; label: string; placeholder: string; secret?: boolean; optional?: boolean }[]>(() => {
  switch (form.value.kind) {
    case 'ntfy':
      return [
        { key: 'server_url', label: t('notif.own.server'), placeholder: 'https://ntfy.sh' },
        { key: 'topic', label: t('notif.own.topic'), placeholder: 'my-orbit-1f3a' },
        { key: 'token', label: t('notif.own.token'), placeholder: 'tk_…', secret: true, optional: true },
      ]
    case 'gotify':
      return [
        { key: 'server_url', label: t('notif.own.server'), placeholder: 'https://push.example.org' },
        { key: 'token', label: t('notif.own.appToken'), placeholder: 'A…', secret: true },
      ]
    case 'telegram':
      return [
        { key: 'bot_token', label: t('notif.own.botToken'), placeholder: '123456:ABC…', secret: true },
        { key: 'chat_id', label: t('notif.own.chatId'), placeholder: '123456789' },
      ]
    default:
      return [{ key: 'webhook_url', label: t('notif.own.webhook'), placeholder: 'https://…', secret: true }]
  }
})
const ready = computed(() => fields.value.every((f) => f.optional || String(form.value[f.key] ?? '').trim()))

async function load() {
  try {
    list.value = await myChannelsApi.list()
  } catch {
    list.value = []
  }
  emit('count', list.value.length)
}
onMounted(load)

async function add() {
  if (!ready.value || busy.value) return
  busy.value = true
  try {
    list.value.push(await myChannelsApi.add(form.value))
    emit('count', list.value.length)
    emit('added', list.value.length)
    toast.success(t('notif.own.added'))
    adding.value = false
    form.value = { kind: form.value.kind }
  } catch (e: any) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
async function test(c: PersonalChannel) {
  try {
    await myChannelsApi.test(c.id)
    c.last_error = ''
    toast.success(t('notif.own.sent'))
  } catch (e: any) {
    c.last_error = e.message
    toast.error(e.message)
  }
}
async function remove(c: PersonalChannel) {
  try {
    await myChannelsApi.remove(c.id)
    list.value = list.value.filter((x) => x.id !== c.id)
    emit('count', list.value.length)
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div data-testid="own-channels">
    <div class="text-[13px] font-semibold">{{ t('notif.own.title') }}</div>
    <div class="text-[12px] text-text-3">{{ t('notif.own.hint') }}</div>
    <div v-if="list.length" class="group mt-3">
      <div v-for="c in list" :key="c.id" class="group-row">
        <div class="min-w-0 flex-1">
          <div class="truncate text-[13.5px]">{{ c.name }} <span class="text-text-3">· {{ label(c.kind) }}</span></div>
          <div class="truncate text-[12px] text-text-3">{{ c.hint }}</div>
          <div v-if="!c.allowed" class="mt-0.5 text-[12px] text-warn-text">{{ t('notif.own.notAllowed') }}</div>
          <div v-else-if="c.last_error" class="mt-0.5 flex items-center gap-1 text-[12px] text-crit-text"><TriangleAlert class="h-3 w-3 shrink-0" /><span class="truncate">{{ c.last_error }}</span></div>
        </div>
        <Button variant="ghost" size="icon" v-if="c.allowed" class="text-text-2" :title="t('notif.own.test')" :aria-label="t('notif.own.test')" @click="test(c)"><Send class="h-4 w-4" /></Button>
        <Button variant="ghost" size="icon" class="text-text-2 text-danger!" :title="t('notif.own.remove')" :aria-label="t('notif.own.remove')" @click="remove(c)"><Trash2 class="h-4 w-4" /></Button>
      </div>
    </div>

    <form v-if="adding" class="mt-3 space-y-2 rounded-lg border border-border p-3" @submit.prevent="add">
      <div class="flex flex-wrap gap-2">
        <NativeSelect size="sm" v-model="form.kind" class="w-auto" :aria-label="t('notif.own.kind')">
          <option v-for="k in KINDS" :key="k" :value="k">{{ label(k) }}</option>
        </NativeSelect>
        <Input v-model="form.name" class="min-w-0 flex-1 py-1! text-[13.5px]" maxlength="60" :placeholder="t('notif.own.namePh')" :aria-label="t('notif.own.name')" />
      </div>
      <label v-for="f in fields" :key="f.key" class="block">
        <span class="text-[12px] text-text-2">{{ f.label }}<template v-if="f.optional"> ({{ t('notif.own.optional') }})</template></span>
        <Input v-model="form[f.key]" class="mt-0.5 py-1! text-[13.5px]" :type="f.secret ? 'password' : 'text'" :placeholder="f.placeholder" autocomplete="off" spellcheck="false" />
      </label>
      <p class="text-[12px] text-text-3">{{ t('notif.own.formHint') }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="secondary" size="sm" type="button" @click="adding = false">{{ t('common.cancel') }}</Button>
        <Button size="sm" :disabled="!ready || busy">{{ busy ? t('notif.own.testing') : t('notif.own.add') }}</Button>
      </div>
    </form>
    <Button variant="secondary" size="sm" v-else-if="KINDS.length" class="mt-3" @click="startAdd"><Plus class="h-4 w-4" />{{ t('notif.own.new') }}</Button>
    <p v-else class="mt-2 text-[12px] text-text-3">{{ t('notif.own.noneAllowed') }}</p>
  </div>
</template>

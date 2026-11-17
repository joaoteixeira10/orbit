<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Archive, Download, Play } from 'lucide-vue-next'
import { api, downloadAuthedPost } from '@/lib/api'
import { useToastStore } from '@/stores/toast'
import Toggle from '@/components/ui/Toggle.vue'
import { Button } from '@/components/shadcn/button'
import { Input } from '@/components/shadcn/input'
import { NativeSelect } from '@/components/shadcn/native-select'

// Owner → Owner: encrypted backups — download one now (sealed with the master
// key, or a passphrase to keep it elsewhere), and the automatic one (to a
// directory or S3). Restoring is a command, with Orbit stopped.
const { t } = useI18n()
const toast = useToastStore()

interface Schedule {
  enabled: boolean
  interval_hours: number
  dest: 'dir' | 's3'
  dir: string
  keep: number
  s3_bucket: string
  s3_prefix: string
  s3_endpoint: string
  s3_region: string
  last?: { at: string; ok: boolean; where?: string; files?: number; error?: string }
  supported?: boolean // false on PostgreSQL (backed up with its own tools)
  files_store?: 'db' | 'dir' | 's3' // where files' bytes are kept
}
const sched = reactive<Schedule>({ enabled: false, interval_hours: 24, dest: 'dir', dir: '/backups', keep: 14, s3_bucket: '', s3_prefix: 'orbit', s3_endpoint: '', s3_region: '' })
const last = ref<Schedule['last'] | null>(null)
const supported = ref(true)
const filesStore = ref<'db' | 'dir' | 's3'>('db')
async function load() {
  try {
    const s = await api<Schedule>('/backup/schedule')
    Object.assign(sched, { ...s, dest: s.dest || 'dir', interval_hours: s.interval_hours || 24, dir: s.dir || '/backups' })
    last.value = s.last && s.last.at && !s.last.at.startsWith('0001') ? s.last : null
    supported.value = s.supported !== false
    filesStore.value = s.files_store ?? 'db'
  } catch (e: any) {
    toast.error(e.message)
  }
}
onMounted(load)

async function save() {
  try {
    const { last: _l, supported: _s, files_store: _f, ...body } = sched
    await api('/backup/schedule', { method: 'PUT', body: JSON.stringify(body) })
    toast.success(t('bk.saved'))
    await load()
  } catch (e: any) {
    toast.error(e.message)
  }
}
const running = ref(false)
async function runNow() {
  running.value = true
  try {
    await save()
    const r = await api<{ where: string }>('/backup/run', { method: 'POST' })
    toast.success(t('bk.written', { where: r.where }))
  } catch (e: any) {
    toast.error(t('bk.runFailed'), e.message)
  } finally {
    running.value = false
    await load()
  }
}

// Download now.
const pass = ref('')
const pass2 = ref('')
const usePass = ref(false)
const passOk = computed(() => !usePass.value || (pass.value.length >= 12 && pass.value === pass2.value))
const downloading = ref(false)
async function download() {
  downloading.value = true
  try {
    const mode = usePass.value ? 'passphrase' : 'masterkey'
    await downloadAuthedPost('/backup/export', usePass.value ? { passphrase: pass.value } : {}, `orbit-backup-${mode}-${new Date().toISOString().slice(0, 19).replace(/\D/g, '')}.json`)
    toast.success(t('bk.downloaded'))
    pass.value = pass2.value = ''
  } catch (e: any) {
    toast.error(t('bk.downloadFailed'), e.message)
  } finally {
    downloading.value = false
  }
}
const fmt = (iso: string) => new Date(iso).toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })
</script>

<template>
  <div class="card overflow-hidden">
    <div class="flex items-center gap-2 border-b border-border px-5 py-3"><Archive class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('bk.title') }}</h3></div>
    <div class="flex flex-col gap-5 p-5">
      <p class="text-[12px] text-text-3">{{ t('bk.intro') }}</p>
      <p v-if="!supported" class="rounded-lg border border-border px-3 py-2.5 text-[12.5px] leading-relaxed text-text-2" data-testid="backup-postgres">{{ t('bk.postgres') }}</p>
      <p v-if="filesStore !== 'db'" class="rounded-lg border border-border px-3 py-2.5 text-[12.5px] leading-relaxed text-text-2" data-testid="backup-files">{{ t(`bk.files.${filesStore}`) }}</p>

      <!-- Download now -->
      <div class="flex flex-col gap-3">
        <div class="text-[13px] font-semibold">{{ t('bk.now') }}</div>
        <div class="flex items-center justify-between gap-4">
          <div class="text-[12px] text-text-3">{{ usePass ? t('bk.passHint') : t('bk.masterHint') }}</div>
          <label class="flex shrink-0 items-center gap-2 text-[12.5px]"><Toggle v-model="usePass" :aria-label="t('bk.usePass')" /> {{ t('bk.usePass') }}</label>
        </div>
        <div v-if="usePass" class="grid gap-3 sm:grid-cols-2">
          <Input v-model="pass" type="password" autocomplete="new-password" :placeholder="t('bk.pass')" :aria-label="t('bk.pass')" />
          <Input v-model="pass2" type="password" autocomplete="new-password" :placeholder="t('bk.pass2')" :aria-label="t('bk.pass2')" />
        </div>
        <div class="flex justify-end">
          <Button :disabled="downloading || !passOk" @click="download"><Download class="h-4 w-4" /> {{ t('bk.download') }}</Button>
        </div>
      </div>

      <!-- Automatic -->
      <div class="flex flex-col gap-3 border-t border-border pt-5">
        <div class="flex items-center justify-between gap-4">
          <div><div class="text-[13px] font-semibold">{{ t('bk.auto') }}</div><div class="text-[12px] text-text-3">{{ t('bk.autoHint') }}</div></div>
          <Toggle v-model="sched.enabled" :aria-label="t('bk.auto')" />
        </div>
        <div class="grid gap-3 sm:grid-cols-3">
          <div><label class="label">{{ t('bk.every') }}</label><Input v-model.number="sched.interval_hours" type="number" min="1" max="744" /></div>
          <div><label class="label">{{ t('bk.keep') }}</label><Input v-model.number="sched.keep" type="number" min="0" max="1000" /></div>
          <div>
            <label class="label">{{ t('bk.dest') }}</label>
            <NativeSelect v-model="sched.dest" class="w-full"><option value="dir">{{ t('bk.dir') }}</option><option value="s3">S3</option></NativeSelect>
          </div>
        </div>
        <div v-if="sched.dest === 'dir'">
          <label class="label">{{ t('bk.dirPath') }}</label>
          <Input v-model="sched.dir" class="font-mono" placeholder="/backups" />
          <p class="mt-1 text-[11.5px] text-text-3">{{ t('bk.dirHint') }}</p>
        </div>
        <div v-else class="grid gap-3 sm:grid-cols-2">
          <div><label class="label">{{ t('bk.bucket') }}</label><Input v-model="sched.s3_bucket" class="font-mono" /></div>
          <div><label class="label">{{ t('bk.prefix') }}</label><Input v-model="sched.s3_prefix" class="font-mono" placeholder="orbit" /></div>
          <div><label class="label">{{ t('bk.endpoint') }}</label><Input v-model="sched.s3_endpoint" class="font-mono" placeholder="minio.example.org:9000" /></div>
          <div><label class="label">{{ t('bk.region') }}</label><Input v-model="sched.s3_region" class="font-mono" placeholder="eu-west-1" /></div>
          <p class="text-[11.5px] text-text-3 sm:col-span-2">{{ t('bk.s3Hint') }}</p>
        </div>
        <div v-if="last" class="rounded-xl border px-3 py-2 text-[12px]" :class="last.ok ? 'border-border bg-surface-2 text-text-2' : 'border-crit-border bg-crit-bg text-crit-text'">
          {{ last.ok ? t('bk.lastOk', { at: fmt(last.at), where: last.where }) + (last.files ? ' ' + t('bk.filesCopied', { n: last.files }, last.files) : '') : t('bk.lastFailed', { at: fmt(last.at), error: last.error }) }}
        </div>
        <div class="flex justify-end gap-2">
          <Button variant="ghost" :disabled="running" @click="runNow"><Play class="h-4 w-4" /> {{ t('bk.runNow') }}</Button>
          <Button @click="save">{{ t('common.save') }}</Button>
        </div>
      </div>

      <div class="border-t border-border pt-4 text-[12px] text-text-3">
        {{ t('bk.restoreHint') }}
        <pre class="mt-1.5 overflow-x-auto whitespace-pre rounded-lg bg-fill-2 px-3 py-2 font-mono text-[11.5px] leading-relaxed text-text">docker compose stop api
docker compose run --rm -v ./orbit-backup.json:/restore.json:ro api restore-backup /restore.json --force
docker compose start api</pre>
      </div>
    </div>
  </div>
</template>

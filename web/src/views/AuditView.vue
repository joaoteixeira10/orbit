<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search } from 'lucide-vue-next'
import { auditApi } from '@/api'
import type { AuditEntry } from '@/api/types'
import { useToastStore } from '@/stores/toast'
import { fmtDate } from '@/lib/format'
import { downloadAuthed } from '@/lib/api'
import { onLiveEvent } from '@/lib/live'

// The audit log: who did what to which account, role, workspace or setting,
// from where, and whether it worked. The first page refreshes when users or
// roles change.
const { t } = useI18n()

// changeDetail reads the JSON detail of an entry (fields vary by action).
function changeDetail(detail: string): { targets?: string[]; ok?: boolean; error?: string; reason?: string; actor?: string; via?: string; scope?: string } {
  try {
    const d = JSON.parse(detail || '{}')
    return d && typeof d === 'object' ? d : {}
  } catch {
    return {}
  }
}
const toast = useToastStore()
const PER_PAGE = 100

const q = ref('')
const entries = ref<AuditEntry[]>([])
const page = ref(1)
const more = ref(false)
const loaded = ref(false)
let seq = 0

async function load(append = false) {
  const my = ++seq
  const p = append ? page.value + 1 : 1
  try {
    const res = await auditApi.list(p, PER_PAGE, q.value.trim())
    if (my !== seq) return
    entries.value = append ? entries.value.concat(res.items) : res.items
    page.value = p
    more.value = p * PER_PAGE < res.total
  } catch (e: any) {
    toast.error(t('audit.cantLoad'), e?.message)
  } finally {
    loaded.value = true
  }
}

let timer: ReturnType<typeof setTimeout> | null = null
watch(q, () => {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => load(), 250)
})
// Accounts or roles changed: refresh the first page (unless searching or paging back).
const offs = ['users', 'roles', 'workspaces'].map((ev) => onLiveEvent(ev, () => !q.value && page.value === 1 && load()))
onMounted(() => load())
onUnmounted(() => offs.forEach((off) => off()))

// Verify the hash chain: every entry links to the one before, unchanged.
const verify = ref<{ ok: boolean; checked: number; broken_at?: string } | null>(null)
const verifying = ref(false)
async function verifyChain() {
  verifying.value = true
  try {
    verify.value = await auditApi.verify()
  } catch (e: any) {
    toast.error(t('audit.verifyFailed'), e?.message)
  } finally {
    verifying.value = false
  }
}
// The whole log (or what the search matches) as CSV.
const downloading = ref(false)
async function downloadCsv() {
  downloading.value = true
  try {
    const qs = q.value.trim() ? `?q=${encodeURIComponent(q.value.trim())}` : ''
    await downloadAuthed(`/audit/export${qs}`, `orbit-audit-${new Date().toISOString().slice(0, 10)}.csv`)
  } catch (e: any) {
    toast.error(t('audit.csvFailed'), e?.message)
  } finally {
    downloading.value = false
  }
}
onUnmounted(() => timer && clearTimeout(timer))

const kind = (a: AuditEntry) => a.action
function targets(a: AuditEntry): string[] {
  const d = changeDetail(a.detail)
  if (d.targets?.length) return d.targets
  if (a.target_name) return [a.target_name]
  if (!a.target_type) return []
  // UUIDs are shortened; other ids (a setting's key) are shown whole.
  const id = /^[0-9a-f-]{36}$/.test(a.target_id) ? a.target_id.slice(0, 8) : a.target_id
  return [`${a.target_type}${id ? ' ' + id : ''}`]
}
function result(a: AuditEntry) {
  const d = changeDetail(a.detail)
  return { ok: d.ok !== false, error: d.error || '', reason: d.reason || '' }
}
// The raw detail (e.g. what a user update touched).
function otherDetail(a: AuditEntry) {
  if (!a.detail) return ''
  return a.detail.length > 160 ? a.detail.slice(0, 160) + '…' : a.detail
}
</script>

<template>
  <div class="audit-page">
  <h1 class="large-title mb-6">{{ t('audit.title') }}</h1>
  <section class="panel">
    <div class="panel-head flex-wrap">
      <span class="spacer" />
      <span v-if="verify" class="verify" :class="verify.ok ? 'ok' : 'bad'">
        {{ verify.ok ? t('audit.intact', { n: verify.checked }) : t('audit.broken', { id: verify.broken_at }) }}
      </span>
      <button type="button" class="btn btn-sm" :disabled="verifying" @click="verifyChain">{{ t('audit.verify') }}</button>
      <button type="button" class="btn btn-sm" :disabled="downloading" :title="t('audit.csvHint')" @click="downloadCsv">{{ t('audit.csv') }}</button>
      <label class="search">
        <Search class="h-4 w-4 shrink-0" aria-hidden="true" />
        <input v-model="q" type="search" :placeholder="t('audit.search')" :aria-label="t('audit.searchLabel')" autocomplete="off" />
      </label>
    </div>
    <div class="table-wrap">
      <table v-if="entries.length" class="list">
        <thead>
          <tr><th>{{ t('audit.when') }}</th><th>{{ t('audit.what') }}</th><th>{{ t('audit.who') }}</th><th>{{ t('audit.targets') }}</th><th>{{ t('audit.result') }}</th></tr>
        </thead>
        <tbody>
          <tr v-for="a in entries" :key="a.id" :class="{ failed: !result(a).ok }">
            <td class="nowrap small">{{ fmtDate(a.created_at) }}</td>
            <td><span class="act-kind" :class="[kind(a), { failed: !result(a).ok }]">{{ kind(a) }}</span></td>
            <td>
              <b>{{ (!a.user_id && changeDetail(a.detail).actor) || a.user_name || '—' }}</b>
              <div class="faint small">{{ [a.impersonator_name ? t('audit.via', { admin: a.impersonator_name }) : '', changeDetail(a.detail).via, a.ip].filter(Boolean).join(' · ') }}</div>
            </td>
            <td class="mono small">
              <div v-if="changeDetail(a.detail).scope" class="scope">{{ changeDetail(a.detail).scope }}</div>
              <template v-if="targets(a).length <= 3">
                <div v-for="x in targets(a)" :key="x">{{ x }}</div>
              </template>
              <template v-else>
                <div v-for="x in targets(a).slice(0, 2)" :key="x">{{ x }}</div>
                <details>
                  <summary>{{ t('audit.nMore', { n: targets(a).length - 2 }) }}</summary>
                  <div v-for="x in targets(a).slice(2)" :key="x">{{ x }}</div>
                </details>
              </template>
            </td>
            <td>
              <span v-if="result(a).ok" class="ok-mark">{{ t('audit.ok') }}</span>
              <span v-else class="err">{{ t('audit.failed') }}</span>
              <div v-if="result(a).error" class="err small">{{ result(a).error }}</div>
              <div v-if="result(a).reason" class="muted small">{{ t('audit.reason') }}: {{ result(a).reason }}</div>
              <div v-if="otherDetail(a)" class="muted small mono">{{ otherDetail(a) }}</div>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else class="empty-state">{{ !loaded ? t('audit.loading') : q ? t('audit.noMatch') : t('audit.none') }}</div>
    </div>
    <div v-if="more" class="panel-body"><button type="button" class="btn btn-sm" @click="load(true)">{{ t('audit.older') }}</button></div>
  </section>
  </div>
</template>

<style scoped>
.audit-page { padding: 32px clamp(16px, 4vw, 48px) 64px; }
.panel-head { justify-content: flex-start; }
.panel-head .search { height: 32px; min-width: 260px; }
.verify { font-size: 12px; font-weight: 550; }
.verify.ok { color: var(--ok); }
.verify.bad { color: var(--crit); }
.scope { font-family: var(--font-sans); font-weight: 600; color: var(--text); margin-bottom: 2px; }
.table-wrap { overflow-x: auto; }
table.list { width: 100%; border-collapse: collapse; font-size: 12.5px; }
table.list th { text-align: left; font-weight: 500; color: var(--text-3); font-size: 11px; text-transform: uppercase; letter-spacing: .06em; padding: 9px 14px; border-bottom: 1px solid var(--border); white-space: nowrap; }
table.list td { padding: 9px 14px; border-bottom: 1px solid var(--line-soft); vertical-align: top; }
table.list tr:last-child td { border-bottom: 0; }
table.list tr.failed td:first-child { box-shadow: inset 3px 0 var(--crit); }
table.list details summary { cursor: pointer; color: var(--text-2); }
.nowrap { white-space: nowrap; }
.mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
.ok-mark { color: var(--ok); font-weight: 550; }
</style>

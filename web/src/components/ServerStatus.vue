<script setup lang="ts">
import { ref, onMounted, computed, watch } from 'vue'
import { aboutApi } from '@/api'
import type { About } from '@/api/types'
import { useBrandingStore } from '@/stores/branding'
import { useAuthStore } from '@/stores/auth'
import { ApiError } from '@/lib/api'
import { fmtDate } from '@/lib/format'
import Overlay from '@/components/ui/Overlay.vue'

const branding = useBrandingStore()
const auth = useAuthStore()
const about = ref<About | null>(null)
const online = ref(false)
const open = ref(false)

async function refresh() {
  try {
    about.value = await aboutApi.get()
    online.value = true
  } catch (e) {
    // An HTTP error means the server answered: only a network failure is "offline".
    online.value = e instanceof ApiError
  }
}
onMounted(refresh)
// /about is refused while a forced password change or two-factor setup is
// pending; fetch it again once that is done.
watch(
  () => !auth.user?.must_change_password && !auth.mustSetupOtp,
  (ready, was) => {
    if (ready && !was) refresh()
  },
)

// The dot reflects the update-check result:
//   red    = backend unreachable
//   grey   = update checking disabled, or no release info yet
//   amber  = a newer release is available
//   green  = checked against the release server and up to date
const dotClass = computed(() => {
  if (!online.value) return 'bg-crit'
  const a = about.value
  if (!a || !a.update_check_enabled || !a.latest_version) return 'bg-text-3'
  return a.update_available ? 'bg-warn' : 'bg-ok'
})
const statusLabel = computed(() => {
  if (!online.value) return 'Server offline'
  const a = about.value
  if (!a || !a.update_check_enabled) return 'Updates off'
  if (!a.latest_version) return 'No release info'
  return a.update_available ? 'Update available' : 'Up to date'
})

// The backend reports the version with or without a leading "v" (dev default
// "1.0.0", release builds may stamp "v1.2.3"); normalize to exactly one "v".
const withV = (s?: string | null) => (s ? 'v' + String(s).replace(/^v/i, '') : '—')

// "Changes" links to the repository's release notes / changelog.
const changesUrl = computed(() => {
  if (!about.value?.repository_url) return ''
  return `${about.value.repository_url.replace(/\/+$/, '')}/releases`
})

// Version history, newest-first, collapsed to the latest few with a "show more".
const HISTORY_PREVIEW = 3
const showAllHistory = ref(false)
const history = computed(() => (about.value ? [...about.value.version_history].reverse() : []))
const shownHistory = computed(() => (showAllHistory.value ? history.value : history.value.slice(0, HISTORY_PREVIEW)))

function openDialog() {
  open.value = true
  showAllHistory.value = false
  refresh()
}
</script>

<template>
  <button
    class="flex w-full items-center justify-between gap-2 whitespace-nowrap rounded-lg px-2 py-1.5 text-[11.5px] hover:bg-surface-hover"
    @click="openDialog"
  >
    <span class="flex min-w-0 items-center gap-2">
      <span class="h-2 w-2 shrink-0 rounded-full" :class="dotClass" />
      <span class="truncate text-text-2">{{ statusLabel }}</span>
    </span>
    <span class="truncate font-mono text-text-3" :title="withV(about?.version)">{{ withV(about?.version) }}</span>
  </button>

  <Overlay :open="open" title="About" mode="centered" wide @close="open = false">
    <div v-if="about" class="grid grid-cols-2 gap-x-8 gap-y-5 text-[13px]">
      <div>
        <div class="font-semibold text-accent-text">{{ branding.appName }}</div>
        <a v-if="about.repository_url" :href="about.repository_url" target="_blank" rel="noopener" class="text-accent hover:underline">
          {{ withV(about.version) }}
        </a>
        <span v-else class="text-text-2">{{ withV(about.version) }}</span>
      </div>

      <div>
        <div class="font-semibold">Updates</div>
        <span v-if="!about.update_check_enabled" class="text-text-3">checking disabled</span>
        <span v-else-if="!about.latest_version" class="text-text-3">no release info</span>
        <span v-else-if="about.update_available" class="text-warn-text">new release: {{ withV(about.latest_version) }}</span>
        <span v-else class="text-ok-text">up to date</span>
      </div>

      <div>
        <div class="font-semibold">Repository</div>
        <a v-if="about.repository_url" :href="about.repository_url" target="_blank" rel="noopener" class="break-all text-accent hover:underline">
          {{ about.repository_url.replace(/^https?:\/\//, '') }}
        </a>
        <span v-else class="text-text-3">—</span>
      </div>
      <div>
        <div class="font-semibold">Changes</div>
        <a v-if="changesUrl" :href="changesUrl" target="_blank" rel="noopener" class="text-accent hover:underline">
          Release notes
        </a>
        <span v-else class="text-text-3">—</span>
      </div>

      <div>
        <div class="font-semibold">Database</div>
        <div class="capitalize text-text-2">{{ about.db_driver }}</div>
      </div>

      <div class="col-span-2">
        <div class="mb-1 font-semibold">Version history</div>
        <ul v-if="history.length" class="flex flex-col gap-0.5 text-text-2">
          <li v-for="(h, i) in shownHistory" :key="i">
            Installed {{ h.version }} on {{ fmtDate(h.installed_at) }}
          </li>
        </ul>
        <div v-else class="text-text-3">No history.</div>
        <button
          v-if="history.length > HISTORY_PREVIEW"
          type="button"
          class="mt-1.5 text-[12px] text-accent hover:underline"
          @click="showAllHistory = !showAllHistory"
        >
          {{ showAllHistory ? 'Show less' : `Show more (${history.length - HISTORY_PREVIEW})` }}
        </button>
      </div>
    </div>
    <div v-else class="text-[13px] text-text-3">Loading…</div>
  </Overlay>
</template>

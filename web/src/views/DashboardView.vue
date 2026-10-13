<script setup lang="ts">
// "Your access": who you are, what you may do and where, and which of the
// servers you can see you may change (as v0's Access → You).
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { ShieldCheck, Network, Settings } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useProxiesStore } from '@/stores/proxies'
import { uniq } from '@/lib/proxy'
import { GLOBAL_PRIVILEGES, SCOPED_PRIVILEGES } from '@/lib/privileges'

const auth = useAuthStore()
const px = useProxiesStore()
const { t } = useI18n()
// Servers you can see, and those you may change (enable, disable or drain).
const seen = computed(() => uniq(px.targets.map((x) => x.label)))
const mine = computed(() => uniq(px.targets.filter((x) => x.can.ready || x.can.maint || x.can.drain).map((x) => x.label)).sort())

const name = computed(() => auth.user?.display_name || auth.user?.username || '')
const roles = computed(() => (auth.user?.roles ?? []).map((r) => r.name))
const globals = computed(() =>
  auth.privileges.superuser ? GLOBAL_PRIVILEGES.filter((p) => !p.ownerOnly) : GLOBAL_PRIVILEGES.filter((p) => auth.privileges.global.includes(p.id)),
)
// Per scoped privilege: "every server" or the scope rules it's limited to.
const scoped = computed(() =>
  SCOPED_PRIVILEGES.map((p) => ({ ...p, scope: auth.privileges.scopes[p.id] ?? [] })).filter((p) => p.scope.length > 0),
)
</script>

<template>
  <div class="max-w-5xl">
    <header class="mb-6">
      <h1 class="text-[19px] font-semibold tracking-tight">{{ t('nav.yourAccess') }}</h1>
      <p class="mt-1 text-[13px] text-text-2">Signed in as <b>{{ name }}</b><span v-if="roles.length"> · {{ roles.join(', ') }}</span></p>
    </header>

    <div class="grid gap-5 lg:grid-cols-2">
      <section class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <ShieldCheck class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('access.privileges') }}</h3>
        </div>
        <div class="flex flex-col gap-4 p-5 text-[13px]">
          <div>
            <div class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-text-3">Servers</div>
            <p v-if="scoped.length === 0" class="text-text-3">No access to servers yet — ask an admin for a role.</p>
            <ul v-else class="flex flex-col gap-1.5">
              <li v-for="p in scoped" :key="p.id" class="flex items-start justify-between gap-3">
                <span class="flex items-center gap-2"><component :is="p.icon" class="h-4 w-4 text-text-3" /> {{ p.label }}</span>
                <span class="text-right text-[12px] text-text-2">
                  <template v-if="p.scope.includes('*')">every server</template>
                  <code v-for="r in p.scope" v-else :key="r" class="ml-1 rounded bg-surface-3 px-1.5 py-0.5 text-[11.5px]">{{ r }}</code>
                </span>
              </li>
            </ul>
          </div>
          <div v-if="globals.length">
            <div class="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-text-3">Administration</div>
            <ul class="flex flex-col gap-1.5">
              <li v-for="p in globals" :key="p.id" class="flex items-center gap-2"><component :is="p.icon" class="h-4 w-4 text-text-3" /> {{ p.label }}</li>
            </ul>
          </div>
        </div>
      </section>

      <section class="card overflow-hidden">
        <div class="flex items-center gap-2 border-b border-border px-5 py-3">
          <Network class="h-[18px] w-[18px] text-text-2" /><h3 class="text-[14px] font-semibold">{{ t('access.canChange') }}</h3>
        </div>
        <div class="flex flex-col gap-3 p-5 text-[13px] text-text-2">
          <p>{{ t('access.nOfM', { n: mine.length, m: seen.length }) }}</p>
          <div v-if="mine.length" class="flex flex-wrap gap-1">
            <code v-for="l in mine.slice(0, 60)" :key="l" class="rounded border border-border bg-surface-2 px-1.5 text-[11.5px] text-text">{{ l }}</code>
            <span v-if="mine.length > 60" class="text-text-3">+{{ mine.length - 60 }}</span>
          </div>
          <div class="flex flex-wrap gap-2">
            <RouterLink :to="{ name: 'proxies' }" class="btn-ghost"><Network class="h-4 w-4" /> {{ t('nav.proxies') }}</RouterLink>
            <RouterLink v-if="auth.isAdminArea" :to="{ name: 'admin' }" class="btn-ghost"><ShieldCheck class="h-4 w-4" /> {{ t('nav.admin') }}</RouterLink>
            <RouterLink :to="{ name: 'settings' }" class="btn-ghost"><Settings class="h-4 w-4" /> {{ t('nav.settings') }}</RouterLink>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Pencil, Trash2, LogIn, Copy } from 'lucide-vue-next'
import Toggle from '@/components/ui/Toggle.vue'
import Confirm from '@/components/ui/Confirm.vue'
import { oidcApi } from '@/api'
import type { OIDCConfig } from '@/api/types'
import { useToastStore } from '@/stores/toast'
import { useBrandingStore } from '@/stores/branding'

// Owner → SSO: the identity providers people sign in with, each its own
// button on the login page — OpenID Connect ones (Authentik, Entra,
// Keycloak…) and Google, which can be kept to your email domains. A
// provider's groups claim keeps its groups as Orbit groups.
const { t } = useI18n()
const toast = useToastStore()
const branding = useBrandingStore()

const list = ref<OIDCConfig[]>([])
const editing = ref<(OIDCConfig & { domainsText: string; isNew: boolean }) | null>(null)
const busy = ref(false)
const removing = ref<OIDCConfig | null>(null)
const callback = `${location.origin}/api/v1/auth/oidc/callback`

async function load() {
  try {
    list.value = await oidcApi.providers()
  } catch (e: any) {
    toast.error(e.message)
  }
}
onMounted(load)

function edit(p?: OIDCConfig, kind: 'oidc' | 'google' | 'saml' = 'oidc') {
  const base: OIDCConfig = p ?? {
    id: '', name: kind === 'google' ? 'Google' : '', kind, enabled: true, issuer: '', client_id: '', client_secret: '',
    redirect_url: kind === 'saml' ? '' : callback, scopes: '', role_claim: '', default_role: '', allow_admin: false, group_claim: '', domains: [], metadata_xml: '',
  }
  if (!p && !list.value.length) base.id = 'default'
  else if (!p) base.id = kind === 'google' && !list.value.some((x) => x.id === 'google') ? 'google' : ''
  editing.value = { ...base, domainsText: (base.domains ?? []).join(', '), isNew: !p }
}

async function save() {
  const e = editing.value
  if (!e?.id) return
  busy.value = true
  try {
    const { domainsText, isNew: _n, ...cfg } = e
    cfg.domains = domainsText.split(/[\s,]+/).filter(Boolean)
    await oidcApi.saveProvider(e.id, cfg)
    editing.value = null
    toast.success(t('adm.sso.saved'))
    await Promise.all([load(), branding.load()])
  } catch (err: any) {
    toast.error(err.message)
  } finally {
    busy.value = false
  }
}
async function remove() {
  const p = removing.value
  removing.value = null
  if (!p?.id) return
  try {
    await oidcApi.removeProvider(p.id)
    await Promise.all([load(), branding.load()])
  } catch (e: any) {
    toast.error(e.message)
  }
}
// Orbit's side of a SAML provider, for the identity provider.
const spBase = (id?: string) => `${location.origin}/api/v1/auth/saml/${id || '<id>'}`
async function copy(s: string) {
  await navigator.clipboard?.writeText(s).catch(() => {})
  toast.info(t('scim.copied'))
}
async function test() {
  const issuer = editing.value?.kind === 'google' ? 'https://accounts.google.com' : editing.value?.issuer
  if (!issuer) return
  try {
    await oidcApi.testDiscovery(issuer)
    toast.success(t('adm.sso.discoveryOk'))
  } catch (e: any) {
    toast.error(e.message)
  }
}
</script>

<template>
  <div class="flex flex-col gap-4" data-testid="sso-providers">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h3 class="text-[14px] font-semibold">{{ t('adm.sso.title') }}</h3>
        <div class="text-[12px] text-text-3">{{ t('adm.sso.hintMany') }}</div>
      </div>
      <div v-if="!editing" class="flex gap-2">
        <button class="btn btn-sm" data-testid="sso-add-google" @click="edit(undefined, 'google')"><Plus class="h-4 w-4" />Google</button>
        <button class="btn btn-sm" data-testid="sso-add-saml" @click="edit(undefined, 'saml')"><Plus class="h-4 w-4" />SAML</button>
        <button class="btn-primary btn-sm" data-testid="sso-add" @click="edit()"><Plus class="h-4 w-4" />{{ t('adm.sso.add') }}</button>
      </div>
    </div>

    <template v-if="!editing">
      <div v-for="p in list" :key="p.id" class="card flex items-center gap-3 px-4 py-3" data-testid="sso-provider">
        <span class="tile h-8 w-8 shrink-0" :style="{ '--tile': p.kind === 'google' ? '#4285f4' : '#5856d6' }"><LogIn class="h-4 w-4" /></span>
        <div class="min-w-0 flex-1">
          <div class="text-[14px] font-semibold">{{ p.name }} <span class="font-mono text-[11.5px] font-normal text-text-3">{{ p.id }}</span></div>
          <div class="truncate text-[12px] text-text-3">
            {{ p.kind === 'google' ? 'Google' : p.kind === 'saml' ? `SAML · ${p.issuer || t('adm.sso.samlPasted')}` : p.issuer }}<template v-if="p.domains?.length"> · {{ p.domains.join(', ') }}</template><template v-if="p.group_claim"> · {{ t('adm.sso.groupsFrom', { claim: p.group_claim }) }}</template>
          </div>
        </div>
        <span class="text-[12px]" :class="p.enabled ? 'text-ok-text' : 'text-text-3'">{{ p.enabled ? t('adm.sso.on') : t('adm.sso.off') }}</span>
        <button class="btn-ghost px-2 py-1" :title="t('admin.edit')" :aria-label="t('admin.edit')" @click="edit(p)"><Pencil class="h-4 w-4" /></button>
        <button class="btn-ghost px-2 py-1 text-crit-text" :title="t('common.delete')" :aria-label="t('common.delete')" @click="removing = p"><Trash2 class="h-4 w-4" /></button>
      </div>
      <p v-if="!list.length" class="card p-6 text-center text-[13px] text-text-3">{{ t('adm.sso.none') }}</p>
    </template>

    <form v-else class="card flex flex-col gap-3 p-5" data-testid="sso-form" @submit.prevent="save">
      <div class="flex items-center justify-between gap-4">
        <h3 class="text-[14px] font-semibold">{{ editing.kind === 'google' ? 'Google' : editing.kind === 'saml' ? 'SAML 2.0' : t('adm.sso.oidc') }}</h3>
        <Toggle v-model="editing.enabled" />
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label class="label">{{ t('adm.sso.name') }}</label>
          <input v-model="editing.name" class="input" :placeholder="t('adm.sso.namePh')" data-testid="sso-name" />
        </div>
        <div>
          <label class="label">{{ t('adm.sso.id') }}</label>
          <input v-model="editing.id" class="input font-mono" :disabled="!editing.isNew" pattern="[a-z0-9][a-z0-9-]*" maxlength="32" required data-testid="sso-id" />
        </div>
      </div>
      <!-- SAML: the identity provider's metadata; Orbit's side to give it. -->
      <template v-if="editing.kind === 'saml'">
        <div>
          <label class="label">{{ t('adm.sso.samlMetadataUrl') }}</label>
          <input v-model="editing.issuer" class="input" placeholder="https://login.microsoftonline.com/…/federationmetadata/2007-06/federationmetadata.xml" data-testid="sso-saml-url" />
        </div>
        <div>
          <label class="label">{{ t('adm.sso.samlMetadataXml') }}</label>
          <textarea v-model="editing.metadata_xml" class="input min-h-[80px] font-mono text-[11.5px]" spellcheck="false" />
        </div>
        <div class="rounded-xl bg-fill-2 p-3 text-[12px]" data-testid="sso-saml-sp">
          <div class="mb-1 font-medium text-text-2">{{ t('adm.sso.samlGive') }}</div>
          <div v-for="row in [
            [t('adm.sso.samlEntity'), spBase(editing.id) + '/metadata'],
            [t('adm.sso.samlAcs'), spBase(editing.id) + '/acs'],
          ]" :key="row[0]" class="flex items-center gap-2 py-0.5">
            <span class="w-40 shrink-0 text-text-3">{{ row[0] }}</span>
            <code class="min-w-0 flex-1 truncate font-mono text-[11.5px]">{{ row[1] }}</code>
            <button type="button" class="btn-ghost px-1.5 py-0.5" :aria-label="t('scim.copy')" @click="copy(row[1])"><Copy class="h-3.5 w-3.5" /></button>
          </div>
        </div>
      </template>
      <div v-else-if="editing.kind !== 'google'">
        <label class="label">{{ t('adm.sso.issuer') }}</label>
        <div class="flex gap-2">
          <input v-model="editing.issuer" class="input" placeholder="https://idp.example.com/realms/main" data-testid="sso-issuer" />
          <button type="button" class="btn-ghost shrink-0" @click="test">{{ t('adm.test') }}</button>
        </div>
      </div>
      <p v-else class="text-[12px] text-text-3">{{ t('adm.sso.googleHint', { callback }) }}</p>
      <div v-if="editing.kind !== 'saml'" class="grid grid-cols-2 gap-3">
        <div><label class="label">{{ t('adm.sso.clientId') }}</label><input v-model="editing.client_id" class="input" data-testid="sso-client-id" /></div>
        <div>
          <label class="label">{{ t('adm.sso.clientSecret') }}</label>
          <input v-model="editing.client_secret" type="password" class="input" autocomplete="new-password" :placeholder="editing.has_secret ? t('adm.sso.secretUnchanged') : ''" />
        </div>
      </div>
      <div v-if="editing.kind !== 'saml'">
        <label class="label">{{ t('adm.sso.redirect') }}</label>
        <input v-model="editing.redirect_url" class="input" :placeholder="callback" />
        <p class="mt-1 text-[11px] text-text-3">{{ t('adm.sso.redirectHint') }}</p>
      </div>
      <div>
        <label class="label">{{ t('adm.sso.domains') }}</label>
        <input v-model="editing.domainsText" class="input" :placeholder="editing.kind === 'google' ? 'example.org' : t('adm.sso.domainsAny')" data-testid="sso-domains" />
        <p class="mt-1 text-[11px] text-text-3">{{ t('adm.sso.domainsHint') }}</p>
      </div>
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div v-if="editing.kind === 'oidc'"><label class="label">{{ t('adm.sso.scopes') }}</label><input v-model="editing.scopes" class="input" placeholder="groups" /></div>
        <div v-if="editing.kind !== 'google'"><label class="label">{{ editing.kind === 'saml' ? t('adm.sso.groupAttr') : t('adm.sso.groupClaim') }}</label><input v-model="editing.group_claim" class="input" placeholder="groups" data-testid="sso-group-claim" /></div>
        <div v-if="editing.kind !== 'google'"><label class="label">{{ editing.kind === 'saml' ? t('adm.sso.roleAttr') : t('adm.sso.roleClaim') }}</label><input v-model="editing.role_claim" class="input" placeholder="roles" /></div>
        <div><label class="label">{{ t('adm.sso.defaultRole') }}</label><input v-model="editing.default_role" class="input" :placeholder="t('adm.sso.defaultRoleNone')" /></div>
      </div>
      <p class="text-[11px] text-text-3">{{ t('adm.sso.provisioning') }} {{ t('adm.sso.groupsHint') }}</p>
      <div class="flex items-center justify-between gap-4 rounded-xl border border-border bg-surface-2 px-4 py-3">
        <div>
          <div class="text-[13px] font-semibold">{{ t('admin.ssoAllowAdmin') }}</div>
          <div class="text-[12px] text-text-3">{{ t('admin.ssoAllowAdminHint') }}</div>
        </div>
        <Toggle :model-value="!!editing.allow_admin" @update:model-value="(v) => editing && (editing.allow_admin = v)" />
      </div>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn" @click="editing = null">{{ t('common.cancel') }}</button>
        <button type="submit" class="btn-primary" :disabled="busy || !editing.id" data-testid="sso-save">{{ busy ? t('common.saving') : t('adm.sso.save') }}</button>
      </div>
    </form>
    <Confirm :open="!!removing" :title="t('adm.sso.removeTitle')" :message="t('adm.sso.removeAsk', { name: removing?.name ?? '' })" :confirm-label="t('common.delete')" danger @confirm="remove" @close="removing = null" />
  </div>
</template>

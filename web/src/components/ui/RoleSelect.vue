<script setup lang="ts">
import { ref, computed, reactive, watch, onUnmounted, nextTick } from 'vue'
import type { Role } from '@/api/types'
import { ShieldCheck, Wrench, Eye, KeyRound, Search, Check, X, ChevronDown } from 'lucide-vue-next'

// Chip-based multi-select with a searchable dropdown. The dropdown is teleported
// to <body> and fixed-positioned under the control, so it floats above the modal
// instead of expanding it.
const props = defineProps<{ modelValue: string[]; roles: Role[] }>()
const emit = defineEmits<{ 'update:modelValue': [string[]] }>()

const open = ref(false)
const query = ref('')
const control = ref<HTMLElement | null>(null)
const menu = reactive({ top: 0, left: 0, width: 0 })

const selected = computed(() => props.roles.filter((r) => props.modelValue.includes(r.id)))
const filtered = computed(() => {
  const s = query.value.trim().toLowerCase()
  if (!s) return props.roles
  return props.roles.filter((r) => r.name.toLowerCase().includes(s) || (r.description || '').toLowerCase().includes(s))
})

function reposition() {
  if (!control.value) return
  const r = control.value.getBoundingClientRect()
  menu.top = r.bottom + 6
  menu.left = r.left
  menu.width = r.width
}
function openMenu() {
  reposition()
  open.value = true
  query.value = ''
  nextTick(reposition)
}
function close() {
  open.value = false
}

watch(open, (o) => {
  if (o) {
    window.addEventListener('scroll', reposition, true)
    window.addEventListener('resize', reposition)
  } else {
    window.removeEventListener('scroll', reposition, true)
    window.removeEventListener('resize', reposition)
  }
})
onUnmounted(() => {
  window.removeEventListener('scroll', reposition, true)
  window.removeEventListener('resize', reposition)
})

function isSelected(id: string) {
  return props.modelValue.includes(id)
}
function toggle(id: string) {
  emit('update:modelValue', isSelected(id) ? props.modelValue.filter((x) => x !== id) : [...props.modelValue, id])
}
function remove(id: string) {
  emit('update:modelValue', props.modelValue.filter((x) => x !== id))
}
function iconFor(name: string) {
  switch (name.toLowerCase()) {
    case 'admin':
      return ShieldCheck
    case 'operator':
      return Wrench
    case 'viewer':
      return Eye
    default:
      return KeyRound
  }
}
</script>

<template>
  <div>
    <!-- Control -->
    <div
      ref="control"
      class="flex min-h-[40px] cursor-pointer flex-wrap items-center gap-1.5 rounded-xl border bg-surface-2 px-2.5 py-1.5"
      :class="open ? 'border-accent' : 'border-border'"
      @click="open ? close() : openMenu()"
    >
      <span
        v-for="r in selected"
        :key="r.id"
        class="inline-flex items-center gap-1 rounded-md bg-accent-bg px-1.5 py-0.5 text-[12px] text-accent-text"
        @click.stop
      >
        <component :is="iconFor(r.name)" class="h-3.5 w-3.5" />
        {{ r.name }}
        <button type="button" class="opacity-70 hover:opacity-100" @click.stop="remove(r.id)"><X class="h-3 w-3" /></button>
      </span>
      <span v-if="selected.length === 0" class="px-1 text-[13px] text-text-3">Select roles…</span>
      <ChevronDown class="ml-auto h-4 w-4 text-text-3 transition" :class="{ 'rotate-180': open }" />
    </div>

    <!-- Floating dropdown (teleported) -->
    <Teleport to="body">
      <template v-if="open">
        <div class="fixed inset-0 z-[60]" @click="close" />
        <div
          class="fixed z-[61] overflow-hidden rounded-xl border border-border bg-surface shadow-soft"
          :style="{ top: menu.top + 'px', left: menu.left + 'px', width: menu.width + 'px' }"
        >
          <div class="border-b border-border p-2">
            <div class="relative">
              <Search class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-text-3" />
              <input
                v-model="query"
                class="w-full rounded-lg border border-border bg-surface-2 py-1.5 pl-8 pr-2 text-[13px] outline-none focus:border-accent"
                placeholder="Search roles…"
              />
            </div>
          </div>
          <div class="max-h-56 overflow-y-auto p-1">
            <button
              v-for="r in filtered"
              :key="r.id"
              type="button"
              class="flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left transition hover:bg-surface-2"
              :class="{ 'bg-accent-bg': isSelected(r.id) }"
              @click="toggle(r.id)"
            >
              <component :is="iconFor(r.name)" class="h-[18px] w-[18px] shrink-0" :class="isSelected(r.id) ? 'text-accent-text' : 'text-text-3'" />
              <div class="min-w-0 flex-1">
                <div class="text-[13px] font-medium" :class="isSelected(r.id) ? 'text-accent-text' : 'text-text'">{{ r.name }}</div>
                <div class="truncate text-[12px] text-text-3">{{ r.description || '—' }}</div>
              </div>
              <Check v-if="isSelected(r.id)" class="h-4 w-4 shrink-0 text-accent-text" />
            </button>
            <div v-if="filtered.length === 0" class="px-2.5 py-3 text-center text-[12px] text-text-3">No roles found.</div>
          </div>
        </div>
      </template>
    </Teleport>
  </div>
</template>

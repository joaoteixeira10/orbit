<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { personColor, type Present } from '@/lib/presence'

// Who else is here: a dot of each person's initials (once each, however many
// tabs), a pen on the ones editing; the rest as "+N".
const props = defineProps<{ people: Present[] }>()
const { t } = useI18n()
const MAX = 4
const each = computed(() => {
  const by = new Map<string, Present & { tabs: number }>()
  for (const p of props.people) {
    const had = by.get(p.user_id)
    if (had) by.set(p.user_id, { ...had, editing: had.editing || p.editing, tabs: had.tabs + 1 })
    else by.set(p.user_id, { ...p, tabs: 1 })
  }
  return [...by.values()]
})
const initials = (name: string) =>
  name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]!.toUpperCase())
    .join('') || '?'
</script>

<template>
  <div v-if="each.length" class="presence" :aria-label="t('presence.here', { n: each.length }, each.length)" data-testid="presence">
    <span
      v-for="p in each.slice(0, MAX)"
      :key="p.user_id"
      class="dot"
      :class="{ editing: p.editing }"
      :style="{ '--c': personColor(p.user_id) }"
      :title="t(p.editing ? 'presence.editing' : 'presence.looking', { name: p.name })"
      data-testid="presence-person"
      >{{ initials(p.name) }}</span
    >
    <span v-if="each.length > MAX" class="dot more">+{{ each.length - MAX }}</span>
  </div>
</template>

<style scoped>
.presence {
  display: flex;
  align-items: center;
}
.dot {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  margin-left: -6px;
  border-radius: 999px;
  border: 2px solid var(--bg);
  background: var(--c);
  color: #fff;
  font-size: 10.5px;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.dot:first-child {
  margin-left: 0;
}
.dot.editing {
  box-shadow: 0 0 0 2px var(--c);
}
.dot.more {
  background: var(--fill);
  color: var(--text-2);
}
</style>

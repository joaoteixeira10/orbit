<script setup lang="ts">
import { computed } from 'vue'
import Avatar from './Avatar.vue'
import type { Member } from '@/api/planner'

// Several people as overlapping avatars (the first few, then "+n"), like the
// assignees of a task in Planner.
const props = withDefaults(defineProps<{ people: Member[]; size?: number; max?: number }>(), { size: 20, max: 3 })
const shown = computed(() => props.people.slice(0, props.people.length > props.max ? props.max - 1 : props.max))
const more = computed(() => props.people.length - shown.value.length)
const names = computed(() => props.people.map((m) => m.display_name || m.username).join(', '))
</script>

<template>
  <span v-if="people.length" class="stack" :title="names" :aria-label="names" :style="{ '--s': size + 'px' }">
    <Avatar v-for="m in shown" :key="m.user_id" :name="m.display_name || m.username" :src="m.avatar_url" :size="size" class="ring" />
    <span v-if="more > 0" class="more ring">+{{ more }}</span>
  </span>
</template>

<style scoped>
.stack {
  display: inline-flex;
  flex: none;
  align-items: center;
}
.stack > * + * {
  margin-left: calc(var(--s) * -0.3);
}
.ring {
  box-shadow: 0 0 0 1.5px var(--surface);
  border-radius: 999px;
}
.more {
  display: grid;
  place-items: center;
  width: var(--s);
  height: var(--s);
  background: var(--fill);
  color: var(--text-2);
  font-size: calc(var(--s) * 0.45);
  font-weight: 600;
}
</style>

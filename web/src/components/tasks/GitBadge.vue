<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { CircleDot, CircleCheck, GitPullRequest, GitMerge } from 'lucide-vue-next'
import type { Task } from '@/api/planner'

// A task's issue or pull request, as git shows it: its number in the
// colour of its state (open green, closed or merged purple), a link to it.
// An orange dot: a change hasn't reached git yet.
const props = defineProps<{ task: Task }>()
const { t } = useI18n()

const state = computed(() => props.task.git_state ?? 'open')
const icon = computed(() => {
  if (props.task.git_pull) return state.value === 'merged' ? GitMerge : GitPullRequest
  return state.value === 'open' ? CircleDot : CircleCheck
})
const label = computed(() => {
  const kind = props.task.git_pull ? t('git.task.pull') : t('git.task.issue')
  let s = `${kind} ${props.task.git_repo}#${props.task.git_number} · ${t(`git.task.state.${state.value}`)}`
  if (props.task.git_dirty) s += ` — ${t('git.task.pending')}`
  return s
})
</script>

<template>
  <a
    v-if="task.git_number"
    :href="task.git_url"
    target="_blank"
    rel="noopener noreferrer"
    class="git-badge"
    :class="state"
    :title="label"
    :aria-label="label"
    @click.stop
  >
    <component :is="icon" class="h-3 w-3" :stroke-width="2.4" />#{{ task.git_number }}
    <span v-if="task.git_dirty" class="dirty" />
  </a>
</template>

<style scoped>
.git-badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-variant-numeric: tabular-nums;
  color: #248a3d;
}
:global(html.dark) .git-badge {
  color: #30d158;
}
.git-badge.closed,
.git-badge.merged {
  color: #8944ab;
}
:global(html.dark) .git-badge.closed,
:global(html.dark) .git-badge.merged {
  color: #bf5af2;
}
.git-badge:hover {
  text-decoration: underline;
}
.dirty {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: var(--warn, #ff9500);
}
</style>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Loader2, Minus, X } from 'lucide-vue-next'
import Checkbox from '@/components/ui/Checkbox.vue'
import type { AssistantAction } from '@/api/assistant'

// The changes an answer lined up in Agent mode, in one card: each ticked to
// start with; untick what you don't want and approve once (or discard them
// all). Once decided, what became of each.
const props = defineProps<{ actions: AssistantAction[]; busy: boolean }>()
const emit = defineEmits<{ apply: [approve: number[]] }>()
const { t } = useI18n()

const open = computed(() => props.actions.some((a) => a.state === 'proposed'))
const ticked = ref(new Set(props.actions.map((a) => a.n)))
function tick(n: number, on: boolean) {
  const s = new Set(ticked.value)
  if (on) s.add(n)
  else s.delete(n)
  ticked.value = s
}
const chosen = computed(() => props.actions.filter((a) => ticked.value.has(a.n)).map((a) => a.n))
const made = computed(() => props.actions.filter((a) => a.state === 'done').length)
</script>

<template>
  <div class="plan" data-testid="assistant-plan">
    <div class="plan-head">
      {{ open ? t('assistant.plan.title', actions.length) : t('assistant.plan.decided', { n: made }, made) }}
    </div>
    <ul>
      <li v-for="a in actions" :key="a.n" class="plan-row" :class="{ crit: a.destructive }" data-testid="assistant-change">
        <Checkbox v-if="open" :model-value="ticked.has(a.n)" :disabled="busy" :aria-label="a.summary || a.title" @update:model-value="tick(a.n, $event)" />
        <span v-else class="plan-state" :class="a.state" :title="t(`assistant.plan.state.${a.state}`)">
          <Check v-if="a.state === 'done'" class="h-3 w-3" :stroke-width="3" />
          <X v-else-if="a.state === 'failed'" class="h-3 w-3" :stroke-width="3" />
          <Minus v-else class="h-3 w-3" :stroke-width="3" />
        </span>
        <div class="min-w-0 flex-1">
          <div class="plan-summary" :class="{ off: !open && a.state !== 'done' }">{{ a.summary || a.title }}</div>
          <div class="plan-kind">
            {{ a.title }}
            <template v-if="!open && a.state !== 'done' && a.result"> · {{ a.state === 'skipped' && a.result === 'not approved' ? t('assistant.plan.state.skipped') : a.result }}</template>
          </div>
        </div>
      </li>
    </ul>
    <div v-if="open" class="plan-foot">
      <button class="btn-ghost btn-sm" :disabled="busy" data-testid="assistant-discard" @click="emit('apply', [])">{{ t('assistant.plan.discard') }}</button>
      <button class="btn-primary btn-sm" :disabled="busy || !chosen.length" data-testid="assistant-approve" @click="emit('apply', chosen)">
        <Loader2 v-if="busy" class="h-3.5 w-3.5 animate-spin" />
        {{ t('assistant.plan.approve', { n: chosen.length }, chosen.length) }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.plan {
  margin-top: 8px;
  overflow: hidden;
  border-radius: 14px;
  background: var(--surface);
  box-shadow: inset 0 0 0 0.5px var(--border-2);
}
.plan-head {
  padding: 9px 12px 7px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-2);
}
.plan-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 7px 12px;
  border-top: 0.5px solid var(--border);
}
.plan-row :deep(button) {
  margin-top: 1px;
}
.plan-summary {
  font-size: 13.5px;
  line-height: 1.35;
  word-break: break-word;
}
.plan-summary.off {
  color: var(--text-3);
  text-decoration: line-through;
}
.crit .plan-summary:not(.off) {
  color: var(--crit-text);
}
.plan-kind {
  margin-top: 1px;
  font-size: 11.5px;
  color: var(--text-3);
}
.plan-state {
  display: grid;
  flex: none;
  width: 17px;
  height: 17px;
  margin-top: 1px;
  place-items: center;
  border-radius: 999px;
  background: var(--fill);
  color: var(--text-3);
}
.plan-state.done {
  background: var(--ok, #34c759);
  color: #fff;
}
.plan-state.failed {
  background: var(--crit-text);
  color: #fff;
}
.plan-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 9px 12px;
  border-top: 0.5px solid var(--border);
}
</style>

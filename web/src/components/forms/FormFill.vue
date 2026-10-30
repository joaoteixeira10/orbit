<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckCircle2 } from 'lucide-vue-next'
import type { FillForm } from '@/lib/shareApi'
import { ApiError } from '@/lib/api'

// A form, filled in: one question per field (a text, a number, a date,
// yes / no, one or several choices, a person), the required ones marked.
// Sent, it says thank you (and offers another answer).
const props = defineProps<{ form: FillForm; send: (values: Record<string, unknown>) => Promise<{ thanks?: string }> }>()
const { t } = useI18n()

const values = reactive<Record<string, unknown>>({})
const hp = ref('') // only robots fill it in
const error = ref('')
const bad = ref('')
const busy = ref(false)
const done = ref(false)
// Answering the question that was missing clears the warning.
watch(
  values,
  () => {
    const v = bad.value ? values[bad.value] : undefined
    if (bad.value && v !== undefined && v !== '' && !(Array.isArray(v) && !v.length)) {
      bad.value = ''
      error.value = ''
    }
  },
  { deep: true },
)
function reset() {
  for (const k of Object.keys(values)) delete values[k]
  done.value = false
}
function toggle(prop: string, id: string) {
  const cur = (values[prop] as string[] | undefined) ?? []
  values[prop] = cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]
}
async function submit() {
  error.value = ''
  bad.value = ''
  for (const f of props.form.fields) {
    const v = values[f.prop]
    if (f.required && f.type !== 'checkbox' && (v === undefined || v === '' || (Array.isArray(v) && !v.length))) {
      error.value = t('forms.needed', { name: f.label })
      bad.value = f.prop
      return
    }
  }
  busy.value = true
  try {
    await props.send({ ...values, ...(hp.value ? { _hp: hp.value } : {}) })
    done.value = true
  } catch (e: any) {
    error.value = e.message
    if (e instanceof ApiError) bad.value = e.body?.field ?? ''
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="form-fill">
    <h1 class="text-[28px] font-bold leading-tight" data-testid="form-heading">{{ form.title }}</h1>
    <p v-if="form.intro" class="mt-2 whitespace-pre-line text-[14.5px] leading-relaxed text-text-2">{{ form.intro }}</p>

    <div v-if="done" class="ff-done" data-testid="form-done">
      <CheckCircle2 class="h-10 w-10 text-[#34c759]" />
      <p class="text-[17px] font-semibold">{{ form.thanks || t('forms.thanks') }}</p>
      <button type="button" class="btn-plain" @click="reset">{{ t('forms.another') }}</button>
    </div>
    <p v-else-if="!form.open" class="ff-closed" data-testid="form-closed">{{ t('forms.closed') }}</p>
    <form v-else class="mt-6 flex flex-col gap-5" novalidate @submit.prevent="submit">
      <div v-for="f in form.fields" :key="f.prop" class="ff-q" :class="{ bad: bad === f.prop }">
        <label :for="`q-${f.prop}`" class="ff-label">{{ f.label }}<span v-if="f.required" class="text-[#ff3b30]"> *</span></label>
        <p v-if="f.help" class="ff-help">{{ f.help }}</p>
        <input v-if="f.type === 'title' || f.type === 'url'" :id="`q-${f.prop}`" v-model="values[f.prop] as string" class="input" :type="f.type === 'url' ? 'url' : 'text'" :maxlength="f.type === 'title' ? 300 : 2000" :data-testid="`form-${f.prop}`" />
        <textarea v-else-if="f.type === 'text'" :id="`q-${f.prop}`" v-model="values[f.prop] as string" class="input" rows="3" maxlength="2000" :data-testid="`form-${f.prop}`" />
        <input v-else-if="f.type === 'number'" :id="`q-${f.prop}`" v-model.number="values[f.prop] as number" class="input !w-40" type="number" step="any" :data-testid="`form-${f.prop}`" />
        <input v-else-if="f.type === 'date'" :id="`q-${f.prop}`" v-model="values[f.prop] as string" class="input !w-48" type="date" :data-testid="`form-${f.prop}`" />
        <label v-else-if="f.type === 'checkbox'" class="flex items-center gap-2 text-[14px]"><input :id="`q-${f.prop}`" v-model="values[f.prop] as boolean" type="checkbox" :data-testid="`form-${f.prop}`" />{{ t('forms.yes') }}</label>
        <div v-else-if="f.type === 'select' || f.type === 'person'" class="ff-choices" role="radiogroup">
          <label v-for="o in f.options" :key="o.id" class="ff-choice" :class="{ on: values[f.prop] === o.id }">
            <input v-model="values[f.prop]" type="radio" :name="`q-${f.prop}`" :value="o.id" :data-testid="`form-${f.prop}-${o.name}`" />
            <span class="dot" :style="o.color ? { background: o.color } : {}" />{{ o.name }}
          </label>
        </div>
        <div v-else-if="f.type === 'multi_select'" class="ff-choices">
          <label v-for="o in f.options" :key="o.id" class="ff-choice" :class="{ on: ((values[f.prop] as string[]) ?? []).includes(o.id) }">
            <input type="checkbox" :checked="((values[f.prop] as string[]) ?? []).includes(o.id)" :data-testid="`form-${f.prop}-${o.name}`" @change="toggle(f.prop, o.id)" />
            <span class="dot" :style="o.color ? { background: o.color } : {}" />{{ o.name }}
          </label>
        </div>
      </div>
      <input v-model="hp" class="ff-hp" tabindex="-1" autocomplete="off" aria-hidden="true" name="website" />
      <p v-if="error" class="rounded-lg bg-crit-bg px-3 py-2 text-[13px] text-crit-text" role="alert" data-testid="form-error">{{ error }}</p>
      <div><button type="submit" class="btn-primary btn-lg" :disabled="busy" data-testid="form-send">{{ t('forms.send') }}</button></div>
    </form>
  </div>
</template>

<style scoped>
.ff-label {
  display: block;
  font-size: 14.5px;
  font-weight: 600;
}
.ff-help {
  margin: 2px 0 6px;
  font-size: 12.5px;
  color: var(--text-3);
}
.ff-q .input {
  margin-top: 6px;
}
.ff-q.bad .input,
.ff-q.bad .ff-choices {
  box-shadow: 0 0 0 2px #ff3b30;
  border-radius: 10px;
}
.ff-choices {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 6px;
}
.ff-choice {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 6px 12px;
  border-radius: 999px;
  background: var(--fill-2);
  font-size: 13.5px;
  cursor: pointer;
}
.ff-choice.on {
  background: var(--accent-bg, var(--fill));
  color: var(--accent-text, var(--accent));
  font-weight: 600;
}
.ff-choice input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}
.ff-choice .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-3);
}
.ff-done {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  margin-top: 40px;
  text-align: center;
}
.ff-closed {
  margin-top: 30px;
  padding: 14px;
  border-radius: 12px;
  background: var(--fill-2);
  font-size: 14px;
  color: var(--text-2);
}
.ff-hp {
  position: absolute;
  left: -10000px;
  width: 1px;
  height: 1px;
}
</style>

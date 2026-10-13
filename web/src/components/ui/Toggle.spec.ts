import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Toggle from './Toggle.vue'

describe('Toggle', () => {
  it('reflects modelValue via aria-checked', () => {
    const on = mount(Toggle, { props: { modelValue: true } })
    expect(on.get('button').attributes('aria-checked')).toBe('true')
    const off = mount(Toggle, { props: { modelValue: false } })
    expect(off.get('button').attributes('aria-checked')).toBe('false')
  })

  it('emits the toggled value on click', async () => {
    const w = mount(Toggle, { props: { modelValue: false } })
    await w.get('button').trigger('click')
    expect(w.emitted('update:modelValue')).toEqual([[true]])

    const w2 = mount(Toggle, { props: { modelValue: true } })
    await w2.get('button').trigger('click')
    expect(w2.emitted('update:modelValue')).toEqual([[false]])
  })
})

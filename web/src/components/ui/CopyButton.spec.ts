import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import CopyButton from './CopyButton.vue'

describe('CopyButton', () => {
  beforeEach(() => {
    // jsdom has no clipboard; stub it.
    Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
  })

  it('writes the value to the clipboard on click', async () => {
    const w = mount(CopyButton, { props: { value: 'orb_secret_token' } })
    await w.get('button').trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('orb_secret_token')
  })

  it('shows the optional label and flips to "Copied" after copying', async () => {
    const w = mount(CopyButton, { props: { value: 'x', label: 'Copy' } })
    expect(w.text()).toContain('Copy')
    await w.get('button').trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Copied')
  })
})

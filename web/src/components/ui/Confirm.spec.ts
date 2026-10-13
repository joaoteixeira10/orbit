import { describe, it, expect, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import Confirm from './Confirm.vue'

// Confirm renders inside Overlay, which <Teleport>s to <body> — so we assert
// against document.body rather than the wrapper subtree.
function bodyButtons() {
  return Array.from(document.body.querySelectorAll('button'))
}

describe('Confirm', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('renders title/message and a danger action button, emits confirm', async () => {
    const w = mount(Confirm, {
      props: { open: true, title: 'Delete user?', message: 'Permanently delete bob?', confirmLabel: 'Delete', danger: true },
    })
    expect(document.body.textContent).toContain('Delete user?')
    expect(document.body.textContent).toContain('Permanently delete bob?')

    const del = bodyButtons().find((b) => b.textContent?.trim() === 'Delete')
    expect(del).toBeTruthy()
    expect(del!.className).toContain('btn-danger')

    del!.click()
    expect(w.emitted('confirm')).toBeTruthy()
    expect(w.emitted('close')).toBeFalsy()
  })

  it('defaults to a primary "Confirm" button and emits close on cancel', async () => {
    const w = mount(Confirm, { props: { open: true, title: 'T', message: 'M' } })

    const confirm = bodyButtons().find((b) => b.textContent?.trim() === 'Confirm')
    expect(confirm!.className).toContain('btn-primary')

    const cancel = bodyButtons().find((b) => b.textContent?.trim() === 'Cancel')
    cancel!.click()
    expect(w.emitted('close')).toBeTruthy()
    expect(w.emitted('confirm')).toBeFalsy()
  })

  it('renders nothing when closed', () => {
    mount(Confirm, { props: { open: false, title: 'Hidden', message: 'nope' } })
    expect(document.body.textContent).not.toContain('Hidden')
  })

  it('with typeToConfirm, enables the action only after the exact text is typed', async () => {
    const w = mount(Confirm, {
      props: { open: true, title: 'Transfer?', message: 'm', confirmLabel: 'Transfer', danger: true, typeToConfirm: 'bob' },
    })
    const btn = () => bodyButtons().find((b) => b.textContent?.trim() === 'Transfer')!
    const input = document.body.querySelector('input') as HTMLInputElement
    expect(btn().disabled).toBe(true)
    input.value = 'bo'
    input.dispatchEvent(new Event('input'))
    await w.vm.$nextTick()
    expect(btn().disabled).toBe(true)
    input.value = 'bob'
    input.dispatchEvent(new Event('input'))
    await w.vm.$nextTick()
    expect(btn().disabled).toBe(false)
    btn().click()
    expect(w.emitted('confirm')).toBeTruthy()
  })
})

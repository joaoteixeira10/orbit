import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Badge from './Badge.vue'

describe('Badge', () => {
  it('humanizes the status label (underscores → spaces)', () => {
    expect(mount(Badge, { props: { status: 'pending_chain' } }).text()).toBe('pending chain')
    expect(mount(Badge, { props: { status: 'valid' } }).text()).toBe('valid')
  })

  it('maps statuses to the right tone class', () => {
    const cls = (s: string) => mount(Badge, { props: { status: s } }).get('span').classes()
    expect(cls('active')).toContain('text-ok-text')
    expect(cls('expiring')).toContain('text-warn-text')
    expect(cls('revoked')).toContain('text-crit-text')
    expect(cls('offline_sealed')).toContain('text-text-2') // neutral
    expect(cls('something-unknown')).toContain('text-text-2') // default → neutral
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'

// The lock's crypto, against a stand-in for /me/lock.
let stored: any = { set: false, sealed: 0 }
vi.mock('./api', () => ({
  api: vi.fn(async (path: string, init?: RequestInit) => {
    if (path !== '/me/lock') throw new Error(path)
    if (init?.method === 'PUT') stored = { set: true, sealed: 0, ...JSON.parse(String(init.body)) }
    return stored
  }),
}))

import { changePin, lockNow, seal, setUp, unlock, unlocked, unseal, WrongPin } from './lock'

describe('lock', () => {
  beforeEach(() => {
    stored = { set: false, sealed: 0 }
    lockNow()
  })

  it('seals with the PIN’s key, opens again with the same PIN only, and survives a new PIN', { timeout: 30000 }, async () => {
    await setUp('2468')
    expect(unlocked.value).toBe(true)
    expect(stored.iterations).toBe(600000)
    const c = await seal({ content: 'my diary' })
    expect(c).not.toContain('diary')
    expect(await unseal(c)).toEqual({ content: 'my diary' })

    lockNow()
    await expect(unseal(c)).rejects.toThrow('locked')
    await expect(unlock('1111')).rejects.toBeInstanceOf(WrongPin)
    await unlock('2468')
    expect(await unseal(c)).toEqual({ content: 'my diary' })

    await changePin('2468', 'new-pin-99')
    lockNow()
    await expect(unlock('2468')).rejects.toBeInstanceOf(WrongPin)
    await unlock('new-pin-99')
    expect(await unseal(c)).toEqual({ content: 'my diary' }) // the same key: nothing re-sealed
  })
})

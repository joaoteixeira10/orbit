import { afterEach, describe, expect, it, vi } from 'vitest'
import { getAccessToken, setAccessToken, tryRefresh } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
  setAccessToken(null)
})

describe('tryRefresh', () => {
  it('coalesces concurrent refreshes into a single request', async () => {
    let resolve!: (r: Response) => void
    const fetchMock = vi.fn(() => new Promise<Response>((r) => (resolve = r)))
    vi.stubGlobal('fetch', fetchMock)

    const a = tryRefresh()
    const b = tryRefresh()
    resolve(new Response(JSON.stringify({ access_token: 'fresh' }), { status: 200 }))

    expect(await a).toBe(true)
    expect(await b).toBe(true)
    // Two parallel 401s must not spend the rotating refresh cookie twice.
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(getAccessToken()).toBe('fresh')
  })

  it('refreshes again once the previous attempt has settled', async () => {
    const fetchMock = vi.fn(async () => new Response('{}', { status: 401 }))
    vi.stubGlobal('fetch', fetchMock)

    expect(await tryRefresh()).toBe(false)
    expect(await tryRefresh()).toBe(false)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useProfilesStore } from './profiles'

describe('profiles store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ ok: true }) } as Response)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ profiles: [] }) } as Response))
  })

  it('uses the route id without duplicating it in the validated request body', async () => {
    const store = useProfilesStore()
    await store.save({
      id: '0123456789abcdef0123456789abcdef',
      name: 'Living room', host: '192.168.1.2', port: 2120,
      username: 'anonymous', password: '', base_path: '/', preset: 'zftpd',
    })

    const fetch = vi.mocked(globalThis.fetch)
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('/api/v1/profiles/0123456789abcdef0123456789abcdef')
    expect(init).toEqual(expect.objectContaining({ method: 'PUT' }))
    expect(JSON.parse(String(init?.body))).toEqual({
      name: 'Living room', host: '192.168.1.2', port: 2120,
      username: 'anonymous', password: '', base_path: '/', preset: 'zftpd',
    })
  })
})

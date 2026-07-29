import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Profile } from '../types'
import { useProfilesStore } from './profiles'

const livingRoom: Profile = {
  id: 'living-room', name: 'Living room', host: '192.168.1.2', port: 2120,
  username: 'anonymous', password: '', base_path: '/', preset: 'zftpd',
}

const bedroom: Profile = {
  id: 'bedroom', name: 'Bedroom', host: '192.168.1.3', port: 2121,
  username: '', password: '', base_path: '/', preset: 'ftpsrv',
}

describe('profiles store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: null }) } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: { profiles: [] } }) } as Response))
  })

  afterEach(() => vi.unstubAllGlobals())

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

  it('keeps the selected PS5 profile valid when the available list changes', () => {
    const store = useProfilesStore()
    store.selectedId = 'deleted'

    store.hydrate([{ id: 'remaining', name: 'Bedroom', host: '192.168.1.3', port: 2121, username: '', base_path: '/', preset: 'ftpsrv' }])
    expect(store.selectedId).toBe('remaining')

    store.hydrate([])
    expect(store.selectedId).toBe('')
  })

  it('selects a remaining profile and clears the final selection after deletes', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: null }) } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: { profiles: [livingRoom] } }) } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: null }) } as Response)
      .mockResolvedValueOnce({ ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data: { profiles: [] } }) } as Response))
    const store = useProfilesStore()
    store.hydrate([livingRoom, bedroom])
    store.selectedId = bedroom.id

    await store.remove(bedroom.id)
    expect(store.selectedId).toBe(livingRoom.id)

    await store.remove(livingRoom.id)
    expect(store.selectedId).toBe('')
  })
})

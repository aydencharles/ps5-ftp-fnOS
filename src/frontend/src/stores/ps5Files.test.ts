import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, BusinessCode } from '../api'
import { usePS5FilesStore } from './ps5Files'

function jsonResponse(data: unknown) {
  return { ok: true, status: 200, json: async () => ({ code: 0, message: 'success', data }) } as Response
}

describe('ps5 files store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => vi.unstubAllGlobals())

  it('tracks idle, connecting, connected, and disconnected states', async () => {
    const store = usePS5FilesStore()
    expect(store.connection).toBe('idle')
    expect(store.connected).toBe(false)

    store.prepare('living-room', '/data')
    expect(store.connection).toBe('connecting')
    expect(store.path).toBe('/data')
    expect(store.entries).toEqual([])

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({
      path: '/data',
      entries: [{ name: 'Games', path: '/data/Games', is_dir: true, size: 0 }],
    })))
    await expect(store.load()).resolves.toBe(true)
    expect(store.connection).toBe('connected')
    expect(store.connected).toBe(true)
    expect(store.entries).toHaveLength(1)

    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new ApiError(400, BusinessCode.PS5OperationFailed, '无法连接到 PS5', null)))
    await expect(store.load()).rejects.toMatchObject({ message: '无法连接到 PS5' })
    expect(store.connection).toBe('disconnected')
    expect(store.connected).toBe(false)
    expect(store.entries).toEqual([])
    expect(store.lastError).toBe('无法连接到 PS5')
    expect(store.path).toBe('/data')

    store.prepare('')
    expect(store.connection).toBe('idle')
    expect(store.profileId).toBe('')
    expect(store.lastError).toBe('')
  })

  it('ignores stale failures after a newer listing has started', async () => {
    const store = usePS5FilesStore()
    store.prepare('living-room', '/')
    let releaseFirst: ((value: Response) => void) | undefined
    const first = new Promise<Response>((resolve) => { releaseFirst = resolve })
    vi.stubGlobal('fetch', vi.fn()
      .mockReturnValueOnce(first)
      .mockResolvedValueOnce(jsonResponse({ path: '/', entries: [{ name: 'Games', path: '/Games', is_dir: true, size: 0 }] })))

    const pending = store.load()
    await store.load()
    releaseFirst?.(jsonResponse({ path: '/', entries: [] }) as Response)
    await expect(pending).resolves.toBe(false)
    expect(store.connection).toBe('connected')
    expect(store.entries).toHaveLength(1)
  })
})

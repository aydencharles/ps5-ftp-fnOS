import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useLibraryStore } from './library'

describe('library store', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => vi.unstubAllGlobals())

  it('only exposes storage volumes in the source selector', () => {
    const store = useLibraryStore()
    store.hydrate([
      { id: 'vol2', label: '存储空间 2', favorite: false, kind: 'volume' },
      { id: 'share', label: '存储空间 2 / PS5 游戏', favorite: false, kind: 'share' },
    ])
    expect(store.storageRoots.map((root) => root.id)).toEqual(['vol2'])
    expect(store.rootId).toBe('vol2')
  })

  it('keeps the newest directory result when a previous request finishes late', async () => {
    const store = useLibraryStore()
    store.hydrate([{ id: 'vol2', label: '存储空间 2', favorite: false, kind: 'volume' }])
    let finishFirst: () => void = () => {}
    const firstResponse = new Promise<Response>((resolve) => {
      finishFirst = () => resolve({ ok: true, json: async () => ({ entries: [{ name: '过期目录', path: 'old', is_dir: true, size: 0 }] }) } as Response)
    })
    vi.stubGlobal('fetch', vi.fn()
      .mockReturnValueOnce(firstResponse)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ entries: [{ name: '当前目录', path: 'new', is_dir: true, size: 0 }] }) } as Response))

    const initial = store.load()
    store.path = 'newer'
    await store.load()
    finishFirst()
    await initial

    expect(store.entries.map((entry) => entry.name)).toEqual(['当前目录'])
    expect(store.loading).toBe(false)
  })
})

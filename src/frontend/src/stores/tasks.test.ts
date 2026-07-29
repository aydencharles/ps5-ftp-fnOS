import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useTasksStore } from './tasks'
import { useTaskCenterStore } from './taskCenter'

describe('tasks store', () => {
  beforeEach(() => { setActivePinia(createPinia()); vi.stubGlobal('fetch', vi.fn()) })
  it('hydrates authoritative snapshots', () => {
    const store = useTasksStore(); store.hydrate([{ id: '1', state: 'queued' } as never])
    expect(store.items[0]?.state).toBe('queued')
  })

  it('opens the task center on the requested tab', () => {
    const store = useTaskCenterStore()
    store.open('extraction', 'history')
    expect(store.visible).toBe(true)
    expect(store.category).toBe('extraction')
    expect(store.statusTab).toBe('history')
    store.close()
    expect(store.visible).toBe(false)
  })

  it('keeps a normalized launcher position in Pinia', () => {
    const store = useTaskCenterStore()
    store.setLauncherPosition({ edge: 'left', ratio: 1.5 })
    expect(store.launcherPosition).toEqual({ edge: 'left', ratio: 1 })
  })

  it('deletes a history record and refreshes the authoritative snapshot', async () => {
    const fetch = vi.mocked(globalThis.fetch)
    fetch
      .mockResolvedValueOnce({ ok: true, json: async () => ({ ok: true }) } as Response)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ tasks: [] }) } as Response)
    const store = useTasksStore()
    store.hydrate([{ id: 'history-1', state: 'succeeded' } as never])

    await store.remove('history-1')

    expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/tasks/history-1', expect.objectContaining({ method: 'DELETE' }))
    expect(store.items).toEqual([])
  })
})

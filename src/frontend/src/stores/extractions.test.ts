import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ExtractionTask } from '../types'
import { useExtractionTasksStore } from './extractions'
import { useTaskCenterStore } from './taskCenter'
import { useTasksStore } from './tasks'

function extraction(id: string, state = 'queued'): ExtractionTask {
  return {
    id,
    source: { root_id: 'root', path: 'Game.7z.001' },
    destination_parent: { root_id: 'root', path: 'downloads' },
    destination: { root_id: 'root', path: 'downloads/Game' },
    delete_sources: false,
    state,
    total_bytes: 0,
    extracted_bytes: 0,
    speed_bytes: 0,
    eta_seconds: null,
    total_items: 0,
    completed_items: 0,
    created_at: '2026-07-29T00:00:00Z',
  }
}

function jsonResponse(data: unknown) {
  return { ok: true, status: 200, json: async () => data } as Response
}

class FakeEventSource {
  static latest: FakeEventSource | null = null
  readonly listeners = new Map<string, (event: MessageEvent) => void>()
  onerror: (() => void) | null = null
  close = vi.fn()

  constructor(readonly url: string) { FakeEventSource.latest = this }
  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    this.listeners.set(type, listener as (event: MessageEvent) => void)
  }
  emit(type: string, data: unknown) {
    this.listeners.get(type)?.({ data: JSON.stringify(data) } as MessageEvent)
  }
}

describe('extraction tasks store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    FakeEventSource.latest = null
  })

  afterEach(() => vi.unstubAllGlobals())

  it('creates, cancels, retries, and removes extraction tasks through independent endpoints', async () => {
    let snapshot: ExtractionTask[] = []
    const requests: Array<{ url: string; method: string; body?: unknown }> = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) : undefined
      requests.push({ url, method, body })
      if (url === '/api/v1/extraction-tasks' && method === 'POST') {
        snapshot = [extraction('created')]
        return jsonResponse({ task: snapshot[0] })
      }
      if (url.endsWith('/cancel')) snapshot = [extraction('created', 'canceling')]
      if (url.endsWith('/retry')) snapshot = [extraction('retried')]
      if (method === 'DELETE') snapshot = []
      if (url === '/api/v1/extraction-tasks' && method === 'GET') return jsonResponse({ tasks: snapshot })
      return jsonResponse({ ok: true, task: snapshot[0] })
    }))
    const store = useExtractionTasksStore()

    await store.create({ root_id: 'root', path: 'Game.7z.001' }, { root_id: 'root', path: 'downloads' }, 'secret', true)
    expect(requests[0]).toEqual({
      url: '/api/v1/extraction-tasks',
      method: 'POST',
      body: {
        source: { root_id: 'root', path: 'Game.7z.001' },
        destination_parent: { root_id: 'root', path: 'downloads' },
        password: 'secret',
        delete_sources: true,
      },
    })
    expect(useTaskCenterStore().category).toBe('extraction')
    expect(store.items[0]?.id).toBe('created')

    await store.cancel('created')
    await store.retry('created', 'new-secret')
    await store.remove('retried')

    expect(requests).toEqual(expect.arrayContaining([
      expect.objectContaining({ url: '/api/v1/extraction-tasks/created/cancel', method: 'POST' }),
      expect.objectContaining({ url: '/api/v1/extraction-tasks/created/retry', method: 'POST', body: { password: 'new-secret' } }),
      expect.objectContaining({ url: '/api/v1/extraction-tasks/retried', method: 'DELETE' }),
    ]))
    expect(store.items).toEqual([])
  })

  it('consumes only the extraction SSE stream', () => {
    vi.stubGlobal('EventSource', FakeEventSource)
    const transfers = useTasksStore()
    transfers.hydrate([{ id: 'transfer', state: 'queued' } as never])
    const store = useExtractionTasksStore()

    store.connect()
    expect(FakeEventSource.latest?.url).toBe('/api/v1/extraction-events')
    FakeEventSource.latest?.emit('extraction-tasks', { tasks: [extraction('archive', 'extracting')] })

    expect(store.items.map((task) => task.id)).toEqual(['archive'])
    expect(transfers.items.map((task) => task.id)).toEqual(['transfer'])
  })
})

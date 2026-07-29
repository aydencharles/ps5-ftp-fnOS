import { defineStore } from 'pinia'
import { api } from '../api'
import type { ExtractionTask, SourceLocator } from '../types'
import { useTaskCenterStore } from './taskCenter'

export const useExtractionTasksStore = defineStore('extraction-tasks', {
  state: () => ({ items: [] as ExtractionTask[], stream: null as EventSource | null }),
  actions: {
    hydrate(items: ExtractionTask[] | null) { this.items = items || [] },
    async refresh() {
      const data = await api<{ tasks: ExtractionTask[] }>('/api/v1/extraction-tasks')
      this.items = data.tasks
    },
    connect() {
      this.stream?.close()
      this.stream = new EventSource('/api/v1/extraction-events')
      this.stream.addEventListener('extraction-tasks', (event) => { this.items = JSON.parse((event as MessageEvent).data).tasks })
      this.stream.onerror = () => {
        this.stream?.close()
        globalThis.setTimeout(() => { void this.refresh().finally(() => this.connect()) }, 2000)
      }
    },
    async create(source: SourceLocator, destinationParent: SourceLocator, password: string, deleteSources: boolean) {
      const data = await api<{ task: ExtractionTask }>('/api/v1/extraction-tasks', {
        method: 'POST',
        body: JSON.stringify({ source, destination_parent: destinationParent, password, delete_sources: deleteSources }),
      })
      await this.refresh()
      useTaskCenterStore().open('extraction', 'active')
      return data.task
    },
    async detail(id: string) { return api<{ task: ExtractionTask }>(`/api/v1/extraction-tasks/${id}`) },
    async cancel(id: string) { await api(`/api/v1/extraction-tasks/${id}/cancel`, { method: 'POST' }); await this.refresh() },
    async retry(id: string, password = '') { await api(`/api/v1/extraction-tasks/${id}/retry`, { method: 'POST', body: JSON.stringify({ password }) }); await this.refresh(); useTaskCenterStore().open('extraction', 'active') },
    async remove(id: string) { await api(`/api/v1/extraction-tasks/${id}`, { method: 'DELETE' }); await this.refresh() },
  },
})

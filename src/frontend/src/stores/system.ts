import { defineStore } from 'pinia'
import { api } from '../api'
import type { ExtractionTask, LibraryRoot, Profile, Task } from '../types'

export const useSystemStore = defineStore('system', {
  state: () => ({ ready: false, error: '', version: '', transferWorkers: 2 }),
  actions: {
    async bootstrap() {
      const data = await api<{ version: string; profiles: Profile[]; library_roots: LibraryRoot[]; tasks: Task[]; extraction_tasks: ExtractionTask[]; settings: { transfer_workers: number } }>('/api/v1/bootstrap')
      this.version = data.version; this.transferWorkers = data.settings.transfer_workers; this.ready = true
      return data
    },
    async saveWorkers(value: number) {
      await api('/api/v1/settings', { method: 'PUT', body: JSON.stringify({ transfer_workers: value }) })
      this.transferWorkers = value
    },
  },
})

import { defineStore } from 'pinia'
import { api } from '../api'
import type { Entry } from '../types'

export const usePS5FilesStore = defineStore('ps5Files', {
  state: () => ({ profileId: '', path: '/', entries: [] as Entry[], query: '', loading: false, error: '', requestId: 0 }),
  actions: {
    async load() {
      if (!this.profileId) { this.entries = []; this.error = ''; return }
      const requestId = ++this.requestId
      const profileId = this.profileId
      const path = this.path
      const query = this.query
      this.loading = true
      this.error = ''
      try {
        const q = new URLSearchParams({ path, query })
        const data = await api<{ entries: Entry[]; path: string }>(`/api/v1/ps5/${profileId}/entries?${q}`)
        if (requestId === this.requestId) {
          this.entries = data.entries
          this.path = data.path
        }
      } catch (error) {
        if (requestId === this.requestId) {
          this.entries = []
          this.error = error instanceof Error ? error.message : String(error)
        }
      } finally { if (requestId === this.requestId) this.loading = false }
    },
    async directories(path: string) {
      const q = new URLSearchParams({ path })
      const data = await api<{ entries: Entry[]; path: string }>(`/api/v1/ps5/${this.profileId}/entries?${q}`)
      return { path: data.path, entries: data.entries.filter((entry) => entry.is_dir) }
    },
    async operation(body: Record<string, unknown>, refresh = true) { const result = await api<{ task?: { id: string } }>(`/api/v1/ps5/${this.profileId}/operations`, { method: 'POST', body: JSON.stringify(body) }); if (refresh) await this.load(); return result },
  },
})

import { defineStore } from 'pinia'
import { api } from '../api'
import type { Entry, PS5ConnectionStatus } from '../types'

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export const usePS5FilesStore = defineStore('ps5Files', {
  state: () => ({
    profileId: '',
    path: '/',
    entries: [] as Entry[],
    query: '',
    loading: false,
    requestId: 0,
    connection: 'idle' as PS5ConnectionStatus,
    lastError: '',
  }),
  getters: {
    connected: (state) => state.connection === 'connected',
  },
  actions: {
    prepare(id: string, path = '/') {
      this.profileId = id
      this.query = ''
      this.entries = []
      this.lastError = ''
      if (!id) {
        this.path = '/'
        this.connection = 'idle'
        return
      }
      this.path = path
      this.connection = 'connecting'
    },
    async load() {
      if (!this.profileId) {
        this.entries = []
        this.connection = 'idle'
        this.lastError = ''
        return false
      }
      const requestId = ++this.requestId
      const profileId = this.profileId
      const path = this.path
      const query = this.query
      this.loading = true
      this.connection = 'connecting'
      this.lastError = ''
      try {
        const q = new URLSearchParams({ path, query })
        const data = await api<{ entries: Entry[]; path: string }>(`/api/v1/ps5/${profileId}/entries?${q}`)
        if (requestId === this.requestId) {
          this.entries = data.entries
          this.path = data.path
          this.connection = 'connected'
          this.lastError = ''
        }
        return requestId === this.requestId
      } catch (error) {
        if (requestId === this.requestId) {
          this.entries = []
          this.connection = 'disconnected'
          this.lastError = errorMessage(error)
          throw error
        }
        return false
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

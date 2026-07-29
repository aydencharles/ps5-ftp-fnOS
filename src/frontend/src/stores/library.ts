import { defineStore } from 'pinia'
import { api } from '../api'
import type { Entry, LibraryRoot } from '../types'

export const useLibraryStore = defineStore('library', {
  state: () => ({ roots: [] as LibraryRoot[], rootId: '', path: '', entries: [] as Entry[], query: '', hidden: false, loading: false, requestId: 0 }),
  getters: {
    storageRoots: (state) => state.roots.filter((root) => root.kind === 'volume'),
  },
  actions: {
    hydrate(roots: LibraryRoot[] | null) {
      this.roots = roots || []
      const storageRoots = this.roots.filter((root) => root.kind === 'volume')
      if (!storageRoots.some((root) => root.id === this.rootId)) this.rootId = storageRoots[0]?.id || ''
    },
    async load() {
      if (!this.rootId) { this.entries = []; return false }
      const requestId = ++this.requestId
      const rootId = this.rootId
      const path = this.path
      const query = this.query
      const hidden = this.hidden
      this.loading = true
      try {
        const q = new URLSearchParams({ root_id: rootId, path, query, hidden: hidden ? '1' : '0' })
        const data = await api<{ entries: Entry[] }>(`/api/v1/library/entries?${q}`)
        if (requestId === this.requestId) this.entries = data.entries
        return requestId === this.requestId
      } catch (error) {
        if (requestId === this.requestId) {
          this.entries = []
          throw error
        }
        return false
      } finally {
        if (requestId === this.requestId) this.loading = false
      }
    },
  },
})

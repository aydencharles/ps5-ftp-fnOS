import { defineStore } from 'pinia'
import { api } from '../api'
import type { Entry, LibraryRoot, SourceLocator } from '../types'

export const useLibraryStore = defineStore('library', {
  state: () => ({ roots: [] as LibraryRoot[], rootId: '', path: '', entries: [] as Entry[], selected: [] as SourceLocator[], query: '', hidden: false, loading: false, error: '', requestId: 0 }),
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
      if (!this.rootId) { this.entries = []; this.error = ''; return }
      const requestId = ++this.requestId
      const rootId = this.rootId
      const path = this.path
      const query = this.query
      const hidden = this.hidden
      this.loading = true
      this.error = ''
      try {
        const q = new URLSearchParams({ root_id: rootId, path, query, hidden: hidden ? '1' : '0' })
        const data = await api<{ entries: Entry[] }>(`/api/v1/library/entries?${q}`)
        if (requestId === this.requestId) this.entries = data.entries
      } catch (error) {
        if (requestId === this.requestId) {
          this.entries = []
          this.error = error instanceof Error ? error.message : String(error)
        }
      } finally {
        if (requestId === this.requestId) this.loading = false
      }
    },
    toggle(entry: Entry) {
      const locator = { root_id: this.rootId, path: entry.path.replace(/^\/+|\/+$/g, '') }
      const index = this.selected.findIndex((v) => v.root_id === locator.root_id && v.path === locator.path)
      if (index >= 0) {
        this.selected.splice(index, 1)
        return
      }
      const hasAncestor = this.selected.some((value) => value.root_id === locator.root_id && locator.path.startsWith(`${value.path}/`))
      if (hasAncestor) return
      this.selected = this.selected.filter((value) => value.root_id !== locator.root_id || !value.path.startsWith(`${locator.path}/`))
      this.selected.push(locator)
    },
    isSelected(entry: Entry) { return this.selected.some((v) => v.root_id === this.rootId && v.path === entry.path) },
  },
})

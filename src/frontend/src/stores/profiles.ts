import { defineStore } from 'pinia'
import { api } from '../api'
import type { ConnectionTestResult, Profile } from '../types'

export const useProfilesStore = defineStore('profiles', {
  state: () => ({ items: [] as Profile[], selectedId: '' }),
  getters: { selected: (state) => state.items.find((p) => p.id === state.selectedId) },
  actions: {
    hydrate(items: Profile[] | null) { this.items = items || []; if (!this.selectedId && this.items[0]) this.selectedId = this.items[0].id },
    async refresh() { const data = await api<{ profiles: Profile[] }>('/api/v1/profiles'); this.hydrate(data.profiles) },
    async save(profile: Partial<Profile>) {
      const method = profile.id ? 'PUT' : 'POST'; const suffix = profile.id ? `/${profile.id}` : ''
      await api(`/api/v1/profiles${suffix}`, { method, body: JSON.stringify(profile) }); await this.refresh()
    },
    async remove(id: string) { await api(`/api/v1/profiles/${id}`, { method: 'DELETE' }); await this.refresh() },
    async test(id: string, signal?: AbortSignal) { return api<ConnectionTestResult>(`/api/v1/profiles/${id}/test`, { method: 'POST', signal }) },
  },
})

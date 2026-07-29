import { defineStore } from 'pinia'
import { api } from '../api'
import type { SourceLocator, Task, TaskEvent, TaskItem } from '../types'

export type TaskLauncherEdge = 'top' | 'right' | 'bottom' | 'left'
export interface TaskLauncherPosition { edge: TaskLauncherEdge; ratio: number }

const launcherStorageKey = 'ps5-ftp-manager:task-launcher-position'
const launcherEdges = new Set<TaskLauncherEdge>(['top', 'right', 'bottom', 'left'])

export const useTasksStore = defineStore('tasks', {
  state: () => ({
    items: [] as Task[],
    stream: null as EventSource | null,
    centerVisible: false,
    centerTab: 'active' as 'active' | 'history',
    launcherPosition: { edge: 'right', ratio: 1 } as TaskLauncherPosition,
  }),
  actions: {
    hydrate(items: Task[] | null) { this.items = items || [] },
    openCenter(tab: 'active' | 'history' = 'active') { this.centerTab = tab; this.centerVisible = true },
    closeCenter() { this.centerVisible = false },
    setLauncherPosition(position: TaskLauncherPosition) {
      this.launcherPosition = {
        edge: launcherEdges.has(position.edge) ? position.edge : 'right',
        ratio: Math.max(0, Math.min(1, Number.isFinite(position.ratio) ? position.ratio : 1)),
      }
      try { globalThis.localStorage?.setItem(launcherStorageKey, JSON.stringify(this.launcherPosition)) } catch { /* Storage can be unavailable in embedded/private contexts. */ }
    },
    hydrateLauncherPosition() {
      try {
        const raw = globalThis.localStorage?.getItem(launcherStorageKey)
        if (!raw) return
        const position = JSON.parse(raw) as Partial<TaskLauncherPosition>
        if (!position.edge || !launcherEdges.has(position.edge) || typeof position.ratio !== 'number') return
        this.launcherPosition = { edge: position.edge, ratio: Math.max(0, Math.min(1, position.ratio)) }
      } catch { /* Ignore malformed or inaccessible storage and keep the safe default. */ }
    },
    async refresh() { const data = await api<{ tasks: Task[] }>('/api/v1/tasks'); this.items = data.tasks },
    connect() { this.stream?.close(); this.stream = new EventSource('/api/v1/events'); this.stream.addEventListener('tasks', (event) => { this.items = JSON.parse((event as MessageEvent).data).tasks }); this.stream.onerror = () => { this.stream?.close(); setTimeout(() => { void this.refresh().finally(() => this.connect()) }, 2000) } },
    async create(profileId: string, sources: SourceLocator[], destination: string, conflict: string) { const data = await api<{ task: Task }>('/api/v1/tasks', { method: 'POST', body: JSON.stringify({ profile_id: profileId, sources, destination, conflict_policy: conflict, type: 'upload' }) }); await this.refresh(); this.openCenter(); return data.task },
    async createDownload(profileId: string, remotePaths: string[], destination: SourceLocator, conflict: string) {
      const data = await api<{ task: Task }>('/api/v1/tasks', {
        method: 'POST',
        body: JSON.stringify({
          profile_id: profileId,
          sources: remotePaths.map((path) => ({ root_id: destination.root_id, path })),
          destination: destination.path,
          conflict_policy: conflict,
          type: 'download',
        }),
      })
      await this.refresh()
      this.openCenter()
      return data.task
    },
    async detail(id: string) { return api<{ task: Task; items: TaskItem[]; events: TaskEvent[] }>(`/api/v1/tasks/${id}`) },
    async cancel(id: string) { await api(`/api/v1/tasks/${id}/cancel`, { method: 'POST' }); await this.refresh() },
    async retry(id: string) { await api(`/api/v1/tasks/${id}/retry`, { method: 'POST' }); await this.refresh() },
    async remove(id: string) { await api(`/api/v1/tasks/${id}`, { method: 'DELETE' }); await this.refresh() },
  },
})

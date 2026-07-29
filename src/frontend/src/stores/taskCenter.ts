import { defineStore } from 'pinia'

export type TaskCategory = 'transfer' | 'extraction'
export type TaskStatusTab = 'active' | 'history'
export type TaskLauncherEdge = 'top' | 'right' | 'bottom' | 'left'
export interface TaskLauncherPosition { edge: TaskLauncherEdge; ratio: number }

const launcherStorageKey = 'ps5-ftp-manager:task-launcher-position'
const launcherEdges = new Set<TaskLauncherEdge>(['top', 'right', 'bottom', 'left'])

export const useTaskCenterStore = defineStore('task-center-ui', {
  state: () => ({
    visible: false,
    category: 'transfer' as TaskCategory,
    statusTab: 'active' as TaskStatusTab,
    launcherPosition: { edge: 'right', ratio: 1 } as TaskLauncherPosition,
  }),
  actions: {
    open(category: TaskCategory = 'transfer', statusTab: TaskStatusTab = 'active') {
      this.category = category
      this.statusTab = statusTab
      this.visible = true
    },
    close() { this.visible = false },
    setLauncherPosition(position: TaskLauncherPosition) {
      this.launcherPosition = {
        edge: launcherEdges.has(position.edge) ? position.edge : 'right',
        ratio: Math.max(0, Math.min(1, Number.isFinite(position.ratio) ? position.ratio : 1)),
      }
      try { globalThis.localStorage?.setItem(launcherStorageKey, JSON.stringify(this.launcherPosition)) } catch { /* Storage can be unavailable. */ }
    },
    hydrateLauncherPosition() {
      try {
        const raw = globalThis.localStorage?.getItem(launcherStorageKey)
        if (!raw) return
        const position = JSON.parse(raw) as Partial<TaskLauncherPosition>
        if (!position.edge || !launcherEdges.has(position.edge) || typeof position.ratio !== 'number') return
        this.launcherPosition = { edge: position.edge, ratio: Math.max(0, Math.min(1, position.ratio)) }
      } catch { /* Keep the safe default. */ }
    },
  },
})

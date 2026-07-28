export interface Profile {
  id: string
  name: string
  host: string
  port: number
  username: string
  password?: string
  base_path: string
  preset: 'zftpd' | 'ftpsrv' | 'custom'
}

export interface LibraryRoot { id: string; label: string; favorite: boolean; kind: 'volume' | 'share' | string }
export interface Entry {
  name: string
  path: string
  is_dir: boolean
  size: number
  modified_at?: string
  game_kind?: 'game-directory' | 'game-image'
}
export interface SourceLocator { root_id: string; path: string }
export interface Task {
  id: string
  type: string
  profile_id: string
  sources: SourceLocator[]
  destination: string
  conflict_policy: 'smart' | 'overwrite' | 'fail'
  state: string
  total_bytes: number
  transferred_bytes: number
  speed_bytes: number
  eta_seconds: number | null
  current_file?: string
  total_items: number
  completed_items: number
  skipped_items: number
  error?: string
  retry_of?: string
  created_at: string
  started_at?: string
  finished_at?: string
}

export interface TaskItem {
  id: string
  task_id: string
  source_path: string
  destination: string
  is_dir: boolean
  size: number
  state: string
  transferred: number
  attempts: number
  error?: string
}

export interface TaskEvent {
  id: number
  task_id: string
  level: string
  message: string
  created_at: string
}

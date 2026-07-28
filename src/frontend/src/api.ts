export class ApiError extends Error { constructor(public status: number, message: string) { super(message) } }

export async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init.headers } : init?.headers,
  })
  const data = await response.json().catch(() => ({})) as T & { error?: string }
  if (!response.ok) throw new ApiError(response.status, data.error || `HTTP ${response.status}`)
  return data
}

export const parentPath = (value: string) => {
  const parts = value.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}
export const remoteParent = (value: string) => `/${parentPath(value)}`.replace(/\/$/, '') || '/'
export const joinPath = (base: string, name: string) => [base.replace(/\/$/, ''), name].filter(Boolean).join('/').replace(/^([^/])/, '/$1')
export const formatBytes = (value = 0) => {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']; const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
export const formatETA = (seconds: number | null | undefined) => {
  if (seconds == null || seconds < 0) return '计算中'
  if (seconds < 60) return `${seconds} 秒`
  if (seconds < 3600) return `${Math.ceil(seconds / 60)} 分钟`
  return `${Math.floor(seconds / 3600)} 小时 ${Math.ceil((seconds % 3600) / 60)} 分钟`
}

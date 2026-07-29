export enum BusinessCode {
  Success = 0,
  InvalidRequest = 1000,
  ProfileNotFound = 1001,
  ResourceNotFound = 1002,
  ResourceConflict = 1003,
  PS5OperationFailed = 2001,
  InternalError = 9000,
}

export interface ApiEnvelope<T> {
  code: number
  message: string
  data: T
}

export class ApiError<T = unknown> extends Error {
  handled = false

  constructor(
    public status: number,
    public code: number,
    message: string,
    public data: T | null,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export type ApiErrorInterceptor = (error: ApiError) => boolean | void | Promise<boolean | void>

let apiErrorInterceptor: ApiErrorInterceptor | null = null

export function setApiErrorInterceptor(interceptor: ApiErrorInterceptor | null) {
  apiErrorInterceptor = interceptor
}

function isEnvelope(value: unknown): value is ApiEnvelope<unknown> {
  if (!value || typeof value !== 'object') return false
  const envelope = value as Partial<ApiEnvelope<unknown>>
  return typeof envelope.code === 'number' && typeof envelope.message === 'string' && 'data' in envelope
}

async function throwApiError(error: ApiError): Promise<never> {
  if (apiErrorInterceptor) {
    try {
      error.handled = await apiErrorInterceptor(error) === true
    } catch {
      error.handled = false
    }
  }
  throw error
}

export async function api<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: init?.body ? { 'Content-Type': 'application/json', ...init.headers } : init?.headers,
  })
  const value = await response.json().catch(() => null) as unknown
  if (!isEnvelope(value)) {
    return throwApiError(new ApiError(response.status, BusinessCode.InternalError, '服务响应格式错误', null))
  }
  if (!response.ok || value.code !== BusinessCode.Success) {
    return throwApiError(new ApiError(response.status, value.code || BusinessCode.InternalError, value.message || `HTTP ${response.status}`, value.data))
  }
  return value.data as T
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

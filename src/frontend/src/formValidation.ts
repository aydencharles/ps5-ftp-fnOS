import type { FormInstanceFunctions, FormRule, FormRules } from 'tdesign-vue-next'
import { ApiError } from './api'
import type { Profile } from './types'

export const optionalPasswordRules: FormRule[] = [
  { max: 1024, message: '密码不能超过 1024 个字符', trigger: 'blur' },
]

export const remoteEntryNameRules: FormRule[] = [
  { required: true, whitespace: true, message: '请输入名称', trigger: 'blur' },
  { max: 255, message: '名称不能超过 255 个字符', trigger: 'blur' },
  {
    validator: (value) => typeof value === 'string' && value !== '.' && value !== '..' && !/[\\/\0]/.test(value),
    message: '名称不能是 . 或 ..，且不能包含 / 或 \\',
    trigger: 'blur',
  },
]

export function unwrapFTPHost(value: string) {
  const host = value.trim()
  return host.startsWith('[') && host.endsWith(']') ? host.slice(1, -1) : host
}

export function isValidFTPHost(value: unknown): boolean {
  if (typeof value !== 'string' || value === '') return false
  if (/[/\\[\]]/.test(value)) return false
  for (const char of value) {
    if (char === '\0' || char.charCodeAt(0) < 32 || /\s/.test(char)) return false
  }
  if (!value.includes(':')) return !value.includes('%')
  const address = value.includes('%') ? value.slice(0, value.indexOf('%')) : value
  try {
    return Boolean(new URL(`http://[${address}]`).hostname)
  } catch {
    return false
  }
}

export function isValidProfilePort(value: unknown): boolean {
  const port = typeof value === 'string' && value.trim() !== '' ? Number(value) : value
  return Number.isInteger(port) && Number(port) >= 1 && Number(port) <= 65535
}

export function isValidBasePath(value: unknown): boolean {
  if (typeof value !== 'string') return false
  const path = value.replaceAll('\\', '/').trim()
  if (!path || path.includes('\0')) return false
  return !path.split('/').includes('..')
}

export function normalizeProfileForm(form: Partial<Profile>): Partial<Profile> {
  return {
    ...form,
    name: form.name?.trim() ?? '',
    host: unwrapFTPHost(form.host ?? ''),
    username: form.username?.trim() ?? '',
    password: form.password ?? '',
    base_path: (form.base_path ?? '').trim() || '/',
    preset: form.preset,
    port: form.port,
  }
}

export const profileFormRules: FormRules<Partial<Profile>> = {
  name: [
    { required: true, whitespace: true, message: '请输入配置名称', trigger: 'blur' },
    { max: 100, message: '名称不能超过 100 个字符', trigger: 'blur' },
  ],
  preset: [{ required: true, enum: ['zftpd', 'ftpsrv', 'custom'], message: '请选择服务器预设', trigger: 'change' }],
  host: [
    { required: true, whitespace: true, message: '请输入 IP 或主机名', trigger: 'blur' },
    { max: 255, message: '主机名不能超过 255 个字符', trigger: 'blur' },
    {
      validator: (value) => isValidFTPHost(typeof value === 'string' ? unwrapFTPHost(value) : value),
      message: '请输入不带协议和端口的 IP 或主机名',
      trigger: 'blur',
    },
  ],
  port: [
    { required: true, message: '请输入端口', trigger: 'change' },
    { validator: isValidProfilePort, message: '端口必须是 1 到 65535 之间的整数', trigger: 'change' },
  ],
  username: [{ max: 255, message: '用户名不能超过 255 个字符', trigger: 'blur' }],
  password: [{ max: 1024, message: '密码不能超过 1024 个字符', trigger: 'blur' }],
  base_path: [
    { required: true, whitespace: true, message: '请输入基础目录', trigger: 'blur' },
    { max: 4096, message: '基础目录不能超过 4096 个字符', trigger: 'blur' },
    { validator: isValidBasePath, message: '基础目录不能包含 .. 路径段', trigger: 'blur' },
  ],
}

export function applyProfileFieldErrors(form: FormInstanceFunctions | undefined, error: unknown) {
  if (!form || !(error instanceof ApiError) || !error.data || typeof error.data !== 'object') return
  const fields = (error.data as { fields?: Record<string, string> }).fields
  if (!fields) return
  form.setValidateMessage(Object.fromEntries(
    Object.entries(fields).map(([name, message]) => [name, [{ type: 'error', message }]]),
  ))
}

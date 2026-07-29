import { MessagePlugin } from 'tdesign-vue-next'
import { ApiError } from './api'

export async function notifyError(error: unknown) {
  if (error instanceof ApiError && error.handled) return
  await MessagePlugin.error(error instanceof Error ? error.message : String(error))
}

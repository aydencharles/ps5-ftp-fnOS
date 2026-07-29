import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MessagePlugin } from 'tdesign-vue-next'
import { ApiError, BusinessCode } from './api'
import { notifyError } from './errorFeedback'

vi.mock('tdesign-vue-next', () => ({ MessagePlugin: { error: vi.fn() } }))

describe('error feedback', () => {
  beforeEach(() => vi.mocked(MessagePlugin.error).mockClear())

  it('suppresses errors already handled by the business interceptor', async () => {
    const handled = new ApiError(400, BusinessCode.ProfileNotFound, 'profile missing', null)
    handled.handled = true

    await notifyError(handled)
    await notifyError(new Error('request failed'))

    expect(MessagePlugin.error).toHaveBeenCalledOnce()
    expect(MessagePlugin.error).toHaveBeenCalledWith('request failed')
  })
})

import { describe, expect, it, vi } from 'vitest'
import { ApiError, BusinessCode } from './api'
import { createProfileErrorInterceptor } from './profileRecovery'

describe('profile error recovery', () => {
  it('coalesces missing-profile recovery and redirects protected routes', async () => {
    let finishRefresh: () => void = () => {}
    const profiles = {
      items: [{ id: 'deleted' }],
      selectedId: 'deleted',
      refresh: vi.fn(() => new Promise<void>((resolve) => {
        finishRefresh = () => {
          profiles.items = []
          resolve()
        }
      })),
    }
    const router = {
      currentRoute: { value: { path: '/files', meta: { requiresProfile: true } } },
      replace: vi.fn().mockResolvedValue(undefined),
    }
    const notify = vi.fn().mockResolvedValue(undefined)
    const intercept = createProfileErrorInterceptor(profiles, router, notify)
    const error = new ApiError(400, BusinessCode.ProfileNotFound, 'PS5 配置不存在或已被删除，请重新配置', null)

    const first = intercept(error)
    const second = intercept(error)
    expect(profiles.selectedId).toBe('')
    expect(profiles.refresh).toHaveBeenCalledOnce()

    finishRefresh()
    await expect(Promise.all([first, second])).resolves.toEqual([true, true])
    expect(notify).toHaveBeenCalledOnce()
    expect(router.replace).toHaveBeenCalledOnce()
    expect(router.replace).toHaveBeenCalledWith('/settings')
  })

  it('keeps the fnOS transfer route available when no profiles remain', async () => {
    const profiles = {
      items: [{ id: 'deleted' }],
      selectedId: 'deleted',
      refresh: vi.fn(async () => { profiles.items = [] }),
    }
    const router = {
      currentRoute: { value: { path: '/', meta: {} } },
      replace: vi.fn().mockResolvedValue(undefined),
    }
    const notify = vi.fn().mockResolvedValue(undefined)
    const intercept = createProfileErrorInterceptor(profiles, router, notify)

    await expect(intercept(new ApiError(400, BusinessCode.ProfileNotFound, 'PS5 配置不存在或已被删除，请重新配置', null))).resolves.toBe(true)

    expect(profiles.selectedId).toBe('')
    expect(router.replace).not.toHaveBeenCalled()
  })
})

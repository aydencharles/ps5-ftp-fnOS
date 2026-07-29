import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, BusinessCode, api, setApiErrorInterceptor } from './api'

describe('request module', () => {
  afterEach(() => {
    setApiErrorInterceptor(null)
    vi.unstubAllGlobals()
  })

  it('returns only data from a successful response envelope', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: 0, message: 'success', data: { value: 42 } }),
    } as Response))

    await expect(api<{ value: number }>('/api/v1/example')).resolves.toEqual({ value: 42 })
  })

  it('passes business errors through the configured interceptor before throwing', async () => {
    const interceptor = vi.fn().mockResolvedValue(true)
    setApiErrorInterceptor(interceptor)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: async () => ({ code: BusinessCode.ProfileNotFound, message: 'PS5 配置不存在或已被删除，请重新配置', data: null }),
    } as Response))

    const error = await api('/api/v1/ps5/missing/entries').catch((reason) => reason)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 400, code: BusinessCode.ProfileNotFound, handled: true, data: null })
    expect(interceptor).toHaveBeenCalledOnce()
  })

  it('rejects a non-JSON response without exposing its body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 502 })))

    await expect(api('/api/v1/example')).rejects.toMatchObject({
      status: 502,
      code: BusinessCode.InternalError,
      message: '服务响应格式错误',
      data: null,
    })
  })

  it('rejects a non-zero business code even when the HTTP status is 200', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ code: BusinessCode.ResourceNotFound, message: '请求的资源不存在', data: null }),
    } as Response))

    await expect(api('/api/v1/example')).rejects.toMatchObject({
      status: 200,
      code: BusinessCode.ResourceNotFound,
      message: '请求的资源不存在',
    })
  })
})

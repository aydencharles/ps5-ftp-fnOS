import { describe, expect, it, vi } from 'vitest'
import { ApiError, BusinessCode } from './api'
import {
  applyProfileFieldErrors,
  isValidBasePath,
  isValidFTPHost,
  isValidProfilePort,
  normalizeProfileForm,
  unwrapFTPHost,
} from './formValidation'

describe('profile form validation', () => {
  it('accepts hostnames, IPv4, and IPv6 while rejecting protocol or port suffixes', () => {
    expect(isValidFTPHost('ps5.local')).toBe(true)
    expect(isValidFTPHost('192.168.1.50')).toBe(true)
    expect(isValidFTPHost('::1')).toBe(true)
    expect(isValidFTPHost('fe80::1%eth0')).toBe(true)
    expect(isValidFTPHost('192.168.1.50:2120')).toBe(false)
    expect(isValidFTPHost('ftp://192.168.1.50')).toBe(false)
    expect(isValidFTPHost('::::')).toBe(false)
    expect(isValidFTPHost('host name')).toBe(false)
    expect(isValidFTPHost('ps5.local%zone')).toBe(false)
    expect(isValidFTPHost(unwrapFTPHost('[::1]'))).toBe(true)
  })

  it('requires an integer FTP port in the valid range', () => {
    expect(isValidProfilePort(2120)).toBe(true)
    expect(isValidProfilePort('2121')).toBe(true)
    expect(isValidProfilePort(0)).toBe(false)
    expect(isValidProfilePort(65536)).toBe(false)
    expect(isValidProfilePort(2120.5)).toBe(false)
    expect(isValidProfilePort(null)).toBe(false)
    expect(isValidProfilePort('')).toBe(false)
  })

  it('rejects traversal and null bytes in the base path', () => {
    expect(isValidBasePath('/')).toBe(true)
    expect(isValidBasePath('/data/homebrew')).toBe(true)
    expect(isValidBasePath(' /data/ ')).toBe(true)
    expect(isValidBasePath('/data/../system')).toBe(false)
    expect(isValidBasePath('/data/\0secret')).toBe(false)
    expect(isValidBasePath('   ')).toBe(false)
  })

  it('trims submitted profile fields and unwraps bracketed IPv6 hosts', () => {
    expect(normalizeProfileForm({
      name: ' 客厅 PS5 ',
      host: '[fe80::1]',
      port: 2120,
      username: ' anonymous ',
      password: '',
      base_path: ' /data/ ',
      preset: 'zftpd',
    })).toEqual({
      name: '客厅 PS5',
      host: 'fe80::1',
      port: 2120,
      username: 'anonymous',
      password: '',
      base_path: '/data/',
      preset: 'zftpd',
    })
  })

  it('maps backend field errors onto the form instance', () => {
    const setValidateMessage = vi.fn()
    applyProfileFieldErrors(
      { setValidateMessage } as never,
      new ApiError(400, BusinessCode.InvalidRequest, '请求参数无效', { fields: { host: '参数 host 格式不正确' } }),
    )
    expect(setValidateMessage).toHaveBeenCalledWith({
      host: [{ type: 'error', message: '参数 host 格式不正确' }],
    })
  })
})

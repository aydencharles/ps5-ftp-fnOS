import { createPinia } from 'pinia'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ConnectionTestDialog from './ConnectionTestDialog.vue'
import type { ConnectionTestResult, Profile } from '../types'

const TDialog = defineComponent({
  props: { visible: Boolean, header: { type: String, default: '' }, confirmBtn: { type: Object, default: () => ({}) } },
  emits: ['update:visible', 'confirm'],
  template: '<section v-if="visible" class="test-dialog"><h2>{{ header }}</h2><slot /><button class="dialog-confirm" :disabled="confirmBtn.disabled" @click="$emit(\'confirm\')">{{ confirmBtn.content }}</button></section>',
})

const profile: Profile = {
  id: 'living-room',
  name: '客厅 PS5',
  host: '192.168.1.50',
  port: 2120,
  username: 'anonymous',
  base_path: '/data',
  preset: 'zftpd',
}

function jsonResponse(data: unknown) {
  return { ok: true, status: 200, json: async () => data } as Response
}

function mountDialog(response: ConnectionTestResult) {
  const fetchMock = vi.fn(async () => jsonResponse(response))
  vi.stubGlobal('fetch', fetchMock)
  const wrapper = mount(ConnectionTestDialog, {
    props: { visible: true, profile },
    global: { plugins: [createPinia()], stubs: { TDialog } },
  })
  return { wrapper, fetchMock }
}

afterEach(() => vi.unstubAllGlobals())

describe('ConnectionTestDialog', () => {
  it('shows every verified stage and supports retry after success', async () => {
    const response: ConnectionTestResult = {
      ok: true,
      duration_ms: 128,
      tested_at: '2026-07-29T10:00:00Z',
      checks: [
        { id: 'resolve', status: 'passed', detail: '192.168.1.50', duration_ms: 1 },
        { id: 'connect', status: 'passed', detail: 'FTP 服务已响应', duration_ms: 42 },
        { id: 'authenticate', status: 'passed', detail: '已以 anonymous 登录', duration_ms: 20 },
        { id: 'directory', status: 'passed', detail: '/data 可访问，共 6 项', duration_ms: 65 },
      ],
    }
    const { wrapper, fetchMock } = mountDialog(response)
    await flushPromises()

    expect(wrapper.text()).toContain('客厅 PS5')
    expect(wrapper.text()).toContain('192.168.1.50:2120 · zftpd · /data')
    expect(wrapper.text()).toContain('连接成功')
    expect(wrapper.text()).toContain('解析主机地址')
    expect(wrapper.text()).toContain('/data 可访问，共 6 项')
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/profiles/living-room/test', expect.objectContaining({ method: 'POST' }))

    await wrapper.get('.dialog-confirm').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('shows the precise failure reason, suggestions, and technical detail', async () => {
    const response: ConnectionTestResult = {
      ok: false,
      duration_ms: 3,
      tested_at: '2026-07-29T10:00:00Z',
      checks: [
        { id: 'resolve', status: 'passed', detail: '192.168.1.50', duration_ms: 1 },
        { id: 'connect', status: 'failed', detail: '端口拒绝连接', duration_ms: 2 },
        { id: 'authenticate', status: 'skipped', duration_ms: 0 },
        { id: 'directory', status: 'skipped', duration_ms: 0 },
      ],
      failure: {
        stage: 'connect',
        code: 'connection_refused',
        title: 'FTP 端口拒绝连接',
        message: '已找到目标设备，但 192.168.1.50:2120 没有接受 FTP 连接。',
        detail: 'dial tcp 192.168.1.50:2120: connect: connection refused',
        suggestions: ['在 PS5 上启动 zftpd 或 ftpsrv', '核对 FTP 端口'],
      },
    }
    const { wrapper } = mountDialog(response)
    await flushPromises()

    expect(wrapper.text()).toContain('连接检测未通过')
    expect(wrapper.text()).toContain('FTP 端口拒绝连接')
    expect(wrapper.text()).toContain('connection_refused')
    expect(wrapper.text()).toContain('在 PS5 上启动 zftpd 或 ftpsrv')
    expect(wrapper.text()).toContain('因前序检测失败而跳过')
    expect(wrapper.text()).toContain('connection refused')
  })
})

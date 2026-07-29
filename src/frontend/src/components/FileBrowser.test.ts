/* eslint-disable vue/one-component-per-file, vue/require-default-prop */
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MessagePlugin } from 'tdesign-vue-next'
import FileBrowser from './FileBrowser.vue'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import type { SourceLocator } from '../types'

vi.mock('tdesign-vue-next', () => ({
  DialogPlugin: { confirm: vi.fn(() => ({ destroy: vi.fn() })) },
  MessagePlugin: { warning: vi.fn(), success: vi.fn(), error: vi.fn() },
}))

const TButton = defineComponent({
  props: { disabled: Boolean },
  emits: ['click'],
  template: '<button :disabled="disabled" @click="$emit(\'click\', $event)"><slot /></button>',
})
const TInput = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  emits: ['update:modelValue', 'enter', 'clear'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})
const TDialog = defineComponent({
  props: { visible: Boolean, header: String },
  emits: ['update:visible', 'confirm'],
  template: '<section v-if="visible" class="test-dialog"><h2>{{ header }}</h2><slot /><button class="dialog-confirm" @click="$emit(\'confirm\')">确认</button></section>',
})
const passThrough = defineComponent({ template: '<div><slot /></div>' })
const TCheckbox = defineComponent({
  props: { checked: Boolean, indeterminate: Boolean },
  emits: ['click', 'change'],
  template: '<input type="checkbox" :checked="checked" :data-indeterminate="indeterminate" @click="$emit(\'click\', $event); $emit(\'change\', !checked)" />',
})

const stubs = {
  TButton,
  TInput,
  TDialog,
  TForm: passThrough,
  TFormItem: passThrough,
  TSelect: passThrough,
  TCheckbox,
  TAlert: passThrough,
}

function jsonResponse(data: unknown) {
  return { ok: true, status: 200, json: async () => data } as Response
}

describe('File Browser', () => {
  const requests: Array<{ url: string; method: string; body?: Record<string, unknown> }> = []

  beforeEach(() => {
    vi.clearAllMocks()
    requests.length = 0
    const pinia = createPinia()
    setActivePinia(pinia)
    useProfilesStore().hydrate([{
      id: 'living-room',
      name: '客厅 PS5',
      host: '192.168.1.50',
      port: 2120,
      username: 'anonymous',
      base_path: '/',
      preset: 'zftpd',
    }])
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'
      const body = init?.body ? JSON.parse(String(init.body)) as Record<string, unknown> : undefined
      requests.push({ url, method, body })
      if (method === 'POST') return jsonResponse({})
      const path = new URL(url, 'http://localhost').searchParams.get('path') || '/'
      if (url.includes('/api/v1/library/entries')) return jsonResponse({
        path: path === '/' ? '' : path,
        entries: path === '/' ? [
          { name: 'PS5 游戏', path: '1000/PS5 游戏', is_dir: true, size: 0, game_kind: 'game-directory' },
          { name: 'backup.exfat', path: '1000/backup.exfat', is_dir: false, size: 4096, game_kind: 'game-image' },
          { name: 'readme.txt', path: '1000/readme.txt', is_dir: false, size: 128 },
        ] : [{ name: 'eboot.bin', path: '1000/PS5 游戏/eboot.bin', is_dir: false, size: 1024 }],
      })
      return jsonResponse({
        path,
        entries: path === '/data/homebrew' ? [
          { name: 'Games', path: '/data/homebrew/Games', is_dir: true, size: 0, modified_at: '2026-07-28T10:00:00Z' },
          { name: 'eboot.bin', path: '/data/homebrew/eboot.bin', is_dir: false, size: 1024, modified_at: '2026-07-28T10:00:00Z' },
        ] : [],
      })
    }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  function mountDestination() {
    return mount(FileBrowser, {
      props: { mode: 'destination', modelValue: '/data/homebrew', compact: true },
      global: { stubs, mocks: { $router: { push: vi.fn() } } },
    })
  }

  function mountManage() {
    return mount(FileBrowser, {
      global: { stubs, mocks: { $router: { push: vi.fn() } } },
    })
  }

  function mountSource(selectedSources: SourceLocator[] = []) {
    const library = useLibraryStore()
    library.hydrate([{ id: 'vol2', label: '存储空间 2', favorite: false, kind: 'volume' }])
    return mount(FileBrowser, {
      props: { mode: 'source', compact: true, selectedSources },
      global: { stubs, mocks: { $router: { push: vi.fn() } } },
    })
  }

  function latestSources(wrapper: ReturnType<typeof mount>): SourceLocator[] {
    const updates = wrapper.emitted('update:selectedSources')
    return (updates?.at(-1)?.[0] || []) as SourceLocator[]
  }

  it('renders path segments with one separator and only lists directories', async () => {
    const wrapper = mountDestination()
    await flushPromises()

    const breadcrumb = wrapper.get('[data-testid="ps5-breadcrumb"]')
    expect(breadcrumb.text()).toBe('客厅 PS5/data/homebrew')
    expect(breadcrumb.text()).not.toContain('//')
    expect(wrapper.findAll('tbody tr[data-entry-path]').map((row) => row.attributes('data-entry-path'))).toEqual(['/data/homebrew/Games'])
    expect(wrapper.text()).not.toContain('eboot.bin')
  })

  it('defines table columns so responsive layouts can collapse hidden columns', async () => {
    const manager = mountManage()
    const destination = mountDestination()
    await flushPromises()

    expect(manager.findAll('.station-table col').map((column) => column.classes().join(' '))).toEqual([
      'check-column',
      '',
      'type-column',
      'size-column',
      'time-column',
    ])
    expect(destination.findAll('.station-table col').map((column) => column.classes().join(' '))).toEqual([
      '',
      'time-column',
    ])
  })

  it('shows PS5 connection failures as a global message instead of page content', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 502,
      json: async () => ({ error: '无法连接到 PS5' }),
    } as Response)

    const wrapper = mountDestination()
    await flushPromises()

    expect(MessagePlugin.error).toHaveBeenCalledWith('无法连接到 PS5')
    expect(wrapper.text()).not.toContain('无法连接到 PS5')
  })

  it('uses the directory currently being browsed as the destination', async () => {
    const wrapper = mountDestination()
    await flushPromises()

    await wrapper.get('tr[data-entry-path="/data/homebrew/Games"]').trigger('dblclick')
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['/data/homebrew/Games'])
  })

  it('uses destination mode as a path-only picker without mutating commands', async () => {
    const wrapper = mountDestination()
    await flushPromises()
    expect(wrapper.text()).not.toContain('新建文件夹')
    expect(wrapper.text()).not.toContain('重命名')
    await wrapper.get('tr[data-entry-path="/data/homebrew/Games"]').trigger('dblclick')
    await flushPromises()
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['/data/homebrew/Games'])
    expect(requests.some((request) => request.method === 'POST')).toBe(false)
  })

  it('uses the same browser for fnOS sources while hiding the numeric UID path', async () => {
    const wrapper = mountSource()
    await flushPromises()

    expect(wrapper.findAll('tbody tr[data-entry-path]')).toHaveLength(3)
    await wrapper.get('tr[data-entry-path="1000/PS5 游戏"] .check-column input').trigger('click')
    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/PS5 游戏' }])

    await wrapper.get('tr[data-entry-path="1000/PS5 游戏"]').trigger('dblclick')
    await flushPromises()
    expect(wrapper.get('[data-testid="ps5-breadcrumb"]').text()).toBe('存储空间 2/PS5 游戏')
    expect(wrapper.get('[data-testid="ps5-breadcrumb"]').text()).not.toContain('1000')
  })

  it('selects a child on the first click after opening a selected directory', async () => {
    const wrapper = mountSource()
    await flushPromises()

    const directory = wrapper.get('tr[data-entry-path="1000/PS5 游戏"]')
    await directory.trigger('click')
    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/PS5 游戏' }])

    await directory.trigger('dblclick')
    await flushPromises()
    expect(latestSources(wrapper)).toEqual([])
    await wrapper.get('tr[data-entry-path="1000/PS5 游戏/eboot.bin"]').trigger('click')

    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/PS5 游戏/eboot.bin' }])
    expect(wrapper.get('tr[data-entry-path="1000/PS5 游戏/eboot.bin"]').classes()).toContain('is-selected')
  })

  it('loads and selects correctly when bootstrap supplies the first Library Root after mounting', async () => {
    const library = useLibraryStore()
    const wrapper = mount(FileBrowser, {
      props: { mode: 'source', compact: true },
      global: { stubs, mocks: { $router: { push: vi.fn() } } },
    })
    await flushPromises()
    expect(wrapper.findAll('tbody tr[data-entry-path]')).toHaveLength(0)

    library.hydrate([{ id: 'vol2', label: '存储空间 2', favorite: false, kind: 'volume' }])
    await flushPromises()

    expect(wrapper.findAll('tbody tr[data-entry-path]')).toHaveLength(3)
    await wrapper.get('tr[data-entry-path="1000/PS5 游戏"] .check-column input').trigger('click')
    await flushPromises()
    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/PS5 游戏' }])
  })

  it('implements desktop plain-click, Ctrl/Command, and Shift selection semantics', async () => {
    const wrapper = mountSource()
    await flushPromises()
    const directory = wrapper.get('tr[data-entry-path="1000/PS5 游戏"]')
    const image = wrapper.get('tr[data-entry-path="1000/backup.exfat"]')
    const readme = wrapper.get('tr[data-entry-path="1000/readme.txt"]')

    await directory.trigger('click')
    await image.trigger('click', { ctrlKey: true })
    expect(latestSources(wrapper)).toEqual([
      { root_id: 'vol2', path: '1000/PS5 游戏' },
      { root_id: 'vol2', path: '1000/backup.exfat' },
    ])

    await readme.trigger('click')
    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/readme.txt' }])

    await directory.trigger('click')
    await readme.trigger('click', { shiftKey: true })
    expect(latestSources(wrapper)).toEqual([
      { root_id: 'vol2', path: '1000/PS5 游戏' },
      { root_id: 'vol2', path: '1000/backup.exfat' },
      { root_id: 'vol2', path: '1000/readme.txt' },
    ])

    await image.trigger('click', { metaKey: true })
    expect(latestSources(wrapper)).toEqual([
      { root_id: 'vol2', path: '1000/PS5 游戏' },
      { root_id: 'vol2', path: '1000/readme.txt' },
    ])
  })

  it('uses item checkboxes as additive toggles and exposes partial/all header state', async () => {
    const wrapper = mountSource()
    await flushPromises()
    const directory = wrapper.get('tr[data-entry-path="1000/PS5 游戏"]')
    const imageCheckbox = wrapper.get('tr[data-entry-path="1000/backup.exfat"] .check-column input')
    const headerCheckbox = wrapper.get('thead .check-column input')

    await directory.trigger('click')
    await imageCheckbox.trigger('click')
    expect(latestSources(wrapper)).toEqual([
      { root_id: 'vol2', path: '1000/PS5 游戏' },
      { root_id: 'vol2', path: '1000/backup.exfat' },
    ])
    expect(headerCheckbox.attributes('data-indeterminate')).toBe('true')

    await headerCheckbox.trigger('click')
    expect(latestSources(wrapper)).toHaveLength(3)
    expect(headerCheckbox.attributes('data-indeterminate')).toBe('false')

    await headerCheckbox.trigger('click')
    expect(latestSources(wrapper)).toEqual([])
  })

  it('preserves selected rows on right-click and replaces selection for an unselected row', async () => {
    const wrapper = mountSource()
    await flushPromises()
    const directory = wrapper.get('tr[data-entry-path="1000/PS5 游戏"]')
    const image = wrapper.get('tr[data-entry-path="1000/backup.exfat"]')
    const readme = wrapper.get('tr[data-entry-path="1000/readme.txt"]')

    await directory.trigger('click')
    await image.trigger('click', { ctrlKey: true })
    await image.trigger('contextmenu', { clientX: 40, clientY: 40 })
    await flushPromises()
    expect(latestSources(wrapper)).toEqual([
      { root_id: 'vol2', path: '1000/PS5 游戏' },
      { root_id: 'vol2', path: '1000/backup.exfat' },
    ])

    await readme.trigger('contextmenu', { clientX: 60, clientY: 60 })
    await flushPromises()
    expect(latestSources(wrapper)).toEqual([{ root_id: 'vol2', path: '1000/readme.txt' }])
    expect(document.body.querySelector('[role="menu"]')).not.toBeNull()
  })

  it('keeps file opening separate from selection and clears selection on blank space', async () => {
    const wrapper = mountSource()
    await flushPromises()
    const directory = wrapper.get('tr[data-entry-path="1000/PS5 游戏"]')
    const image = wrapper.get('tr[data-entry-path="1000/backup.exfat"]')

    await directory.trigger('click')
    const updateCount = wrapper.emitted('update:selectedSources')?.length
    await image.trigger('dblclick')
    await flushPromises()
    expect(wrapper.emitted('update:selectedSources')?.length).toBe(updateCount)
    expect(wrapper.text()).toContain('backup.exfat')

    await wrapper.get('.station-table-wrap').trigger('click')
    expect(latestSources(wrapper)).toEqual([])
  })

  it('offers extraction only for a .7z archive or its first split volume', async () => {
    const wrapper = mountSource()
    await flushPromises()
    const library = useLibraryStore()
    library.entries = [
      { name: 'single.7z', path: '1000/single.7z', is_dir: false, size: 1 },
      { name: 'split.7z.001', path: '1000/split.7z.001', is_dir: false, size: 1 },
      { name: 'split.7z.002', path: '1000/split.7z.002', is_dir: false, size: 1 },
    ]
    await flushPromises()
    const extractionButton = () => wrapper.findAll('.station-actions button').find((button) => button.text().includes('解压'))!

    await wrapper.get('tr[data-entry-path="1000/single.7z"]').trigger('click')
    await extractionButton().trigger('click')
    expect(wrapper.emitted('extract-archive')?.at(-1)?.[0]).toEqual(library.entries[0])

    await wrapper.get('tr[data-entry-path="1000/split.7z.001"]').trigger('click')
    await extractionButton().trigger('click')
    expect(wrapper.emitted('extract-archive')?.at(-1)?.[0]).toEqual(library.entries[1])

    const emitted = wrapper.emitted('extract-archive')?.length
    await wrapper.get('tr[data-entry-path="1000/split.7z.002"]').trigger('click')
    expect(extractionButton().attributes('disabled')).toBeDefined()
    await extractionButton().trigger('click')
    expect(wrapper.emitted('extract-archive')?.length).toBe(emitted)
  })

  it('selects a picker target on one click and navigates only on double click', async () => {
    const wrapper = mountDestination()
    await flushPromises()
    const games = wrapper.get('tr[data-entry-path="/data/homebrew/Games"]')

    await games.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)).toEqual(['/data/homebrew/Games'])
    expect(wrapper.get('[data-testid="ps5-breadcrumb"]').text()).toBe('客厅 PS5/data/homebrew')
    expect(games.classes()).toContain('is-target')

    await games.trigger('dblclick')
    await flushPromises()
    expect(wrapper.get('[data-testid="ps5-breadcrumb"]').text()).toBe('客厅 PS5/data/homebrew/Games')
  })
})

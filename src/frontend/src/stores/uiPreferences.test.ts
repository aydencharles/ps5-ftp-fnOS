import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UI_PREFERENCES_STORAGE_KEY, useUiPreferencesStore } from './uiPreferences'

function createMemoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: key => values.get(key) ?? null,
    key: index => [...values.keys()][index] ?? null,
    removeItem: key => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  }
}

describe('ui preferences store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.stubGlobal('localStorage', createMemoryStorage())
    localStorage.clear()
    delete document.documentElement.dataset.uiFontSize
    delete document.documentElement.dataset.uiButtonSize
  })

  it('applies the standard defaults when no preferences were saved', () => {
    const store = useUiPreferencesStore()

    store.hydrate()

    expect(store.fontSize).toBe('standard')
    expect(store.buttonSize).toBe('standard')
    expect(document.documentElement.dataset.uiFontSize).toBe('standard')
    expect(document.documentElement.dataset.uiButtonSize).toBe('standard')
  })

  it('hydrates and applies valid saved preferences', () => {
    localStorage.setItem(UI_PREFERENCES_STORAGE_KEY, JSON.stringify({ fontSize: 'large', buttonSize: 'compact' }))
    const store = useUiPreferencesStore()

    store.hydrate()

    expect(store.fontSize).toBe('large')
    expect(store.buttonSize).toBe('compact')
    expect(document.documentElement.dataset.uiFontSize).toBe('large')
    expect(document.documentElement.dataset.uiButtonSize).toBe('compact')
  })

  it('keeps defaults for invalid values', () => {
    localStorage.setItem(UI_PREFERENCES_STORAGE_KEY, JSON.stringify({ fontSize: 'huge', buttonSize: 12 }))
    const store = useUiPreferencesStore()

    store.hydrate()

    expect(store.fontSize).toBe('standard')
    expect(store.buttonSize).toBe('standard')
  })

  it('persists changes and applies them immediately', () => {
    const store = useUiPreferencesStore()

    store.setFontSize('small')
    store.setButtonSize('large')

    expect(JSON.parse(localStorage.getItem(UI_PREFERENCES_STORAGE_KEY) || '{}')).toEqual({
      fontSize: 'small',
      buttonSize: 'large',
    })
    expect(document.documentElement.dataset.uiFontSize).toBe('small')
    expect(document.documentElement.dataset.uiButtonSize).toBe('large')
  })
})

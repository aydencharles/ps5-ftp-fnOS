import { defineStore } from 'pinia'

export type UiFontSize = 'small' | 'standard' | 'large'
export type UiButtonSize = 'compact' | 'standard' | 'large'

export const UI_PREFERENCES_STORAGE_KEY = 'ps5-ftp-manager.ui-preferences'

const fontSizes: UiFontSize[] = ['small', 'standard', 'large']
const buttonSizes: UiButtonSize[] = ['compact', 'standard', 'large']

function isFontSize(value: unknown): value is UiFontSize {
  return fontSizes.includes(value as UiFontSize)
}

function isButtonSize(value: unknown): value is UiButtonSize {
  return buttonSizes.includes(value as UiButtonSize)
}

export const useUiPreferencesStore = defineStore('uiPreferences', {
  state: () => ({
    fontSize: 'standard' as UiFontSize,
    buttonSize: 'standard' as UiButtonSize,
  }),
  actions: {
    hydrate() {
      try {
        const saved = JSON.parse(localStorage.getItem(UI_PREFERENCES_STORAGE_KEY) || '{}') as Record<string, unknown>
        if (isFontSize(saved.fontSize)) this.fontSize = saved.fontSize
        if (isButtonSize(saved.buttonSize)) this.buttonSize = saved.buttonSize
      } catch {
        // Ignore unreadable local preferences and keep the documented defaults.
      }
      this.apply()
    },
    setFontSize(value: UiFontSize) {
      this.fontSize = value
      this.commit()
    },
    setButtonSize(value: UiButtonSize) {
      this.buttonSize = value
      this.commit()
    },
    commit() {
      this.apply()
      try {
        localStorage.setItem(UI_PREFERENCES_STORAGE_KEY, JSON.stringify({
          fontSize: this.fontSize,
          buttonSize: this.buttonSize,
        }))
      } catch {
        // Applying the preference is still useful if storage is unavailable.
      }
    },
    apply() {
      document.documentElement.dataset.uiFontSize = this.fontSize
      document.documentElement.dataset.uiButtonSize = this.buttonSize
    },
  },
})

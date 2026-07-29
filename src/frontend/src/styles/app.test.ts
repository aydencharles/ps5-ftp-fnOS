import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(resolve(process.cwd(), 'src/styles/app.less'), 'utf8')

describe('global UI standards', () => {
  it('keeps the button-size illustration visible', () => {
    expect(styles).toMatch(/\.interface-button-icon i \{[^}]*width: 100%/)
  })

  it('assigns semantic font tokens to file-browser entry text', () => {
    expect(styles).toMatch(/\.station-file-name \{[^}]*font-size: var\(--ui-font-body\)/)
    expect(styles).toMatch(/\.file-name-copy > small \{[^}]*font-size: var\(--ui-font-tiny\)/)
    expect(styles).toMatch(/\.station-table \{[^}]*font-size: var\(--ui-font-small\)/)
  })

  it('does not bypass the typography standard with fixed font sizes', () => {
    expect(styles).not.toMatch(/font-size:\s*\d+px/)
    expect(styles).not.toMatch(/font:\s*[^;{}]*\d+px/)
  })

  it('uses shared sizing tokens for standard action buttons', () => {
    for (const selector of ['.t-button,', '.file-context-menu button', '.task-center-actions button']) {
      const rule = styles.slice(styles.indexOf(selector), styles.indexOf('}', styles.indexOf(selector)))
      expect(rule).toContain('var(--ui-button-height)')
      expect(rule).toContain('var(--ui-button-padding-x)')
      expect(rule).toContain('var(--ui-button-font-size)')
    }
  })
})

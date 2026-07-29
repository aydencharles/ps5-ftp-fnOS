import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const styles = readFileSync(resolve(process.cwd(), 'src/styles/app.less'), 'utf8')
const mobileStyles = styles.slice(
  styles.indexOf('@media (max-width: 600px)'),
  styles.indexOf('@media (prefers-reduced-motion: reduce)'),
)

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

  it('keeps dense workbench content inside a phone viewport', () => {
    expect(mobileStyles).toMatch(/\.station-actions \{[^}]*flex-wrap: wrap/)
    expect(mobileStyles).toMatch(/\.station-table \{[^}]*min-width: 0/)
    expect(mobileStyles).toMatch(/\.station-table \.type-column, \.station-table \.time-column \{[^}]*display: none/)
    expect(mobileStyles).not.toContain('min-width: 560px')
  })

  it('stacks settings and forms into a single mobile column', () => {
    expect(mobileStyles).toMatch(/\.settings-section \{[^}]*grid-template-columns: 1fr/)
    expect(mobileStyles).toMatch(/\.profile-row \{[^}]*minmax\(0, 1fr\)/)
    expect(mobileStyles).toMatch(/\.form-grid \{[^}]*grid-template-columns: 1fr/)
  })

  it('keeps settings section endings visually consistent', () => {
    expect(styles).toMatch(/\.profile-row:last-child \{ border-bottom: 0; \}/)
    expect(styles).toMatch(/\.settings-body > \.t-slider__container \{ margin: 24px 12px 6px; \}/)
    expect(mobileStyles).toMatch(/\.profile-row:last-child \{ padding-bottom: 0; \}/)
    expect(mobileStyles).toMatch(/\.settings-body > \.t-slider__container \{ margin-bottom: 26px; \}/)
  })
})

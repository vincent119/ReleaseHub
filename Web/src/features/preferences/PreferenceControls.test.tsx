import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/shared/i18n/config'
import { ThemePreferenceProvider } from '@/shared/theme/ThemePreferenceProvider'

import { PreferenceControls } from './PreferenceControls'

describe('PreferenceControls', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  it('renders the Traditional Chinese preference labels', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ThemePreferenceProvider>
          <PreferenceControls />
        </ThemePreferenceProvider>
      </I18nextProvider>,
    )

    expect(screen.getByText('語言')).toBeInTheDocument()
    expect(screen.getByText('主題')).toBeInTheDocument()
  })
})

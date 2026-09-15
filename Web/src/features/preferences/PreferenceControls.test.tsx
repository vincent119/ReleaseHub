import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/shared/i18n/config'
import { ThemePreferenceProvider } from '@/shared/theme/ThemePreferenceProvider'

import {
  LanguagePreferenceControl,
  ThemePreferenceControl,
} from './PreferenceControls'

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

  afterEach(cleanup)

  it('renders only the language options and applies the selection', async () => {
    renderControl(<LanguagePreferenceControl />)

    expect(screen.getByRole('combobox', { name: '語言' })).toBeInTheDocument()
    expect(screen.queryByText('語言')).toBeNull()
    expect(screen.queryByText('主題')).toBeNull()

    fireEvent.mouseDown(screen.getByLabelText('語言'))
    fireEvent.click(screen.getByTitle('English'))

    await waitFor(() => expect(i18n.resolvedLanguage).toBe('en'))
    expect(localStorage.getItem('releasehub.language')).toBe('en')
  })

  it('renders only the theme options and applies the selection', async () => {
    renderControl(<ThemePreferenceControl />)

    expect(screen.getByRole('combobox', { name: '主題' })).toBeInTheDocument()
    expect(screen.queryByText('主題')).toBeNull()
    expect(screen.queryByText('語言')).toBeNull()

    fireEvent.mouseDown(screen.getByLabelText('主題'))
    fireEvent.click(screen.getByTitle('深色'))

    await waitFor(() =>
      expect(document.documentElement).toHaveAttribute('data-theme', 'dark'),
    )
    expect(localStorage.getItem('releasehub.theme')).toBe('dark')
  })
})

function renderControl(control: React.ReactNode) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ThemePreferenceProvider>{control}</ThemePreferenceProvider>
    </I18nextProvider>,
  )
}

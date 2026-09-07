import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'

import {
  resolveTheme,
  THEME_STORAGE_KEY,
  ThemePreferenceProvider,
} from './ThemePreferenceProvider'
import { useThemePreference } from './useThemePreference'

function ThemeProbe() {
  const { preference, resolvedTheme, setPreference } = useThemePreference()

  return (
    <>
      <output>{`${preference}:${resolvedTheme}`}</output>
      <button type="button" onClick={() => setPreference('dark')}>
        Set dark
      </button>
    </>
  )
}

describe('resolveTheme', () => {
  it('uses the system preference only when the saved preference is system', () => {
    expect(resolveTheme('system', true)).toBe('dark')
    expect(resolveTheme('system', false)).toBe('light')
    expect(resolveTheme('light', true)).toBe('light')
  })
})

describe('ThemePreferenceProvider', () => {
  beforeEach(() => {
    localStorage.clear()
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  it('falls back to system and persists a changed preference', () => {
    render(
      <ThemePreferenceProvider>
        <ThemeProbe />
      </ThemePreferenceProvider>,
    )

    expect(screen.getByText('system:light')).toBeInTheDocument()

    act(() => screen.getByRole('button', { name: 'Set dark' }).click())

    expect(screen.getByText('dark:dark')).toBeInTheDocument()
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
  })
})

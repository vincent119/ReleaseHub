import { useContext } from 'react'

import {
  type ThemePreference,
  ThemePreferenceContext,
} from './ThemePreferenceProvider'

export type { ThemePreference }

export function useThemePreference() {
  const context = useContext(ThemePreferenceContext)

  if (!context) {
    throw new Error(
      'useThemePreference must be used within ThemePreferenceProvider',
    )
  }

  return context
}

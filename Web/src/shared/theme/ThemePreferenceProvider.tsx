import {
  createContext,
  type Dispatch,
  type PropsWithChildren,
  type SetStateAction,
  useEffect,
  useMemo,
  useState,
} from 'react'

export type ThemePreference = 'system' | 'light' | 'dark'
export type ResolvedTheme = Exclude<ThemePreference, 'system'>

export const THEME_STORAGE_KEY = 'releasehub.theme'

export interface ThemePreferenceContextValue {
  preference: ThemePreference
  resolvedTheme: ResolvedTheme
  setPreference: Dispatch<SetStateAction<ThemePreference>>
}

export const ThemePreferenceContext =
  createContext<ThemePreferenceContextValue | null>(null)

export function resolveTheme(
  preference: ThemePreference,
  systemPrefersDark: boolean,
): ResolvedTheme {
  if (preference === 'system') {
    return systemPrefersDark ? 'dark' : 'light'
  }

  return preference
}

function getInitialPreference(): ThemePreference {
  const storedPreference = localStorage.getItem(THEME_STORAGE_KEY)

  if (
    storedPreference === 'light' ||
    storedPreference === 'dark' ||
    storedPreference === 'system'
  ) {
    return storedPreference
  }

  return 'system'
}

function getSystemPrefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

export function ThemePreferenceProvider({ children }: PropsWithChildren) {
  const [preference, setPreference] =
    useState<ThemePreference>(getInitialPreference)
  const [systemPrefersDark, setSystemPrefersDark] =
    useState(getSystemPrefersDark)
  const resolvedTheme = resolveTheme(preference, systemPrefersDark)

  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (event: MediaQueryListEvent) =>
      setSystemPrefersDark(event.matches)

    mediaQuery.addEventListener('change', onChange)
    return () => mediaQuery.removeEventListener('change', onChange)
  }, [])

  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, preference)
    document.documentElement.dataset.theme = resolvedTheme
  }, [preference, resolvedTheme])

  const value = useMemo(
    () => ({ preference, resolvedTheme, setPreference }),
    [preference, resolvedTheme],
  )

  return (
    <ThemePreferenceContext.Provider value={value}>
      {children}
    </ThemePreferenceContext.Provider>
  )
}

import { App as AntdApp, ConfigProvider, theme } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { I18nextProvider } from 'react-i18next'

import i18n from '@/shared/i18n/config'
import { ThemePreferenceProvider } from '@/shared/theme/ThemePreferenceProvider'
import { useThemePreference } from '@/shared/theme/useThemePreference'

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
})

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <I18nextProvider i18n={i18n}>
      <ThemePreferenceProvider>
        <QueryClientProvider client={queryClient}>
          <ThemedProvider>{children}</ThemedProvider>
        </QueryClientProvider>
      </ThemePreferenceProvider>
    </I18nextProvider>
  )
}

function ThemedProvider({ children }: PropsWithChildren) {
  const { resolvedTheme } = useThemePreference()

  return (
    <ConfigProvider
      theme={{
        algorithm:
          resolvedTheme === 'dark'
            ? theme.darkAlgorithm
            : theme.defaultAlgorithm,
        token: {
          borderRadius: 8,
          colorPrimary: '#1677ff',
          fontFamily: 'Inter, "Noto Sans TC", system-ui, sans-serif',
        },
      }}
    >
      <AntdApp>{children}</AntdApp>
    </ConfigProvider>
  )
}

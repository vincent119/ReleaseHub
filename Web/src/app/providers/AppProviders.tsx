import { App as AntdApp, ConfigProvider } from 'antd'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { PropsWithChildren } from 'react'
import { I18nextProvider, useTranslation } from 'react-i18next'

import { resolveAntdLocale } from '@/shared/i18n/antdLocale'
import i18n from '@/shared/i18n/config'
import { ThemePreferenceProvider } from '@/shared/theme/ThemePreferenceProvider'
import { useThemePreference } from '@/shared/theme/useThemePreference'
import { createReleaseHubTheme } from '@/shared/theme/releaseHubTheme'

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
  const { i18n: activeI18n } = useTranslation()
  const { resolvedTheme } = useThemePreference()
  const locale = resolveAntdLocale(
    activeI18n.resolvedLanguage ?? activeI18n.language,
  )

  return (
    <ConfigProvider
      locale={locale}
      theme={createReleaseHubTheme(resolvedTheme)}
    >
      <AntdApp notification={{ maxCount: 3, placement: 'topRight' }}>
        {children}
      </AntdApp>
    </ConfigProvider>
  )
}

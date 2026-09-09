import { ConfigProvider, Empty } from 'antd'
import { act, cleanup, render, screen } from '@testing-library/react'
import { useContext } from 'react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import i18n from '@/shared/i18n/config'

import { AppProviders } from './AppProviders'

function LocaleProbe() {
  const { locale } = useContext(ConfigProvider.ConfigContext)
  return <output aria-label="Ant Design locale">{locale?.locale}</output>
}

describe('AppProviders Ant Design locale', () => {
  afterEach(() => cleanup())

  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('en')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  it('updates the provider and built-in component copy without remounting', async () => {
    const view = render(
      <AppProviders>
        <LocaleProbe />
        <Empty />
      </AppProviders>,
    )

    expect(screen.getByLabelText('Ant Design locale')).toHaveTextContent('en')
    expect(
      view.container.querySelector('.ant-empty-description'),
    ).toHaveTextContent('No data')

    await act(async () => {
      await i18n.changeLanguage('zh-TW')
    })

    expect(screen.getByLabelText('Ant Design locale')).toHaveTextContent(
      'zh-tw',
    )
    expect(
      view.container.querySelector('.ant-empty-description'),
    ).toHaveTextContent('暫無資料')
  })

  it('falls back to English when i18next reports an unsupported locale', async () => {
    await i18n.changeLanguage('ja')

    render(
      <AppProviders>
        <LocaleProbe />
      </AppProviders>,
    )

    expect(screen.getByLabelText('Ant Design locale')).toHaveTextContent('en')
  })
})

import enUS from 'antd/locale/en_US'
import zhTW from 'antd/locale/zh_TW'
import { describe, expect, it } from 'vitest'

import { antdLocaleRegistry, resolveAntdLocale } from './antdLocale'

describe('Ant Design locale registry', () => {
  it('maps the supported ReleaseHub locales explicitly', () => {
    expect(antdLocaleRegistry.en).toBe(enUS)
    expect(antdLocaleRegistry['zh-TW']).toBe(zhTW)
  })

  it.each([undefined, '', 'en-US', 'ja', 'zh-CN'])(
    'falls back to English for %s',
    (language) => {
      expect(resolveAntdLocale(language)).toBe(enUS)
    },
  )

  it('resolves Traditional Chinese without changing its locale code', () => {
    expect(resolveAntdLocale('zh-TW')).toBe(zhTW)
  })
})

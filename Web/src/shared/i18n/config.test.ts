import { createInstance } from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('document 語言同步', () => {
  beforeEach(() => {
    vi.resetModules()
    // 每個案例使用真實的新 instance，避免 singleton 的舊監聽器掩蓋首次載入缺陷。
    vi.doMock('i18next', () => ({ default: createInstance() }))
    localStorage.clear()
    document.documentElement.lang = 'zh-Hant'
  })

  afterEach(() => vi.doUnmock('i18next'))

  it.each([
    ['en', 'en'],
    ['zh-TW', 'zh-TW'],
    [null, 'en'],
    ['ja', 'en'],
  ])('初始偏好 %s 同步為 %s', async (preference, expected) => {
    if (preference) localStorage.setItem('releasehub.language', preference)
    const { default: i18n } = await import('./config')
    await vi.waitFor(() => {
      expect(i18n.resolvedLanguage).toBe(expected)
      expect(document.documentElement.lang).toBe(expected)
    })
  })

  it('雙向切換保存偏好，重新初始化保持已選語言', async () => {
    localStorage.setItem('releasehub.language', 'en')
    const { default: i18n } = await import('./config')
    await i18n.changeLanguage('zh-TW')
    expect(document.documentElement.lang).toBe('zh-TW')
    expect(localStorage.getItem('releasehub.language')).toBe('zh-TW')
    await i18n.changeLanguage('en')
    expect(document.documentElement.lang).toBe('en')
    expect(localStorage.getItem('releasehub.language')).toBe('en')

    vi.resetModules()
    document.documentElement.lang = 'zh-Hant'
    vi.doMock('i18next', () => ({ default: createInstance() }))
    await import('./config')
    await vi.waitFor(() => expect(document.documentElement.lang).toBe('en'))
  })
})

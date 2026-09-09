import enUS from 'antd/locale/en_US'
import zhTW from 'antd/locale/zh_TW'

export const antdLocaleRegistry = {
  en: enUS,
  'zh-TW': zhTW,
} as const

export type SupportedLocale = keyof typeof antdLocaleRegistry

export function resolveAntdLocale(language?: string) {
  if (language === 'zh-TW') return antdLocaleRegistry['zh-TW']
  return antdLocaleRegistry.en
}

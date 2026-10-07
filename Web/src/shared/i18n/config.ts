import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import { en } from './locales/en'
import { zhTW } from './locales/zh-TW'

export const LANGUAGE_STORAGE_KEY = 'releasehub.language'

// 初始化也會發出語言事件，需先訂閱才能同步首次載入的 document 語言。
i18n.on('languageChanged', (language) => {
  localStorage.setItem(LANGUAGE_STORAGE_KEY, language)
  document.documentElement.lang = language
})

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: en },
    'zh-TW': { translation: zhTW },
  },
  lng: localStorage.getItem(LANGUAGE_STORAGE_KEY) ?? undefined,
  fallbackLng: 'en',
  supportedLngs: ['en', 'zh-TW'],
  interpolation: {
    escapeValue: false,
  },
})

export default i18n

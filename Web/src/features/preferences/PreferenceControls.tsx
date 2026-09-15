import { Select } from 'antd'
import { useTranslation } from 'react-i18next'

import {
  type ThemePreference,
  useThemePreference,
} from '@/shared/theme/useThemePreference'

import styles from './PreferenceControls.module.css'

type SupportedLanguage = 'en' | 'zh-TW'

export function LanguagePreferenceControl() {
  const { i18n, t } = useTranslation()
  const language: SupportedLanguage =
    i18n.resolvedLanguage === 'zh-TW' ? 'zh-TW' : 'en'

  return (
    <Select<SupportedLanguage>
      aria-label={t('preferences.language.label')}
      className={styles.select}
      options={[
        { label: t('preferences.language.english'), value: 'en' },
        {
          label: t('preferences.language.traditionalChinese'),
          value: 'zh-TW',
        },
      ]}
      value={language}
      onChange={(nextLanguage) => void i18n.changeLanguage(nextLanguage)}
    />
  )
}

export function ThemePreferenceControl() {
  const { t } = useTranslation()
  const { preference, setPreference } = useThemePreference()

  return (
    <Select<ThemePreference>
      aria-label={t('preferences.theme.label')}
      className={styles.select}
      options={[
        { label: t('preferences.theme.system'), value: 'system' },
        { label: t('preferences.theme.light'), value: 'light' },
        { label: t('preferences.theme.dark'), value: 'dark' },
      ]}
      value={preference}
      onChange={setPreference}
    />
  )
}

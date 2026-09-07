import { Segmented, Space, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import {
  type ThemePreference,
  useThemePreference,
} from '@/shared/theme/useThemePreference'

export function PreferenceControls() {
  const { i18n, t } = useTranslation()
  const { preference, setPreference } = useThemePreference()

  return (
    <Space direction="vertical" size="middle">
      <div>
        <Typography.Text strong>
          {t('preferences.language.label')}
        </Typography.Text>
        <Segmented
          aria-label={t('preferences.language.label')}
          block
          options={[
            { label: t('preferences.language.english'), value: 'en' },
            {
              label: t('preferences.language.traditionalChinese'),
              value: 'zh-TW',
            },
          ]}
          value={i18n.resolvedLanguage}
          onChange={(language) => void i18n.changeLanguage(language)}
        />
      </div>
      <div>
        <Typography.Text strong>{t('preferences.theme.label')}</Typography.Text>
        <Segmented<ThemePreference>
          aria-label={t('preferences.theme.label')}
          block
          options={[
            { label: t('preferences.theme.system'), value: 'system' },
            { label: t('preferences.theme.light'), value: 'light' },
            { label: t('preferences.theme.dark'), value: 'dark' },
          ]}
          value={preference}
          onChange={setPreference}
        />
      </div>
    </Space>
  )
}

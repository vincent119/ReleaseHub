import { Modal, Popover } from 'antd'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  LanguagePreferenceControl,
  ThemePreferenceControl,
} from '@/features/preferences'

import styles from './AccountMenu.module.css'

export type PreferenceSettingsKind = 'language' | 'theme'

interface PreferenceSettingsProps {
  anchor: ReactNode
  desktop: boolean
  kind: PreferenceSettingsKind
  open: boolean
  onClose: () => void
  onAfterClose: () => void
}

export function PreferenceSettings({
  anchor,
  desktop,
  kind,
  open,
  onClose,
  onAfterClose,
}: PreferenceSettingsProps) {
  const { t } = useTranslation()
  const title =
    kind === 'language'
      ? t('account.languageSettings')
      : t('account.themeSettings')
  const controls = (
    <div className={styles.preferenceSettings}>
      {kind === 'language' ? (
        <LanguagePreferenceControl />
      ) : (
        <ThemePreferenceControl />
      )}
    </div>
  )

  return (
    <>
      <Popover
        open={desktop && open}
        title={title}
        content={controls}
        placement="bottomRight"
        trigger="click"
        onOpenChange={(nextOpen) => {
          if (!nextOpen) onClose()
        }}
      >
        {anchor}
      </Popover>
      <Modal
        open={!desktop && open}
        title={title}
        footer={null}
        onCancel={onClose}
        afterClose={onAfterClose}
      >
        {controls}
      </Modal>
    </>
  )
}

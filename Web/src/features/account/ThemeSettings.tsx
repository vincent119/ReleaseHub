import { Modal, Popover } from 'antd'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PreferenceControls } from '@/features/preferences'

import styles from './AccountMenu.module.css'

interface ThemeSettingsProps {
  anchor: ReactNode
  desktop: boolean
  open: boolean
  onClose: () => void
  onAfterClose: () => void
}

export function ThemeSettings({
  anchor,
  desktop,
  open,
  onClose,
  onAfterClose,
}: ThemeSettingsProps) {
  const { t } = useTranslation()
  const controls = (
    <div className={styles.themeSettings}>
      <PreferenceControls />
    </div>
  )

  return (
    <>
      <Popover
        open={desktop && open}
        title={t('account.themeSettings')}
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
        title={t('account.themeSettings')}
        footer={null}
        onCancel={onClose}
        afterClose={onAfterClose}
      >
        {controls}
      </Modal>
    </>
  )
}

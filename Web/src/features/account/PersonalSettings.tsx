import { LockOutlined } from '@ant-design/icons'
import { Descriptions, Modal, Typography } from 'antd'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import styles from './AccountMenu.module.css'
import { ChangePasswordForm } from './ChangePasswordForm'

interface PersonalSettingsProps {
  open: boolean
  userId: string
  username: string
  passwordChangeAvailable: boolean
  csrfToken?: string
  onClose: () => void
  onAfterClose: () => void
}

export function PersonalSettings({
  open,
  userId,
  username,
  passwordChangeAvailable,
  csrfToken,
  onClose,
  onAfterClose,
}: PersonalSettingsProps) {
  const { t } = useTranslation()
  const [passwordSubmitting, setPasswordSubmitting] = useState(false)

  const close = () => {
    if (passwordSubmitting) return
    onClose()
  }

  const afterClose = () => {
    setPasswordSubmitting(false)
    onAfterClose()
  }

  return (
    <Modal
      open={open}
      title={t('account.personalSettings')}
      footer={null}
      width={580}
      className={styles.personalSettingsModal}
      closable={!passwordSubmitting}
      keyboard={!passwordSubmitting}
      mask={{ closable: !passwordSubmitting }}
      onCancel={close}
      afterClose={afterClose}
      destroyOnHidden
    >
      <div className={styles.personalSettingsContent}>
        <Descriptions
          className={styles.identityDetails}
          column={1}
          size="small"
          bordered
        >
          <Descriptions.Item label={t('account.fields.username')}>
            <Typography.Text>{username}</Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label={t('account.fields.userId')}>
            <Typography.Text className={styles.userId}>
              {userId}
            </Typography.Text>
          </Descriptions.Item>
        </Descriptions>
        {passwordChangeAvailable && (
          <section
            className={styles.passwordSection}
            aria-labelledby="personal-settings-password-title"
          >
            <div className={styles.passwordSectionHeader}>
              <span className={styles.passwordSectionIcon} aria-hidden="true">
                <LockOutlined />
              </span>
              <div>
                <Typography.Title
                  level={5}
                  id="personal-settings-password-title"
                  className={styles.passwordSectionTitle}
                >
                  {t('account.password.title')}
                </Typography.Title>
                <Typography.Text type="secondary">
                  {t('account.password.description')}
                </Typography.Text>
              </div>
            </div>
            <ChangePasswordForm
              csrfToken={csrfToken}
              onCancel={close}
              onSuccess={onClose}
              onSubmittingChange={setPasswordSubmitting}
            />
          </section>
        )}
      </div>
    </Modal>
  )
}

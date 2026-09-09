import { Descriptions, Modal, Typography } from 'antd'
import { useTranslation } from 'react-i18next'

import styles from './AccountMenu.module.css'

interface PersonalSettingsProps {
  open: boolean
  userId: string
  username: string
  onClose: () => void
  onAfterClose: () => void
}

export function PersonalSettings({
  open,
  userId,
  username,
  onClose,
  onAfterClose,
}: PersonalSettingsProps) {
  const { t } = useTranslation()

  return (
    <Modal
      open={open}
      title={t('account.personalSettings')}
      footer={null}
      onCancel={onClose}
      afterClose={onAfterClose}
    >
      <Descriptions column={1} size="small" bordered>
        <Descriptions.Item label={t('account.fields.username')}>
          <Typography.Text>{username}</Typography.Text>
        </Descriptions.Item>
        <Descriptions.Item label={t('account.fields.userId')}>
          <Typography.Text className={styles.userId}>{userId}</Typography.Text>
        </Descriptions.Item>
      </Descriptions>
    </Modal>
  )
}

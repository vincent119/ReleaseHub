import { App as AntdApp } from 'antd'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import styles from './feedback.module.css'

export type FeedbackType = 'success' | 'info' | 'warning' | 'error'

const durationByType: Record<FeedbackType, number> = {
  success: 4,
  info: 4,
  warning: 7,
  error: 0,
}

const titleKeyByType = {
  success: 'feedback.successTitle',
  info: 'feedback.infoTitle',
  warning: 'feedback.warningTitle',
  error: 'feedback.errorTitle',
} as const

export function createFeedbackOptions(
  type: FeedbackType,
  title: string,
  description: string,
) {
  return {
    type,
    title,
    description,
    duration: durationByType[type],
    placement: 'topRight' as const,
    role: type === 'error' ? ('alert' as const) : ('status' as const),
    closeIcon: type === 'success' || type === 'info' ? false : undefined,
    className: `${styles.feedback} ${styles[type]}`,
  }
}

export function useFeedback() {
  const { notification } = AntdApp.useApp()
  const { t } = useTranslation()

  return useMemo(
    () => ({
      success: (description: string) =>
        notification.open(
          createFeedbackOptions(
            'success',
            t(titleKeyByType.success),
            description,
          ),
        ),
      info: (description: string) =>
        notification.open(
          createFeedbackOptions('info', t(titleKeyByType.info), description),
        ),
      warning: (description: string) =>
        notification.open(
          createFeedbackOptions(
            'warning',
            t(titleKeyByType.warning),
            description,
          ),
        ),
      error: (description: string) =>
        notification.open(
          createFeedbackOptions('error', t(titleKeyByType.error), description),
        ),
    }),
    [notification, t],
  )
}

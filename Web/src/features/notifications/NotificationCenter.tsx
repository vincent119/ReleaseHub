import { BellOutlined } from '@ant-design/icons'
import { Badge, Button, Drawer, Empty, Space, Typography } from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  markAllNotificationsRead,
  markNotificationRead,
  useListNotifications,
} from '@/generated/api'
import type { Notification } from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { SemanticList, SemanticListItemContent } from '@/shared/list'
import { SemanticTag } from '@/shared/tag/SemanticTag'

import { useNotificationEvents } from './useNotificationEvents'
import styles from './NotificationCenter.module.css'

export function NotificationCenter() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [open, setOpen] = useState(false)
  const notifications = useListNotifications({ limit: 100 })
  const values =
    notifications.data?.status === 200 ? notifications.data.data.data : []
  const unread = values.filter((value) => !value.read).length
  useNotificationEvents(true)
  const markRead = async (notification: Notification) => {
    if (notification.read) return
    const response = await markNotificationRead(
      notification.id,
      mutationOptions(),
    )
    if (response.status !== 204)
      return void feedback.error(t('notifications.mutation.error'))
    await notifications.refetch()
  }
  const markAll = async () => {
    const response = await markAllNotificationsRead(mutationOptions())
    if (response.status !== 204)
      return void feedback.error(t('notifications.mutation.error'))
    await notifications.refetch()
  }
  return (
    <>
      <Badge count={unread} size="small">
        <Button
          type="text"
          icon={<BellOutlined className={styles.icon} />}
          aria-label={t('notifications.open')}
          onClick={() => setOpen(true)}
        />
      </Badge>
      <Drawer
        open={open}
        size="large"
        title={t('notifications.title')}
        onClose={() => setOpen(false)}
        extra={
          <Button
            disabled={!unread}
            loading={notifications.isFetching}
            onClick={() => void markAll()}
          >
            {t('notifications.markAll')}
          </Button>
        }
      >
        {notifications.isError ||
        (notifications.data && notifications.data.status !== 200) ? (
          <Typography.Text type="danger">
            {t('notifications.unavailable')}
          </Typography.Text>
        ) : values.length ? (
          <SemanticList
            items={values}
            rowKey="id"
            loading={notifications.isPending}
            renderItem={(notification) => (
              <SemanticListItemContent
                actions={[
                  <Button
                    key="read"
                    type="link"
                    disabled={notification.read}
                    onClick={() => void markRead(notification)}
                  >
                    {notification.read
                      ? t('notifications.read')
                      : t('notifications.markRead')}
                  </Button>,
                ]}
                title={
                  <Space>
                    <Badge
                      status={notification.read ? 'default' : 'processing'}
                    />
                    <span>
                      {t(`notifications.events.${notification.eventType}`, {
                        defaultValue: notification.eventType,
                      })}
                    </span>
                    {notification.restricted && (
                      <SemanticTag>{t('notifications.restricted')}</SemanticTag>
                    )}
                  </Space>
                }
                description={
                  <Space orientation="vertical" size={0}>
                    <span>
                      {new Date(notification.occurredAt).toLocaleString()}
                    </span>
                    <NotificationLink
                      value={notification}
                      onNavigate={() => setOpen(false)}
                    />
                  </Space>
                }
              />
            )}
          />
        ) : (
          <Empty description={t('notifications.empty')} />
        )}
      </Drawer>
    </>
  )
}

function NotificationLink({
  value,
  onNavigate,
}: {
  value: Notification
  onNavigate: () => void
}) {
  const { t } = useTranslation()
  const target = notificationTarget(value)
  if (!target || value.restricted) return null
  return (
    <Link to={target} onClick={onNavigate}>
      {t('notifications.openResource')}
    </Link>
  )
}

function notificationTarget(value: Notification) {
  if (value.resourceType === 'deployment_request')
    return `/requests/${value.resourceId}`
  if (value.resourceType === 'application_onboarding')
    return `/applications/${value.resourceId}`
  return undefined
}

function mutationOptions() {
  return { headers: { 'X-CSRF-Token': browserCookie('releasehub_csrf') ?? '' } }
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}

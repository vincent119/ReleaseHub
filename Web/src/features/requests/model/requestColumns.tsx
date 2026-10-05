import { Tag, type TableProps } from 'antd'
import type { TFunction } from 'i18next'

import type { DeploymentRequestSummary } from '@/generated/model'
import { ThemedLink } from '@/shared/link'
import { SemanticTag } from '@/shared/tag/SemanticTag'

import { RequestListDetails } from '../components/RequestListDetails'
import {
  requestStatusColor,
  requestStatusLabel,
  shortRequestID,
} from './presentation'
import styles from '../RequestsPage.module.css'

export function requestColumns(
  t: TFunction,
): TableProps<DeploymentRequestSummary>['columns'] {
  return [
    {
      title: t('requests.columns.title'),
      dataIndex: 'title',
      width: 360,
      render: (title: string, request) => (
        <div className={styles.titleCell}>
          <div className={styles.identityText}>
            <ThemedLink
              className={styles.titleLink}
              to={`/requests/${request.id}`}
            >
              {title}
            </ThemedLink>
            <div className={styles.secondaryLine}>
              <span className={styles.subtle}>
                {shortRequestID(request.id)}
              </span>
              <SemanticTag>{request.classification}</SemanticTag>
            </div>
          </div>
          <RequestListDetails request={request} kind="identity" />
        </div>
      ),
    },
    {
      title: t('requests.columns.status'),
      dataIndex: 'status',
      width: 130,
      render: (status: string) =>
        requestStatusColor(status) === 'default' ? (
          <SemanticTag>{requestStatusLabel(status)}</SemanticTag>
        ) : (
          <Tag color={requestStatusColor(status)}>
            {requestStatusLabel(status)}
          </Tag>
        ),
    },
    {
      title: t('requests.columns.version'),
      dataIndex: 'activeVersionNumber',
      width: 70,
      align: 'right',
    },
    {
      title: t('requests.columns.applications'),
      dataIndex: 'applicationCount',
      width: 110,
      align: 'right',
    },
    {
      title: t('requests.columns.schedule'),
      dataIndex: 'scheduleState',
      width: 150,
      render: (_state: string, request) => (
        <RequestListDetails request={request} kind="schedule" />
      ),
    },
    {
      title: t('requests.columns.updatedAt'),
      dataIndex: 'updatedAt',
      width: 260,
      render: (value: string) => (
        <time className={styles.updatedAt} dateTime={value}>
          {new Date(value).toLocaleString()}
        </time>
      ),
    },
  ]
}

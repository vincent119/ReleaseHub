import { Alert, Card, Empty, Flex, Space, Table, Tag, Typography } from 'antd'
import type { TableProps } from 'antd'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { useState } from 'react'

import {
  useGetCatalogResourceTree,
  useListDeploymentRequests,
} from '@/generated/api'
import type { DeploymentRequestSummary } from '@/generated/model'
import { SemanticTag } from '@/shared/tag/SemanticTag'

import {
  RequestScopeSelector,
  type RequestScope,
} from './components/RequestScopeSelector'
import { requestStatusColor, requestStatusLabel } from './model/presentation'
import styles from './RequestsPage.module.css'

const emptyID = '00000000-0000-0000-0000-000000000000'

export function RequestsPage() {
  const { t } = useTranslation()
  const resources = useGetCatalogResourceTree()
  const [scope, setScope] = useState<RequestScope>()
  const requests = useListDeploymentRequests(
    {
      organizationId: scope?.organizationId ?? emptyID,
      projectId: scope?.projectId ?? emptyID,
      environmentId: scope?.environmentId ?? emptyID,
      limit: 100,
    },
    { query: { enabled: Boolean(scope) } },
  )
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  if (resources.isError || (resources.data && resources.data.status !== 200)) {
    return <Alert type="error" showIcon title={t('requests.unavailable')} />
  }
  return (
    <Space orientation="vertical" size="large" className={styles.page}>
      <Flex justify="space-between" align="start" gap="middle" wrap>
        <div>
          <Typography.Title level={2}>{t('requests.title')}</Typography.Title>
          <Typography.Paragraph type="secondary">
            {t('requests.description')}
          </Typography.Paragraph>
        </div>
      </Flex>
      <Card loading={resources.isPending}>
        <RequestScopeSelector
          organizations={organizations}
          value={scope}
          onChange={setScope}
        />
      </Card>
      {!scope ? (
        <Empty description={t('requests.scope.empty')} />
      ) : requests.isError || requests.data?.status !== 200 ? (
        <Alert type="error" showIcon title={t('requests.unavailable')} />
      ) : (
        <Card>
          <Table<DeploymentRequestSummary>
            rowKey="id"
            columns={requestColumns(t)}
            dataSource={requests.data.data.data}
            loading={requests.isPending}
            pagination={false}
            locale={{ emptyText: t('requests.empty') }}
            scroll={{ x: 1080 }}
          />
        </Card>
      )}
    </Space>
  )
}

function requestColumns(
  t: ReturnType<typeof useTranslation>['t'],
): TableProps<DeploymentRequestSummary>['columns'] {
  return [
    {
      title: t('requests.columns.title'),
      dataIndex: 'title',
      render: (title: string, request) => (
        <div className={styles.titleCell}>
          <Link to={`/requests/${request.id}`}>{title}</Link>
          <div className={styles.subtle}>{request.id}</div>
        </div>
      ),
    },
    {
      title: t('requests.columns.status'),
      dataIndex: 'status',
      render: (status: string) => (
        <Tag color={requestStatusColor(status)}>
          {requestStatusLabel(status)}
        </Tag>
      ),
    },
    {
      title: t('requests.columns.classification'),
      dataIndex: 'classification',
      render: (value: string) => <SemanticTag>{value}</SemanticTag>,
    },
    { title: t('requests.columns.version'), dataIndex: 'activeVersionNumber' },
    {
      title: t('requests.columns.applications'),
      dataIndex: 'applicationCount',
    },
    {
      title: t('requests.columns.schedule'),
      dataIndex: 'scheduleState',
      render: (state: string, request) => (
        <div className={styles.scheduleCell}>
          <Tag color={state === 'Ready' ? 'green' : 'gold'}>
            {t(`requestDetail.schedule.states.${state}`)}
          </Tag>
          <span className={styles.subtle}>
            {t(`requestDetail.schedule.reasons.${request.scheduleReason}`)}
          </span>
          <span>
            {t('requests.schedule.requested')}：
            {request.scheduledFor
              ? new Date(request.scheduledFor).toLocaleString()
              : t('requestDetail.values.notSet')}
          </span>
          <span>
            {t('requests.schedule.next')}：
            {new Date(request.nextEligibleAt).toLocaleString()}
          </span>
        </div>
      ),
    },
    {
      title: t('requests.columns.updatedAt'),
      dataIndex: 'updatedAt',
      render: (value: string) => new Date(value).toLocaleString(),
    },
  ]
}

import {
  Alert,
  Button,
  Card,
  Result,
  Space,
  Spin,
  Table,
  Tabs,
  Typography,
} from 'antd'
import type { TableProps } from 'antd'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  confirmApplicationOnboarding,
  useDryRunApplicationOnboarding,
  useGetCatalogApplication,
  useGetCatalogApplicationStatus,
  useListVisibleCatalogApplications,
} from '@/generated/api'
import type { CatalogApplication } from '@/generated/model'
import { RuntimeTopologyPanel } from '@/features/runtime'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { ThemedLink } from '@/shared/link'

import styles from './ApplicationsPage.module.css'

export function ApplicationsPage() {
  const { t } = useTranslation()
  const applications = useListVisibleCatalogApplications()
  const columns: TableProps<CatalogApplication>['columns'] = [
    {
      title: t('applications.columns.name'),
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
      render: (name: string, application) => (
        <ThemedLink to={`/applications/${application.id}`}>{name}</ThemedLink>
      ),
    },
    {
      title: t('applications.columns.argocd'),
      dataIndex: 'argocdApplicationName',
      key: 'argocdApplicationName',
      ellipsis: true,
    },
    {
      title: t('applications.columns.project'),
      dataIndex: 'argocdProject',
      key: 'argocdProject',
      ellipsis: true,
    },
    {
      title: t('applications.columns.revision'),
      dataIndex: 'sourceTargetRevision',
      key: 'sourceTargetRevision',
      ellipsis: true,
    },
  ]

  if (applications.isError || applications.data?.status !== 200) {
    return (
      <Alert
        type="error"
        showIcon
        title={t('applications.error.title')}
        description={t('applications.error.description')}
      />
    )
  }

  return (
    <Space orientation="vertical" size="large" className={styles.pageSection}>
      <div>
        <Typography.Title level={2}>{t('applications.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('applications.description')}
        </Typography.Paragraph>
      </div>
      <Card>
        <Table<CatalogApplication>
          rowKey="id"
          columns={columns}
          dataSource={
            applications.data?.status === 200 ? applications.data.data.data : []
          }
          loading={applications.isPending}
          pagination={false}
          locale={{ emptyText: t('applications.empty') }}
        />
      </Card>
    </Space>
  )
}

export function ApplicationDetailPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const { applicationId } = useParams()
  const application = useGetCatalogApplication(applicationId ?? '', {
    query: { enabled: Boolean(applicationId) },
  })
  const status = useGetCatalogApplicationStatus(applicationId ?? '', {
    query: { enabled: Boolean(applicationId) },
  })
  const csrfToken = browserCookie('releasehub_csrf')
  const dryRun = useDryRunApplicationOnboarding({
    fetch: { headers: csrfToken ? { 'X-CSRF-Token': csrfToken } : {} },
  })

  if (
    !applicationId ||
    application.isError ||
    application.data?.status !== 200
  ) {
    return (
      <Result
        status="404"
        title={t('applicationDetail.notFound.title')}
        subTitle={t('applicationDetail.notFound.description')}
        extra={<Link to="/applications">{t('applicationDetail.back')}</Link>}
      />
    )
  }
  if (application.isPending || status.isPending) {
    return (
      <main className={styles.centered}>
        <Spin size="large" description={t('applicationDetail.loading')} />
      </main>
    )
  }
  if (status.isError || status.data?.status !== 200) {
    return (
      <Alert
        type="error"
        showIcon
        title={t('applicationDetail.error.title')}
        description={t('applicationDetail.error.description')}
      />
    )
  }

  const value = application.data.data.data
  const runtime = status.data.data.data
  return (
    <Space orientation="vertical" size="large" className={styles.pageSection}>
      <div>
        <Link to="/applications">{t('applicationDetail.back')}</Link>
        <Typography.Title level={2}>{value.name}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {value.argocdNamespace}/{value.argocdApplicationName}
        </Typography.Paragraph>
      </div>
      <Tabs
        items={[
          {
            key: 'overview',
            label: t('applicationDetail.tabs.overview'),
            children: (
              <Space
                orientation="vertical"
                size="large"
                className={styles.pageSection}
              >
                <Card title={t('applicationDetail.runtime.title')}>
                  <Table
                    size="small"
                    pagination={false}
                    showHeader={false}
                    rowKey="key"
                    columns={[
                      { dataIndex: 'label', key: 'label' },
                      { dataIndex: 'value', key: 'value' },
                    ]}
                    dataSource={[
                      {
                        key: 'sync',
                        label: t('applicationDetail.runtime.sync'),
                        value: runtime.syncStatus || '-',
                      },
                      {
                        key: 'health',
                        label: t('applicationDetail.runtime.health'),
                        value: runtime.healthStatus || '-',
                      },
                      {
                        key: 'operation',
                        label: t('applicationDetail.runtime.operation'),
                        value: runtime.operationPhase || '-',
                      },
                      {
                        key: 'revision',
                        label: t('applicationDetail.runtime.revision'),
                        value: runtime.resolvedRevision || '-',
                      },
                      {
                        key: 'onboarding',
                        label: t('applicationDetail.runtime.onboarding'),
                        value:
                          runtime.onboardingStatus ||
                          t('applicationDetail.runtime.notStarted'),
                      },
                      {
                        key: 'automated',
                        label: t('applicationDetail.runtime.automatedSync'),
                        value: runtime.automatedSync
                          ? t('common.enabled')
                          : t('common.disabled'),
                      },
                    ]}
                  />
                </Card>
                <Card title={t('applicationDetail.onboarding.title')}>
                  <Space>
                    <Button
                      type="primary"
                      loading={dryRun.isPending}
                      disabled={!csrfToken}
                      onClick={() =>
                        dryRun.mutate(
                          { applicationId },
                          {
                            onSuccess: (response) => {
                              if (response.status === 200) {
                                void status.refetch()
                                feedback.success(
                                  t('applicationDetail.onboarding.success'),
                                )
                              }
                            },
                            onError: () =>
                              feedback.error(
                                t('applicationDetail.onboarding.error'),
                              ),
                          },
                        )
                      }
                    >
                      {t('applicationDetail.onboarding.dryRun')}
                    </Button>
                    {runtime.onboardingStatus === 'AwaitingConfirmation' &&
                      runtime.onboardingVersion > 0 && (
                        <Button
                          disabled={!csrfToken}
                          onClick={() => {
                            void confirmApplicationOnboarding(
                              applicationId,
                              { expectedVersion: runtime.onboardingVersion },
                              {
                                headers: {
                                  'X-CSRF-Token': csrfToken ?? '',
                                  'Idempotency-Key': crypto.randomUUID(),
                                },
                              },
                            )
                              .then((response) => {
                                void status.refetch()
                                if (response.status === 202) {
                                  feedback.info(
                                    t('applicationDetail.onboarding.pending'),
                                  )
                                  return
                                }
                                feedback.success(
                                  t('applicationDetail.onboarding.success'),
                                )
                              })
                              .catch(() =>
                                feedback.error(
                                  t('applicationDetail.onboarding.error'),
                                ),
                              )
                          }}
                        >
                          {t('applicationDetail.onboarding.confirm')}
                        </Button>
                      )}
                  </Space>
                </Card>
                {runtime.driftReasons.length > 0 && (
                  <Alert
                    type="warning"
                    showIcon
                    title={t('applicationDetail.drift.title')}
                    description={runtime.driftReasons.join(', ')}
                  />
                )}
              </Space>
            ),
          },
          {
            key: 'topology',
            label: t('applicationDetail.tabs.topology'),
            children: <RuntimeTopologyPanel applicationId={applicationId} />,
          },
        ]}
      />
    </Space>
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}

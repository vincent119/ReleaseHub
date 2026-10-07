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
import { useState } from 'react'
import { useParams } from 'react-router'
import { useTranslation } from 'react-i18next'

import {
  confirmApplicationOnboarding,
  useDryRunApplicationOnboarding,
  useGetCatalogApplication,
  useGetCatalogApplicationStatus,
  useGetCatalogResourceTree,
  useListVisibleCatalogApplications,
} from '@/generated/api'
import type { CatalogApplication } from '@/generated/model'
import { RuntimeTopologyPanel } from '@/features/runtime'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { ThemedLink } from '@/shared/link'
import {
  ResourceScopeSelector,
  resolveResourceScope,
  type ResourceScopeValue,
} from '@/shared/scope'

import styles from './ApplicationsPage.module.css'

export function ApplicationsPage() {
  const { t } = useTranslation()
  const applications = useListVisibleCatalogApplications()
  const resources = useGetCatalogResourceTree()
  const [choice, setChoice] = useState<ResourceScopeValue>()
  const [scopeInvalidated, setScopeInvalidated] = useState(false)
  const organizations =
    resources.data?.status === 200 ? resources.data.data.data : []
  const catalogAvailable =
    !resources.isPending && !resources.isError && resources.data?.status === 200
  const catalogFailed =
    resources.isError ||
    Boolean(resources.data && resources.data.status !== 200)
  const resolved = resolveResourceScope(organizations, choice).value
  // 只有成功的 Catalog 能確認範圍失效，暫時失敗不應擴大目前篩選。
  if (
    catalogAvailable &&
    choice &&
    ((choice.organizationId !== undefined &&
      choice.organizationId !== resolved.organizationId) ||
      (choice.projectId !== undefined &&
        choice.projectId !== resolved.projectId) ||
      (choice.environmentId !== undefined &&
        choice.environmentId !== resolved.environmentId))
  ) {
    setChoice(undefined)
    setScopeInvalidated(true)
  }
  const hasFilter = Boolean(
    choice?.organizationId || choice?.projectId || choice?.environmentId,
  )
  // 單組織控制項的預設 ID 不代表使用者已要求篩選，也不能以 tree 取代授權清單。
  const visibleApplications =
    applications.data?.status === 200 ? applications.data.data.data : []
  const filteredApplications = visibleApplications.filter(
    (application) =>
      (!choice?.organizationId ||
        application.organizationId === choice.organizationId) &&
      (!choice?.projectId || application.projectId === choice.projectId) &&
      (!choice?.environmentId ||
        application.environmentId === choice.environmentId),
  )
  const applicationsFailed =
    applications.isError ||
    Boolean(applications.data && applications.data.status !== 200)
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

  return (
    <Space orientation="vertical" size="large" className={styles.pageSection}>
      <div>
        <Typography.Title level={2}>{t('applications.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('applications.description')}
        </Typography.Paragraph>
      </div>
      <section
        className={styles.filters}
        aria-label={t('applications.scope.label')}
      >
        <ResourceScopeSelector
          organizations={organizations}
          value={choice}
          environmentMode="optional"
          loading={resources.isPending}
          disabled={!catalogAvailable}
          onChange={(value) => {
            setChoice(value)
            setScopeInvalidated(false)
          }}
        />
        <div className={styles.filterActions}>
          <Typography.Text type="secondary">
            {t('applications.scope.hint')}
          </Typography.Text>
          <Button
            className={styles.filterAction}
            disabled={!hasFilter}
            onClick={() => {
              setChoice(undefined)
              setScopeInvalidated(false)
            }}
          >
            {t('applications.scope.clear')}
          </Button>
        </div>
      </section>
      {catalogFailed && (
        <Alert
          type="warning"
          showIcon
          title={t('applications.scope.unavailable')}
          description={
            !applicationsFailed && !applications.isPending
              ? t(
                  hasFilter
                    ? 'applications.scope.retained'
                    : 'applications.scope.listAvailable',
                )
              : undefined
          }
          action={
            <Button
              className={styles.filterAction}
              loading={resources.isFetching}
              onClick={() => void resources.refetch()}
            >
              {t('applications.scope.retry')}
            </Button>
          }
        />
      )}
      {scopeInvalidated && (
        <Alert
          type="warning"
          showIcon
          title={t('applications.scope.invalidated')}
        />
      )}
      {applicationsFailed ? (
        <Alert
          type="error"
          showIcon
          title={t('applications.error.title')}
          description={t('applications.error.description')}
        />
      ) : (
        <Card>
          <Table<CatalogApplication>
            rowKey="id"
            columns={columns}
            dataSource={filteredApplications}
            loading={applications.isPending}
            pagination={false}
            locale={{
              emptyText: t(
                applications.isPending
                  ? 'applications.loading'
                  : hasFilter && visibleApplications.length > 0
                    ? 'applications.scope.empty'
                    : 'applications.empty',
              ),
            }}
          />
        </Card>
      )}
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
        extra={
          <ThemedLink to="/applications">
            {t('applicationDetail.back')}
          </ThemedLink>
        }
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
        <ThemedLink to="/applications">
          {t('applicationDetail.back')}
        </ThemedLink>
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
            children: (
              <RuntimeTopologyPanel
                applicationId={applicationId}
                applicationName={value.name}
              />
            ),
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

import {
  AppstoreOutlined,
  ApartmentOutlined,
  DeploymentUnitOutlined,
  FileDoneOutlined,
  NodeIndexOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import {
  Alert,
  Button,
  Card,
  Flex,
  Layout,
  Menu,
  Result,
  Space,
  Spin,
  Table,
  Typography,
  message,
} from 'antd'
import type { TableProps } from 'antd'
import { Link, NavLink, Navigate, Route, Routes, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'

import { PreferenceControls } from '@/features/preferences'
import { CandidatesPage } from '@/features/candidates'
import { ResourcesPage } from '@/features/resources'
import { AccessPage } from '@/features/access'
import { WorkflowsPage } from '@/features/workflows'
import { PlansPage } from '@/features/plans'
import { NotificationCenter } from '@/features/notifications'
import { RequestDetailPage, RequestsPage } from '@/features/requests'
import {
  confirmApplicationOnboarding,
  useDryRunApplicationOnboarding,
  useGetAuthSession,
  useGetCatalogApplication,
  useGetCatalogApplicationStatus,
  useListVisibleCatalogApplications,
} from '@/generated/api'
import type { CatalogApplication } from '@/generated/model'

import styles from './App.module.css'

export function App() {
  return (
    <Routes>
      <Route path="*" element={<ApplicationShell />} />
    </Routes>
  )
}

function ApplicationShell() {
  const { t } = useTranslation()
  const session = useGetAuthSession()

  if (session.isPending) {
    return <LoadingPage label={t('session.loading')} />
  }
  if (session.isError || session.data?.status !== 200) {
    return <SignInPage />
  }

  const username = session.data.data.data.username

  return (
    <Layout className={styles.layout}>
      <Layout.Sider breakpoint="lg" collapsedWidth="0" className={styles.sider}>
        <div className={styles.brand}>{t('app.title')}</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={navigationItems(t)}
        />
      </Layout.Sider>
      <Layout>
        <Layout.Header className={styles.header}>
          <Typography.Text strong>{t('app.title')}</Typography.Text>
          <Flex align="center" gap="middle">
            <Typography.Text type="secondary">{username}</Typography.Text>
            <NotificationCenter />
            <PreferenceControls />
          </Flex>
        </Layout.Header>
        <Layout.Content className={styles.content}>
          <Routes>
            <Route index element={<OverviewPage />} />
            <Route path="resources" element={<ResourcesPage />} />
            <Route path="candidates" element={<CandidatesPage />} />
            <Route path="applications" element={<ApplicationsPage />} />
            <Route
              path="applications/:applicationId"
              element={<ApplicationDetailPage />}
            />
            <Route path="access" element={<AccessPage />} />
            <Route path="workflows" element={<WorkflowsPage />} />
            <Route path="plans" element={<PlansPage />} />
            <Route path="requests" element={<RequestsPage />} />
            <Route path="requests/:requestId" element={<RequestDetailPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Layout.Content>
      </Layout>
    </Layout>
  )
}

function navigationItems(t: ReturnType<typeof useTranslation>['t']) {
  return [
    {
      key: '/',
      icon: <AppstoreOutlined />,
      label: <NavLink to="/">{t('navigation.overview')}</NavLink>,
    },
    {
      key: '/resources',
      icon: <DeploymentUnitOutlined />,
      label: <NavLink to="/resources">{t('navigation.resources')}</NavLink>,
    },
    {
      key: '/candidates',
      icon: <DeploymentUnitOutlined />,
      label: <NavLink to="/candidates">{t('navigation.candidates')}</NavLink>,
    },
    {
      key: '/applications',
      icon: <SafetyCertificateOutlined />,
      label: (
        <NavLink to="/applications">{t('navigation.applications')}</NavLink>
      ),
    },
    {
      key: '/requests',
      icon: <FileDoneOutlined />,
      label: <NavLink to="/requests">{t('navigation.requests')}</NavLink>,
    },
    {
      key: '/workflows',
      icon: <ApartmentOutlined />,
      label: <NavLink to="/workflows">{t('navigation.workflows')}</NavLink>,
    },
    {
      key: '/plans',
      icon: <NodeIndexOutlined />,
      label: <NavLink to="/plans">{t('navigation.plans')}</NavLink>,
    },
    {
      key: '/access',
      icon: <TeamOutlined />,
      label: <NavLink to="/access">{t('navigation.access')}</NavLink>,
    },
  ]
}

function OverviewPage() {
  const { t } = useTranslation()
  return (
    <Space direction="vertical" size="large" className={styles.pageSection}>
      <div>
        <Typography.Title level={2}>{t('overview.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('overview.description')}
        </Typography.Paragraph>
      </div>
      <Alert
        type="info"
        showIcon
        message={t('overview.apiBoundary.title')}
        description={t('overview.apiBoundary.description')}
      />
    </Space>
  )
}

function ApplicationsPage() {
  const { t } = useTranslation()
  const applications = useListVisibleCatalogApplications()
  const columns: TableProps<CatalogApplication>['columns'] = [
    {
      title: t('applications.columns.name'),
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
      render: (name: string, application) => (
        <Link to={`/applications/${application.id}`}>{name}</Link>
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
        message={t('applications.error.title')}
        description={t('applications.error.description')}
      />
    )
  }

  return (
    <Space direction="vertical" size="large" className={styles.pageSection}>
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

function ApplicationDetailPage() {
  const { t } = useTranslation()
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
    return <LoadingPage label={t('applicationDetail.loading')} />
  }
  if (status.isError || status.data?.status !== 200) {
    return (
      <Alert
        type="error"
        showIcon
        message={t('applicationDetail.error.title')}
        description={t('applicationDetail.error.description')}
      />
    )
  }

  const value = application.data.data.data
  const runtime = status.data.data.data
  return (
    <Space direction="vertical" size="large" className={styles.pageSection}>
      <div>
        <Link to="/applications">{t('applicationDetail.back')}</Link>
        <Typography.Title level={2}>{value.name}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {value.argocdNamespace}/{value.argocdApplicationName}
        </Typography.Paragraph>
      </div>
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
                      message.success(t('applicationDetail.onboarding.success'))
                    }
                  },
                  onError: () =>
                    message.error(t('applicationDetail.onboarding.error')),
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
                        message.info(t('applicationDetail.onboarding.pending'))
                        return
                      }
                      message.success(t('applicationDetail.onboarding.success'))
                    })
                    .catch(() =>
                      message.error(t('applicationDetail.onboarding.error')),
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
          message={t('applicationDetail.drift.title')}
          description={runtime.driftReasons.join(', ')}
        />
      )}
    </Space>
  )
}

function browserCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((value) => value.startsWith(`${name}=`))
    ?.slice(name.length + 1)
}

function LoadingPage({ label }: { label: string }) {
  return (
    <main className={styles.centered}>
      <Spin size="large" tip={label} />
    </main>
  )
}

function SignInPage() {
  const { t } = useTranslation()
  return (
    <main className={styles.centered}>
      <Result
        status="403"
        title={t('session.signInRequired.title')}
        subTitle={t('session.signInRequired.description')}
        extra={
          <Button type="primary" href="/api/v1/auth/login">
            {t('session.signInRequired.action')}
          </Button>
        }
      />
    </main>
  )
}

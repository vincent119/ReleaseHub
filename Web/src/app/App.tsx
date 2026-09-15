import { MenuFoldOutlined, MenuUnfoldOutlined } from '@ant-design/icons'
import {
  Alert,
  Button,
  Card,
  Flex,
  Form,
  Input,
  Layout,
  Menu,
  Result,
  Space,
  Spin,
  Table,
  Tooltip,
  Typography,
} from 'antd'
import { useState } from 'react'
import type { TableProps } from 'antd'
import {
  Link,
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from 'react-router'
import { useTranslation } from 'react-i18next'

import { AccountMenu } from '@/features/account'
import { CandidatesPage } from '@/features/candidates'
import { ResourcesPage } from '@/features/resources'
import { AccessPage } from '@/features/access'
import { WorkflowsPage } from '@/features/workflows'
import { PlansPage } from '@/features/plans'
import { NotificationCenter } from '@/features/notifications'
import { RequestDetailPage, RequestsPage } from '@/features/requests'
import { useThemePreference } from '@/shared/theme/useThemePreference'
import {
  confirmApplicationOnboarding,
  changeLocalPassword,
  loginLocal,
  useDryRunApplicationOnboarding,
  useGetAuthSession,
  useGetCatalogApplication,
  useGetCatalogApplicationStatus,
  useGetSystemStatus,
  useListVisibleCatalogApplications,
} from '@/generated/api'
import type { CatalogApplication } from '@/generated/model'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { useSessionExpiryCheck } from '@/shared/auth/sessionExpiry'
import releaseHubMark from '@/assets/releasehub-mark.svg'

import styles from './App.module.css'
import { navigationItems } from './navigationItems'
import { useSidebarPreference } from './useSidebarPreference'

export function App() {
  return (
    <Routes>
      <Route path="*" element={<ApplicationShell />} />
    </Routes>
  )
}

function ApplicationShell() {
  const { t } = useTranslation()
  const location = useLocation()
  const { resolvedTheme } = useThemePreference()
  const { collapsed: desktopCollapsed, setCollapsed: setDesktopCollapsed } =
    useSidebarPreference()
  const [belowLg, setBelowLg] = useState(
    () => window.matchMedia('(max-width: 991.98px)').matches,
  )
  const session = useGetAuthSession()
  useSessionExpiryCheck(
    session.data?.status === 200
      ? session.data.data.data.idleExpiresAt
      : undefined,
    session.data?.status === 200
      ? session.data.data.data.absoluteExpiresAt
      : undefined,
  )
  const sidebarCollapsed = belowLg || desktopCollapsed

  if (session.isPending) {
    return <LoadingPage label={t('session.loading')} />
  }
  if (session.isError || session.data?.status !== 200) {
    return <SignInPage />
  }

  const { userId, username, passwordChangeAvailable } = session.data.data.data

  if (session.data.data.data.mustChangePassword) {
    return <ChangePasswordPage />
  }

  return (
    <Layout className={styles.layout}>
      <Layout.Sider
        breakpoint="lg"
        width={200}
        collapsed={sidebarCollapsed}
        collapsedWidth={belowLg ? 0 : 80}
        trigger={null}
        className={styles.sider}
        onBreakpoint={setBelowLg}
      >
        <div
          className={`${styles.brand} ${sidebarCollapsed ? styles.brandCollapsed : ''}`}
        >
          <img
            className={styles.brandMark}
            src={releaseHubMark}
            alt={t('app.title')}
          />
          {!sidebarCollapsed && (
            <span aria-hidden="true">{t('app.title')}</span>
          )}
        </div>
        <Menu
          theme={resolvedTheme}
          mode="inline"
          inlineCollapsed={sidebarCollapsed}
          selectedKeys={[selectedNavigationKey(location.pathname)]}
          items={navigationItems(t)}
        />
        {!belowLg && (
          <div className={styles.sidebarControl}>
            <Tooltip
              placement="right"
              title={
                desktopCollapsed
                  ? t('app.sidebar.expand')
                  : t('app.sidebar.collapse')
              }
            >
              <Button
                type="text"
                block
                className={styles.sidebarControlButton}
                icon={
                  desktopCollapsed ? (
                    <MenuUnfoldOutlined />
                  ) : (
                    <MenuFoldOutlined />
                  )
                }
                aria-label={
                  desktopCollapsed
                    ? t('app.sidebar.expand')
                    : t('app.sidebar.collapse')
                }
                onClick={() => setDesktopCollapsed((collapsed) => !collapsed)}
              >
                {!desktopCollapsed && t('app.sidebar.collapse')}
              </Button>
            </Tooltip>
          </div>
        )}
      </Layout.Sider>
      <Layout>
        <Layout.Header className={styles.header}>
          <Typography.Text strong>{t('app.title')}</Typography.Text>
          <Flex align="center" gap="small">
            <NotificationCenter />
            <AccountMenu
              userId={userId}
              username={username}
              passwordChangeAvailable={passwordChangeAvailable}
            />
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

function selectedNavigationKey(pathname: string) {
  const [segment] = pathname.split('/').filter(Boolean)
  return segment ? `/${segment}` : '/'
}

function OverviewPage() {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" size="large" className={styles.pageSection}>
      <div>
        <Typography.Title level={2}>{t('overview.title')}</Typography.Title>
        <Typography.Paragraph type="secondary">
          {t('overview.description')}
        </Typography.Paragraph>
      </div>
      <Alert
        type="info"
        showIcon
        title={t('overview.apiBoundary.title')}
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

function ApplicationDetailPage() {
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
    return <LoadingPage label={t('applicationDetail.loading')} />
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
                    feedback.error(t('applicationDetail.onboarding.error')),
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
                        feedback.info(t('applicationDetail.onboarding.pending'))
                        return
                      }
                      feedback.success(
                        t('applicationDetail.onboarding.success'),
                      )
                    })
                    .catch(() =>
                      feedback.error(t('applicationDetail.onboarding.error')),
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
      <Spin size="large" description={label} />
    </main>
  )
}

export function SignInPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [submitting, setSubmitting] = useState(false)
  const systemStatus = useGetSystemStatus()
  const oidcEnabled =
    systemStatus.data?.status === 200 && systemStatus.data.data.data.oidcEnabled

  async function submit(values: { username: string; password: string }) {
    setSubmitting(true)
    try {
      const response = await loginLocal(values)
      if (response.status !== 200) throw new Error('login rejected')
      window.location.reload()
    } catch {
      feedback.error(t('session.local.error'))
      setSubmitting(false)
    }
  }

  return (
    <main className={styles.authPage}>
      <div className={styles.authShell}>
        <section className={styles.authBrand} aria-label={t('app.title')}>
          <div className={styles.authBrandHeader}>
            <img
              className={styles.authBrandMark}
              src={releaseHubMark}
              alt={t('session.brand.logoAlt')}
            />
            <Typography.Text className={styles.authBrandName} aria-hidden>
              {t('app.title')}
            </Typography.Text>
          </div>
          <div className={styles.authBrandContent}>
            <Typography.Text className={styles.authEyebrow}>
              {t('session.brand.eyebrow')}
            </Typography.Text>
            <Typography.Title level={1} className={styles.authBrandTitle}>
              {t('session.brand.title')}
            </Typography.Title>
            <Typography.Paragraph className={styles.authBrandDescription}>
              {t('session.brand.description')}
            </Typography.Paragraph>
          </div>
          <div className={styles.authFlow} aria-hidden="true">
            <span className={styles.authFlowLine} />
            <span
              className={`${styles.authFlowNode} ${styles.authFlowNodeOne}`}
            />
            <span
              className={`${styles.authFlowNode} ${styles.authFlowNodeTwo}`}
            />
            <span
              className={`${styles.authFlowNode} ${styles.authFlowNodeThree}`}
            />
          </div>
        </section>
        <section
          className={styles.authPanel}
          aria-labelledby="releasehub-sign-in-title"
        >
          <div className={styles.authFormContainer}>
            <Typography.Text className={styles.authFormEyebrow}>
              {t('session.signInRequired.eyebrow')}
            </Typography.Text>
            <Typography.Title
              level={2}
              id="releasehub-sign-in-title"
              className={styles.authFormTitle}
            >
              {t('session.signInRequired.title')}
            </Typography.Title>
            <Typography.Paragraph
              type="secondary"
              className={styles.authFormDescription}
            >
              {t('session.signInRequired.description')}
            </Typography.Paragraph>
            <Form
              className={styles.authForm}
              layout="vertical"
              onFinish={submit}
            >
              <Form.Item
                name="username"
                label={t('session.local.username')}
                rules={[{ required: true }]}
              >
                <Input autoComplete="username" />
              </Form.Item>
              <Form.Item
                name="password"
                label={t('session.local.password')}
                rules={[{ required: true }]}
              >
                <Input.Password autoComplete="current-password" />
              </Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                loading={submitting}
                block
              >
                {t('session.local.signIn')}
              </Button>
            </Form>
            {oidcEnabled && (
              <div className={styles.enterpriseSignIn}>
                <div className={styles.authDivider}>
                  <span>{t('session.signInRequired.alternative')}</span>
                </div>
                <Button href="/api/v1/auth/login" block>
                  {t('session.signInRequired.action')}
                </Button>
              </div>
            )}
          </div>
        </section>
      </div>
    </main>
  )
}

export function ChangePasswordPage() {
  const { t } = useTranslation()
  const feedback = useFeedback()
  const [submitting, setSubmitting] = useState(false)

  async function submit(values: {
    currentPassword: string
    newPassword: string
  }) {
    setSubmitting(true)
    try {
      const response = await changeLocalPassword(values, {
        headers: { 'X-CSRF-Token': browserCookie('releasehub_csrf') ?? '' },
      })
      if (response.status !== 204) throw new Error('password change rejected')
      window.location.reload()
    } catch {
      feedback.error(t('session.passwordChange.error'))
      setSubmitting(false)
    }
  }

  return (
    <main className={styles.centered}>
      <Card
        title={t('session.passwordChange.title')}
        className={styles.authCard}
      >
        <Space
          orientation="vertical"
          size="middle"
          className={styles.fullWidth}
        >
          <Alert
            type="warning"
            title={t('session.passwordChange.required')}
            showIcon
          />
          <Form layout="vertical" onFinish={submit}>
            <Form.Item
              name="currentPassword"
              label={t('session.passwordChange.current')}
              rules={[{ required: true }]}
            >
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            <Form.Item
              name="newPassword"
              label={t('session.passwordChange.new')}
              rules={[{ required: true, min: 8 }]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={submitting} block>
              {t('session.passwordChange.action')}
            </Button>
          </Form>
        </Space>
      </Card>
    </main>
  )
}

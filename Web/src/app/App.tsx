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
  Space,
  Spin,
  Tooltip,
  Typography,
} from 'antd'
import { lazy, Suspense, useState } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'

import { AccountMenu } from '@/features/account'
import { NotificationCenter } from '@/features/notifications'
import { useThemePreference } from '@/shared/theme/useThemePreference'
import {
  changeLocalPassword,
  getGetAuditCapabilitiesQueryKey,
  loginLocal,
  useGetAuthSession,
  useGetAuditCapabilities,
  useGetSystemStatus,
} from '@/generated/api'
import { useFeedback } from '@/shared/feedback/useFeedback'
import { useSessionExpiryCheck } from '@/shared/auth/sessionExpiry'
import releaseHubMark from '@/assets/releasehub-mark.svg'

import styles from './App.module.css'
import { navigationItems } from './navigationItems'
import { useSidebarPreference } from './useSidebarPreference'

const AccessPage = lazy(() =>
  import('@/features/access').then(({ AccessPage }) => ({
    default: AccessPage,
  })),
)
const AuditPage = lazy(() =>
  import('@/features/audit').then(({ AuditPage }) => ({
    default: AuditPage,
  })),
)
const ApplicationDetailPage = lazy(() =>
  import('@/features/applications').then(({ ApplicationDetailPage }) => ({
    default: ApplicationDetailPage,
  })),
)
const ApplicationsPage = lazy(() =>
  import('@/features/applications').then(({ ApplicationsPage }) => ({
    default: ApplicationsPage,
  })),
)
const CandidatesPage = lazy(() =>
  import('@/features/candidates').then(({ CandidatesPage }) => ({
    default: CandidatesPage,
  })),
)
const PlansPage = lazy(() =>
  import('@/features/plans').then(({ PlansPage }) => ({
    default: PlansPage,
  })),
)
const RequestDetailPage = lazy(() =>
  import('@/features/requests').then(({ RequestDetailPage }) => ({
    default: RequestDetailPage,
  })),
)
const RequestsPage = lazy(() =>
  import('@/features/requests').then(({ RequestsPage }) => ({
    default: RequestsPage,
  })),
)
const ResourcesPage = lazy(() =>
  import('@/features/resources').then(({ ResourcesPage }) => ({
    default: ResourcesPage,
  })),
)
const WorkflowsPage = lazy(() =>
  import('@/features/workflows').then(({ WorkflowsPage }) => ({
    default: WorkflowsPage,
  })),
)

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
  const sessionUserID =
    session.data?.status === 200 ? session.data.data.data.userId : undefined
  const auditCapabilities = useGetAuditCapabilities({
    query: {
      enabled: Boolean(sessionUserID),
      queryKey: [...getGetAuditCapabilitiesQueryKey(), sessionUserID],
    },
  })
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
          items={navigationItems(
            t,
            auditCapabilities.data?.status === 200 &&
              auditCapabilities.data.data.data.visible,
          )}
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
          <Suspense fallback={<RouteLoadingPage />}>
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
              <Route
                path="audit"
                element={<AuditPage principalID={userId} />}
              />
              <Route path="workflows" element={<WorkflowsPage />} />
              <Route path="plans" element={<PlansPage />} />
              <Route path="requests" element={<RequestsPage />} />
              <Route
                path="requests/:requestId"
                element={<RequestDetailPage />}
              />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
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

function RouteLoadingPage() {
  const { t } = useTranslation()
  return (
    <div className={styles.routeLoading}>
      <Spin size="large" description={t('session.loading')} />
    </div>
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

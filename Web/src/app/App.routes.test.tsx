import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router'

import i18n from '@/shared/i18n/config'

import { App } from './App'
import { AppProviders } from './providers/AppProviders'

const api = vi.hoisted(() => ({
  session: vi.fn(),
  auditCapabilities: vi.fn(),
}))
const workflowGate = vi.hoisted(() => {
  let releasePromise: () => void = () => undefined
  const ready = new Promise<void>((resolve) => {
    releasePromise = resolve
  })
  return {
    ready,
    release: () => releasePromise(),
  }
})

vi.mock('@/generated/api', () => ({
  changeLocalPassword: vi.fn(),
  getGetAuditCapabilitiesQueryKey: () => ['/api/v1/audit/capabilities'],
  loginLocal: vi.fn(),
  useGetAuthSession: api.session,
  useGetAuditCapabilities: api.auditCapabilities,
  useGetSystemStatus: vi.fn(),
}))
vi.mock('@/shared/auth/sessionExpiry', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/shared/auth/sessionExpiry')>()),
  useSessionExpiryCheck: vi.fn(),
}))
vi.mock('@/features/account', () => ({
  AccountMenu: () => <div data-testid="account-menu" />,
}))
vi.mock('@/features/notifications', () => ({
  NotificationCenter: () => <div data-testid="notification-center" />,
}))
vi.mock('@/features/access', () => ({
  AccessPage: () => <div data-testid="access-route" />,
}))
vi.mock('@/features/audit', () => ({
  AuditPage: () => <div data-testid="audit-route" />,
}))
vi.mock('@/features/applications', () => ({
  ApplicationDetailPage: () => <div data-testid="application-detail-route" />,
  ApplicationsPage: () => <div data-testid="applications-route" />,
}))
vi.mock('@/features/candidates', () => ({
  CandidatesPage: () => <div data-testid="candidates-route" />,
}))
vi.mock('@/features/plans', () => ({
  PlansPage: () => <div data-testid="plans-route" />,
}))
vi.mock('@/features/requests', () => ({
  RequestDetailPage: () => <div data-testid="request-detail-route" />,
  RequestsPage: () => <div data-testid="requests-route" />,
}))
vi.mock('@/features/resources', () => ({
  ResourcesPage: () => <div data-testid="resources-route" />,
}))
vi.mock('@/features/workflows', async () => {
  await workflowGate.ready
  return {
    WorkflowsPage: () => <div data-testid="workflows-route" />,
  }
})

describe('Application lazy routes', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('zh-TW')
    api.session.mockReturnValue({
      data: {
        status: 200,
        data: {
          data: {
            absoluteExpiresAt: '2026-09-16T00:00:00Z',
            idleExpiresAt: '2026-09-15T23:00:00Z',
            mustChangePassword: false,
            passwordChangeAvailable: true,
            userId: 'user-1',
            username: 'admin',
          },
        },
      },
      isError: false,
      isPending: false,
    })
    api.auditCapabilities.mockReturnValue({
      data: { status: 200, data: { data: { visible: true, scopeRoots: [] } } },
      isError: false,
      isPending: false,
    })
  })

  afterEach(cleanup)

  it('keeps the app shell visible while a lazy route resolves', async () => {
    renderRoute('/workflows')

    expect(screen.getAllByText('ReleaseHub')).not.toHaveLength(0)
    expect(screen.getByText('正在確認登入狀態')).toBeInTheDocument()

    workflowGate.release()
    expect(await screen.findByTestId('workflows-route')).toBeInTheDocument()
  })

  it.each([
    ['/resources', 'resources-route'],
    ['/candidates', 'candidates-route'],
    ['/applications', 'applications-route'],
    ['/applications/application-1', 'application-detail-route'],
    ['/access', 'access-route'],
    ['/audit', 'audit-route'],
    ['/workflows', 'workflows-route'],
    ['/plans', 'plans-route'],
    ['/requests', 'requests-route'],
    ['/requests/request-1', 'request-detail-route'],
  ])('loads %s through its lazy route boundary', async (path, testId) => {
    renderRoute(path)

    expect(await screen.findByTestId(testId)).toBeInTheDocument()
    expect(screen.getByTestId('account-menu')).toBeInTheDocument()
  })
})

function renderRoute(path: string) {
  return render(
    <AppProviders>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </AppProviders>,
  )
}

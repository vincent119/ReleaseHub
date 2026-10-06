import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type {
  DeploymentExecution,
  DeploymentRequestVersion,
} from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { RequestDetailPage } from './RequestDetailPage'

const api = vi.hoisted(() => ({
  request: vi.fn(),
  execution: vi.fn(),
  workflows: vi.fn(),
  plans: vi.fn(),
  history: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useGetDeploymentRequest: api.request,
  useGetDeploymentExecution: api.execution,
  useListReleaseWorkflows: api.workflows,
  useListDeploymentPlans: api.plans,
  useListDeploymentHistory: api.history,
  decideDeploymentReview: vi.fn(),
  reassignDeploymentReview: vi.fn(),
  transitionDeploymentRequest: vi.fn(),
  retryDeploymentRequest: vi.fn(),
  terminateDeploymentRequest: vi.fn(),
  unlockDeploymentRequest: vi.fn(),
  updateDeploymentRequestVersionMetadata: vi.fn(),
}))

vi.mock('@/features/runtime', () => ({
  RuntimeTopologyPanel: ({ applicationId }: { applicationId: string }) => (
    <div data-testid="runtime-topology">{applicationId}</div>
  ),
}))

describe('RequestDetailPage', () => {
  afterEach(cleanup)
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    const request = requestFixture()
    api.request.mockReturnValue(
      queryResult({ status: 200, data: { data: request } }),
    )
    api.execution.mockReturnValue(queryResult(undefined))
    api.workflows.mockReturnValue(
      queryResult({ status: 200, data: { data: [] } }),
    )
    api.plans.mockReturnValue(queryResult({ status: 200, data: { data: [] } }))
    api.history.mockReturnValue(
      queryResult({ status: 200, data: { data: [] } }),
    )
  })

  it('shows backend status while hiding operations without capabilities', () => {
    renderPage()
    expect(screen.getAllByText('PendingReview').length).toBeGreaterThan(0)
    expect(screen.getByText('Request Version').parentElement).toHaveTextContent(
      'Request Version2',
    )
    expect(screen.queryByRole('button', { name: '核准申請' })).toBeNull()
    expect(screen.queryByRole('button', { name: '編輯選填資訊' })).toBeNull()
    expect(screen.getByText('等待下一個維護時段')).toBeInTheDocument()
    expect(screen.getByText('等待排程')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'payment-api' })).toHaveAttribute(
      'href',
      '#request-application-snapshots',
    )
  })

  it('renders request actions only when the backend grants capabilities', () => {
    const request = requestFixture()
    request.capabilities = [
      'deployment_request.update',
      'deployment_request.review',
      'deployment_request.reassign',
    ]
    api.request.mockReturnValue(
      queryResult({ status: 200, data: { data: request } }),
    )

    renderPage()

    expect(
      screen.getByRole('button', { name: '編輯選填資訊' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '核准申請' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '重新指派' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '前往審核' })).toHaveAttribute(
      'href',
      '#request-review',
    )
  })

  it('shows Request and Execution separately before detailed evidence', () => {
    const request = requestFixture()
    request.status = 'Deploying'
    request.workflowStateKey = 'deploying'
    request.executionId = 'execution-1'
    request.applications.push({
      ...request.applications[0],
      id: 'snapshot-2',
      applicationId: 'application-2',
      applicationKey: 'worker',
      order: 1,
    })
    api.request.mockReturnValue(
      queryResult({ status: 200, data: { data: request } }),
    )
    api.execution.mockReturnValue(
      queryResult({
        status: 200,
        data: {
          data: {
            ...revisionMismatchExecution(),
            status: 'Running',
            nodes: [
              { ...revisionMismatchExecution().nodes[0], status: 'Succeeded' },
              {
                ...revisionMismatchExecution().nodes[0],
                id: 'node-2',
                applicationId: 'application-2',
                nodeKey: 'worker',
                status: 'Syncing',
                errorCode: '',
                errorMessage: '',
              },
            ],
          },
        },
      }),
    )

    const { container } = renderPage()
    const summary = screen.getByText('目前部署進度').closest('.ant-card')
    expect(summary).toHaveTextContent(/Request 狀態：\s*Deploying/)
    expect(summary).toHaveTextContent('Execution 狀態Running')
    expect(summary).toHaveTextContent('受影響 Applications（2）')
    expect(summary).toHaveTextContent('workerSyncing')
    expect(summary).not.toHaveTextContent('Failed')
    expect(screen.getByRole('link', { name: 'worker' })).toHaveAttribute(
      'href',
      '#request-application-evidence',
    )
    expect(
      container.querySelector('#request-application-evidence'),
    ).toHaveTextContent('worker')
    expect(
      screen.getByRole('button', { name: '查看 worker 的資源拓撲' }),
    ).toBeInTheDocument()
  })

  it('distinguishes an absent Execution from loading and query failure', () => {
    const request = requestFixture()
    request.executionId = 'execution-1'
    api.request.mockReturnValue(
      queryResult({ status: 200, data: { data: request } }),
    )
    api.execution.mockReturnValue({
      ...queryResult(undefined),
      isPending: true,
    })
    const { rerender } = renderPage()
    expect(
      screen.getByText('正在取得 Deployment execution…'),
    ).toBeInTheDocument()

    api.execution.mockReturnValue({ ...queryResult(undefined), isError: true })
    rerender(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter initialEntries={['/requests/request-1']}>
          <Routes>
            <Route
              path="/requests/:requestId"
              element={<RequestDetailPage />}
            />
          </Routes>
        </MemoryRouter>
      </I18nextProvider>,
    )
    expect(
      screen.getAllByText('無法取得 Deployment execution。').length,
    ).toBeGreaterThan(0)
  })

  it.each(['Succeeded', 'Failed', 'PartialFailed'] as const)(
    'keeps terminal Request and Execution status separate for %s',
    (status) => {
      const request = requestFixture()
      request.status = status
      request.executionId = 'execution-1'
      api.request.mockReturnValue(
        queryResult({ status: 200, data: { data: request } }),
      )
      api.execution.mockReturnValue(
        queryResult({
          status: 200,
          data: { data: { ...revisionMismatchExecution(), status } },
        }),
      )

      renderPage()
      const summary = screen.getByText('目前部署進度').closest('.ant-card')
      const sections = summary?.querySelectorAll('section')
      const label = status === 'PartialFailed' ? 'Partial Failed' : status
      expect(sections?.[0]).toHaveTextContent(`Request 狀態： ${label}`)
      expect(sections?.[1]).toHaveTextContent(`Execution 狀態${label}`)
    },
  )

  it('defers rendering cumulative diff until the Application is expanded', () => {
    renderPage()
    expect(screen.queryByText(/apps\/Deployment/)).toBeNull()
    fireEvent.click(screen.getAllByRole('button', { name: /payment-api/ })[0])
    fireEvent.click(screen.getByRole('button', { name: '展開累積 diff' }))
    expect(screen.getByText(/"group": "apps"/)).toBeInTheDocument()
    expect(screen.getByText(/"kind": "Deployment"/)).toBeInTheDocument()
  })

  it('explains successful deployment evidence when the reviewed revision differs', () => {
    const request = requestFixture()
    request.executionId = 'execution-1'
    api.request.mockReturnValue(
      queryResult({ status: 200, data: { data: request } }),
    )
    api.execution.mockReturnValue(
      queryResult({ status: 200, data: { data: revisionMismatchExecution() } }),
    )

    renderPage()

    expect(
      screen.getByText('實際部署已成功，但審核版本不一致'),
    ).toBeInTheDocument()
    expect(screen.getByText('實際部署 revision')).toBeInTheDocument()
    expect(screen.getByText('newer-commit')).toBeInTheDocument()
    expect(screen.getByText('審核 revision')).toBeInTheDocument()
    expect(screen.getAllByText('commit-b').length).toBeGreaterThan(0)
  })
})

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter initialEntries={['/requests/request-1']}>
        <Routes>
          <Route path="/requests/:requestId" element={<RequestDetailPage />} />
        </Routes>
      </MemoryRouter>
    </I18nextProvider>,
  )
}

function queryResult(data: unknown) {
  return {
    data,
    isPending: false,
    isError: false,
    isFetching: false,
    refetch: vi.fn().mockResolvedValue(undefined),
  }
}

function requestFixture(): DeploymentRequestVersion {
  return {
    id: 'version-1',
    requestId: 'request-1',
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    versionNumber: 2,
    status: 'PendingReview',
    classification: 'Standard',
    fingerprint: 'fingerprint',
    workflowVersionId: 'workflow-version-1',
    planVersionId: 'plan-version-1',
    title: 'Deploy payment',
    changeDescription: '',
    issueUrl: '',
    scheduleState: 'Waiting',
    nextEligibleAt: '2026-09-17T01:00:00Z',
    scheduleReason: 'MaintenanceWindow',
    lockVersion: 4,
    workflowStateKey: 'review',
    capabilities: [],
    createdAt: '2026-09-07T00:00:00Z',
    reviews: [
      {
        id: 'review-1',
        stateKey: 'review',
        stageNumber: 1,
        policyType: 'AnyApprover',
        requiredApprovals: 1,
        allowSelfReview: false,
        status: 'Pending',
      },
    ],
    applications: [
      {
        id: 'snapshot-1',
        applicationId: 'application-1',
        applicationKey: 'payment-api',
        liveRevision: 'commit-a',
        targetRevision: 'commit-b',
        targetRevisions: ['commit-b'],
        manifestHash: 'manifest-hash',
        diffHash: 'diff-hash',
        order: 0,
        images: [],
        diff: {
          resources: [
            {
              group: 'apps',
              kind: 'Deployment',
              namespace: 'payment',
              name: 'payment-api',
            },
          ],
        },
      },
    ],
  }
}

function revisionMismatchExecution(): DeploymentExecution {
  return {
    id: 'execution-1',
    requestVersionId: 'version-1',
    planVersionId: 'plan-version-1',
    attempt: 1,
    status: 'Failed',
    triggerKind: 'Workflow',
    lockVersion: 1,
    nodes: [
      {
        id: 'node-1',
        applicationId: 'application-1',
        nodeKey: 'payment-api',
        status: 'Failed',
        operationId: 'operation-1',
        syncStatus: 'Synced',
        healthStatus: 'Healthy',
        actualRevision: 'newer-commit',
        actualImages: [],
        errorCode: 'target_revision_mismatch',
        errorMessage: 'Argo CD completed a different target revision',
      },
    ],
    createdAt: '2026-09-21T00:00:00Z',
  }
}

import { fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter, Route, Routes } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentRequestVersion } from '@/generated/model'
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

describe('RequestDetailPage', () => {
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
  })

  it('defers rendering cumulative diff until the Application is expanded', () => {
    renderPage()
    expect(screen.queryByText(/apps\/Deployment/)).toBeNull()
    fireEvent.click(screen.getAllByRole('button', { name: /payment-api/ })[0])
    fireEvent.click(screen.getByRole('button', { name: '展開累積 diff' }))
    expect(screen.getByText(/"group": "apps"/)).toBeInTheDocument()
    expect(screen.getByText(/"kind": "Deployment"/)).toBeInTheDocument()
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

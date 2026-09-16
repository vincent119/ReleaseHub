import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentRequestVersion } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { ReviewDecisionPanel } from './ReviewDecisionPanel'

const api = vi.hoisted(() => ({ decide: vi.fn(), reassign: vi.fn() }))

vi.mock('@/generated/api', () => ({
  decideDeploymentReview: api.decide,
  reassignDeploymentReview: api.reassign,
}))

vi.mock('./ReviewReassignmentModal', () => ({
  ReviewReassignmentModal: ({
    open,
    onSubmit,
  }: {
    open: boolean
    onSubmit: (value: {
      userIds: string[]
      roleIds: string[]
      reason: string
    }) => void
  }) =>
    open ? (
      <button
        type="button"
        onClick={() =>
          onSubmit({
            userIds: ['019c1230-0000-7000-8000-000000000009'],
            roleIds: [],
            reason: 'Original reviewer is unavailable',
          })
        }
      >
        送出重新指派測試
      </button>
    ) : null,
}))

describe('ReviewDecisionPanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.decide.mockReset()
    api.reassign.mockReset()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    vi.stubGlobal('crypto', { randomUUID: () => 'decision-key' })
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
  })

  afterEach(() => cleanup())

  it('submits an approval with the current lock version', async () => {
    const request = deploymentRequest()
    api.decide.mockResolvedValue({
      status: 200,
      data: { data: { ...request, status: 'Approved' } },
    })
    const onUpdated = vi.fn()

    renderPanel(request, onUpdated)
    fireEvent.click(screen.getByRole('button', { name: '核准申請' }))

    await waitFor(() =>
      expect(api.decide).toHaveBeenCalledWith(
        request.requestId,
        request.id,
        request.reviews[0].id,
        { decision: 'Approve', reason: undefined, expectedVersion: 4 },
        {
          headers: {
            'X-CSRF-Token': 'csrf-token',
            'Idempotency-Key': 'decision-key',
          },
        },
      ),
    )
    expect(onUpdated).toHaveBeenCalledOnce()
  })

  it('requires a reason before submitting a rejection', async () => {
    const request = deploymentRequest()
    api.decide.mockResolvedValue({ status: 200, data: { data: request } })

    renderPanel(request, vi.fn())
    fireEvent.click(screen.getByRole('button', { name: '拒絕申請' }))
    fireEvent.click(screen.getByRole('button', { name: '確認拒絕' }))

    expect(await screen.findByText('拒絕時必須填寫理由。')).toBeInTheDocument()
    expect(api.decide).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('拒絕理由'), {
      target: { value: 'Production verification failed' },
    })
    fireEvent.click(screen.getByRole('button', { name: '確認拒絕' }))

    await waitFor(() =>
      expect(api.decide).toHaveBeenCalledWith(
        request.requestId,
        request.id,
        request.reviews[0].id,
        {
          decision: 'Reject',
          reason: 'Production verification failed',
          expectedVersion: 4,
        },
        expect.any(Object),
      ),
    )
  })

  it('only exposes reassignment when the parent grants the capability', () => {
    const request = deploymentRequest()
    const { rerender } = renderPanel(request, vi.fn())
    expect(screen.queryByRole('button', { name: '重新指派' })).toBeNull()

    rerender(
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <ReviewDecisionPanel
            request={request}
            onUpdated={vi.fn()}
            canReassign
          />
        </I18nextProvider>
      </AntdApp>,
    )

    expect(screen.getByRole('button', { name: '重新指派' })).toBeVisible()
  })

  it('submits replacement assignees and a required reason', async () => {
    const request = deploymentRequest()
    const replacementID = '019c1230-0000-7000-8000-000000000009'
    api.reassign.mockResolvedValue({ status: 200, data: { data: request } })
    render(
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <ReviewDecisionPanel
            request={request}
            onUpdated={vi.fn()}
            canReassign
          />
        </I18nextProvider>
      </AntdApp>,
    )

    fireEvent.click(screen.getByRole('button', { name: '重新指派' }))
    fireEvent.click(screen.getByRole('button', { name: '送出重新指派測試' }))

    await waitFor(() =>
      expect(api.reassign).toHaveBeenCalledWith(
        request.requestId,
        request.id,
        request.reviews[0].id,
        {
          userIds: [replacementID],
          roleIds: [],
          reason: 'Original reviewer is unavailable',
          expectedVersion: request.lockVersion,
        },
        expect.any(Object),
      ),
    )
  })
})

function renderPanel(
  request: DeploymentRequestVersion,
  onUpdated: (value: DeploymentRequestVersion) => void,
) {
  return render(
    <AntdApp>
      <I18nextProvider i18n={i18n}>
        <ReviewDecisionPanel request={request} onUpdated={onUpdated} />
      </I18nextProvider>
    </AntdApp>,
  )
}

function deploymentRequest(): DeploymentRequestVersion {
  return {
    id: 'version-1',
    requestId: 'request-1',
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    versionNumber: 1,
    status: 'PendingReview',
    classification: 'Standard',
    fingerprint: 'fingerprint',
    workflowVersionId: 'workflow-version-1',
    planVersionId: 'plan-version-1',
    title: 'Release payment',
    changeDescription: '',
    issueUrl: '',
    scheduleState: 'Ready',
    nextEligibleAt: '2026-09-07T00:00:00Z',
    scheduleReason: 'Ready',
    lockVersion: 4,
    applications: [],
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
    capabilities: [],
    createdAt: '2026-09-07T00:00:00Z',
  }
}

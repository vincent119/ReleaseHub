import { fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { CandidatesPage } from './CandidatesPage'

const api = vi.hoisted(() => ({
  refetch: vi.fn(),
  useCandidates: vi.fn(),
  useScopes: vi.fn(),
  assign: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useListArgoCDCandidates: api.useCandidates,
  useListArgoCDCandidateAssignmentScopes: api.useScopes,
  assignArgoCDCandidate: api.assign,
}))

describe('CandidatesPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.refetch.mockReset()
    api.assign.mockReset()
    api.useCandidates.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: '019c1230-0000-7000-8000-000000000001',
              argocdNamespace: 'argocd',
              argocdApplicationName: 'payment-production',
              argocdProject: 'payment',
              sources: [
                {
                  repositoryUrl: 'https://git.example.com/manifests.git',
                  targetRevision: 'main',
                  path: 'production/payment',
                },
              ],
              destinationServer: 'https://kubernetes.default.svc',
              destinationName: '',
              destinationNamespace: 'payment',
              automatedSync: true,
              syncStatus: 'Synced',
              healthStatus: 'Healthy',
              operationPhase: 'Succeeded',
              resolvedRevision: 'commit-a',
              resourceVersion: '42',
              firstSeenAt: '2026-09-02T00:00:00Z',
              lastSeenAt: '2026-09-02T00:00:30Z',
              version: 3,
            },
          ],
          meta: { requestId: 'request-1', timestamp: '2026-09-02T00:00:30Z' },
        },
      },
    })
    api.useScopes.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          data: [
            {
              organizationId: '019c1230-0000-7000-8000-000000000002',
              organizationName: 'Tenant A',
              projectId: '019c1230-0000-7000-8000-000000000003',
              projectName: 'Payment',
              environmentId: '019c1230-0000-7000-8000-000000000004',
              environmentName: 'Production',
            },
          ],
          meta: { requestId: 'request-2', timestamp: '2026-09-02T00:00:30Z' },
        },
      },
    })
  })

  it('opens the assignment form with candidate-controlled source data', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <CandidatesPage />
      </I18nextProvider>,
    )

    fireEvent.click(screen.getByRole('button', { name: /指\s*派/ }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByDisplayValue('payment-production')).toBeInTheDocument()
    expect(screen.getByText('GitOps source')).toBeInTheDocument()
  })
})

import { fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'
import linkStyles from '@/shared/link/ThemedLink.module.css'
import tagStyles from '@/shared/tag/SemanticTag.module.css'

import { RequestsPage } from './RequestsPage'

const api = vi.hoisted(() => ({ resources: vi.fn(), requests: vi.fn() }))

vi.mock('@/generated/api', () => ({
  useGetCatalogResourceTree: api.resources,
  useListDeploymentRequests: api.requests,
}))

vi.mock('./components/RequestScopeSelector', () => ({
  RequestScopeSelector: ({
    onChange,
  }: {
    onChange: (scope: object) => void
  }) => (
    <button
      type="button"
      onClick={() =>
        onChange({
          organizationId: 'organization-1',
          projectId: 'project-1',
          environmentId: 'environment-1',
        })
      }
    >
      選擇測試 Environment
    </button>
  ),
}))

describe('RequestsPage schedule projection', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.resources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: [] } },
    })
    api.requests.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: 'request-1',
              organizationId: 'organization-1',
              projectId: 'project-1',
              environmentId: 'environment-1',
              status: 'Approved',
              classification: 'Standard',
              activeVersionNumber: 2,
              title: 'Deploy payment',
              applicationCount: 2,
              scheduledFor: '2026-09-17T00:00:00Z',
              scheduleState: 'Waiting',
              nextEligibleAt: '2026-09-17T02:00:00Z',
              scheduleReason: 'Blackout',
              updatedAt: '2026-09-16T00:00:00Z',
            },
          ],
        },
      },
    })
  })

  it('renders the Server-provided reason and next eligible time', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <MemoryRouter>
          <RequestsPage />
        </MemoryRouter>
      </I18nextProvider>,
    )
    fireEvent.click(
      screen.getByRole('button', { name: '選擇測試 Environment' }),
    )

    expect(screen.getByText('目前位於禁止時段')).toBeInTheDocument()
    expect(screen.getByText('等待排程')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Deploy payment' })).toHaveClass(
      linkStyles.link,
    )
    expect(screen.getByText('Standard')).toHaveClass(tagStyles.neutral)
    expect(
      screen.getByText(
        `下一個可部署時間：${new Date('2026-09-17T02:00:00Z').toLocaleString()}`,
      ),
    ).toBeInTheDocument()
  })
})

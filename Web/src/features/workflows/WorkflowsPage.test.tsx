import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { WorkflowsPage } from './WorkflowsPage'

const api = vi.hoisted(() => ({
  lifecycle: vi.fn(),
  refetch: vi.fn(),
  useWorkflows: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeReleaseWorkflowVersionLifecycle: api.lifecycle,
  createReleaseWorkflow: vi.fn(),
  createReleaseWorkflowVersion: vi.fn(),
  useListReleaseWorkflows: api.useWorkflows,
}))

describe('WorkflowsPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.lifecycle.mockReset()
    api.refetch.mockReset()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.useWorkflows.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: 'workflow-1',
              name: 'Production approval',
              description: 'Production workflow',
              active: true,
              versions: [
                {
                  id: 'version-1',
                  workflowId: 'workflow-1',
                  versionNumber: 1,
                  lifecycle: 'Draft',
                  lockVersion: 3,
                  createdAt: '2026-09-07T00:00:00Z',
                  document: {
                    initialState: 'review',
                    states: [{ key: 'review', name: 'Review', type: 'Review' }],
                    transitions: [],
                  },
                },
              ],
            },
          ],
        },
      },
    })
  })

  it('publishes a draft version with optimistic lock data', async () => {
    api.lifecycle.mockResolvedValue({ status: 200 })

    render(
      <I18nextProvider i18n={i18n}>
        <WorkflowsPage />
      </I18nextProvider>,
    )
    expect(screen.queryByRole('button', { name: '停用版本' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '發布版本' }))

    await waitFor(() =>
      expect(api.lifecycle).toHaveBeenCalledWith(
        'workflow-1',
        'version-1',
        { lifecycle: 'Published', expectedVersion: 3 },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
  })
})

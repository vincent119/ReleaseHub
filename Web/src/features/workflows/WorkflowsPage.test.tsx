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

import i18n from '@/shared/i18n/config'

import { WorkflowsPage } from './WorkflowsPage'

const api = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  lifecycle: vi.fn(),
  refetch: vi.fn(),
  useWorkflows: vi.fn(),
  useReviewOptions: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeReleaseWorkflowVersionLifecycle: api.lifecycle,
  createReleaseWorkflow: api.create,
  createReleaseWorkflowVersion: vi.fn(),
  deleteReleaseWorkflow: api.delete,
  useListReleaseWorkflows: api.useWorkflows,
  useGetReleaseWorkflowReviewOptions: api.useReviewOptions,
}))

describe('WorkflowsPage', () => {
  afterEach(cleanup)

  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.lifecycle.mockReset()
    api.create.mockReset()
    api.delete.mockReset()
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
    api.useWorkflows.mockReturnValue(workflowQuery())
    api.useReviewOptions.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: { data: { users: [], roles: [] } },
      },
    })
  })

  it('copies the selected version into an independent new Workflow draft', async () => {
    api.create.mockResolvedValue({ status: 201 })
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /複製 Workflow/ }))
    expect(screen.getByText('複製 Release Workflow')).toBeInTheDocument()
    const name = screen.getByLabelText('Workflow 名稱')
    expect(name).toHaveValue('Production approval 複本')
    fireEvent.change(name, { target: { value: 'Staging approval' } })
    fireEvent.click(screen.getByRole('button', { name: '儲存 Workflow' }))

    await waitFor(() => expect(api.create).toHaveBeenCalledOnce())
    const payload = api.create.mock.calls[0][0]
    expect(payload).toMatchObject({
      name: 'Staging approval',
      description: 'Production workflow',
      document: sourceDocument,
    })
    expect(payload.document).not.toBe(sourceDocument)
    expect(sourceDocument.states[0].name).toBe('Start')
    expect(api.refetch).toHaveBeenCalledOnce()
  })

  it('requires the full name and sends the latest version number when deleting', async () => {
    api.delete.mockResolvedValue({ status: 204 })
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /刪除 Workflow/ }))
    const confirm = screen.getByRole('button', { name: '永久刪除' })
    expect(confirm).toBeDisabled()
    fireEvent.change(screen.getByLabelText('輸入 Workflow 名稱確認刪除'), {
      target: { value: 'Production approval' },
    })
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm)

    await waitFor(() =>
      expect(api.delete).toHaveBeenCalledWith(
        'workflow-1',
        { expectedVersion: 1 },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
    expect(screen.getByText('選擇一個 Workflow 查看內容。')).toBeInTheDocument()
  })

  it('preserves the confirmation when the backend rejects deletion', async () => {
    api.delete.mockResolvedValue({ status: 409 })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: /刪除 Workflow/ }))
    const input = screen.getByLabelText('輸入 Workflow 名稱確認刪除')
    fireEvent.change(input, { target: { value: 'Production approval' } })
    fireEvent.click(screen.getByRole('button', { name: '永久刪除' }))

    await waitFor(() => expect(api.delete).toHaveBeenCalledOnce())
    expect(input).toHaveValue('Production approval')
    expect(screen.getByText('刪除 Release Workflow')).toBeInTheDocument()
    expect(api.refetch).not.toHaveBeenCalled()
  })

  it('disables deletion when a version left Draft', () => {
    api.useWorkflows.mockReturnValue(workflowQuery('Published'))
    renderPage()
    expect(screen.getByRole('button', { name: /刪除 Workflow/ })).toBeDisabled()
  })

  it('publishes a draft version with optimistic lock data', async () => {
    api.lifecycle.mockResolvedValue({ status: 200 })

    renderPage()
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

const sourceDocument = {
  initialState: 'start',
  states: [{ key: 'start', name: 'Start', type: 'Start' as const }],
  transitions: [],
}

function workflowQuery(lifecycle: 'Draft' | 'Published' = 'Draft') {
  return {
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
                lifecycle,
                lockVersion: 3,
                createdAt: '2026-09-07T00:00:00Z',
                document: sourceDocument,
              },
            ],
          },
        ],
      },
    },
  }
}

function renderPage() {
  return render(
    <AntdApp>
      <I18nextProvider i18n={i18n}>
        <WorkflowsPage />
      </I18nextProvider>
    </AntdApp>,
  )
}

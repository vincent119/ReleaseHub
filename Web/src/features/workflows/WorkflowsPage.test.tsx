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
  createVersion: vi.fn(),
  delete: vi.fn(),
  lifecycle: vi.fn(),
  refetch: vi.fn(),
  useWorkflows: vi.fn(),
  useReviewOptions: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeReleaseWorkflowVersionLifecycle: api.lifecycle,
  createReleaseWorkflow: api.create,
  createReleaseWorkflowVersion: api.createVersion,
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
    api.createVersion.mockReset()
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
    expect(
      screen.getByRole('heading', { name: '複製 Release Workflow' }),
    ).toBeInTheDocument()
    expect(document.querySelector('.ant-drawer')).toBeNull()
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

  it('opens the create editor inside the page and returns to the list', () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /建立 Workflow/ }))

    expect(
      screen.getByRole('heading', { name: '建立 Release Workflow' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('region', { name: 'Workflow 編輯工作區' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Workflow 清單')).toBeNull()
    expect(document.querySelector('.ant-drawer')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: '返回 Workflow 清單' }))
    expect(screen.getByText('Workflow 清單')).toBeInTheDocument()
    expect(screen.getAllByText('Production approval')).toHaveLength(2)
  })

  it('opens a new version in the same page workspace', async () => {
    api.createVersion.mockResolvedValue({ status: 201 })
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: '建立新版本' }))
    expect(
      screen.getByRole('heading', { name: '建立 Workflow Version' }),
    ).toBeInTheDocument()
    expect(document.querySelector('.ant-drawer')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '儲存 Workflow' }))

    await waitFor(() =>
      expect(api.createVersion).toHaveBeenCalledWith(
        'workflow-1',
        { expectedVersion: 1, document: sourceDocument },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
  })

  it('shows duplicate-name feedback when Workflow creation conflicts', async () => {
    api.create.mockResolvedValue({
      status: 409,
      data: {
        code: 'WORKFLOW_NAME_CONFLICT',
        message: 'Release workflow name already exists',
        requestId: 'request-1',
      },
    })
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /複製 Workflow/ }))
    fireEvent.change(screen.getByLabelText('Workflow 名稱'), {
      target: { value: 'Production approval' },
    })
    fireEvent.click(screen.getByRole('button', { name: '儲存 Workflow' }))

    expect(
      await screen.findByText('Workflow 名稱已存在，請使用其他名稱。'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Workflow 名稱')).toHaveValue(
      'Production approval',
    )
    expect(api.refetch).not.toHaveBeenCalled()
  })

  it('keeps the editor open when new-version creation conflicts', async () => {
    api.createVersion.mockResolvedValue({ status: 409, data: undefined })
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: '建立新版本' }))
    fireEvent.click(screen.getByRole('button', { name: '儲存 Workflow' }))

    expect(
      await screen.findByText(
        'Workflow 版本已變更或已有 Draft，請重新整理後再試。',
      ),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: '建立 Workflow Version' }),
    ).toBeInTheDocument()
    expect(api.refetch).not.toHaveBeenCalled()
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

  it('keeps lifecycle metadata in the detail header and supports keyboard list selection', () => {
    const query = workflowQuery()
    const staging = structuredClone(query.data.data.data[0])
    staging.id = 'workflow-2'
    staging.name = 'Staging approval'
    staging.description = 'Staging workflow'
    staging.versions[0].id = 'version-2'
    staging.versions[0].workflowId = staging.id
    query.data.data.data.push(staging)
    api.useWorkflows.mockReturnValue(query)
    renderPage()

    const stagingItem = screen.getByRole('button', {
      name: /Staging approval Staging workflow/,
    })
    fireEvent.keyDown(stagingItem, { key: 'Enter' })

    expect(stagingItem).toHaveAttribute('aria-current', 'page')
    expect(screen.getByText('Draft').closest('.ant-card-head')).not.toBeNull()
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

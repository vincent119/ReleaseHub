import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntdApp } from 'antd'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentPlanDocument } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { PlansPage } from './PlansPage'

const api = vi.hoisted(() => ({
  createPlan: vi.fn(),
  createVersion: vi.fn(),
  lifecycle: vi.fn(),
  refetch: vi.fn(),
  usePlans: vi.fn(),
  useResources: vi.fn(),
  useWorkflows: vi.fn(),
}))
const feedback = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  changeDeploymentPlanVersionLifecycle: api.lifecycle,
  createDeploymentPlan: api.createPlan,
  createDeploymentPlanVersion: api.createVersion,
  getGetAuthSessionQueryKey: () => ['/api/v1/auth/session'],
  useGetCatalogResourceTree: api.useResources,
  useListDeploymentPlans: api.usePlans,
  useListReleaseWorkflows: api.useWorkflows,
}))

vi.mock('@/shared/feedback/useFeedback', () => ({
  useFeedback: () => feedback,
}))

vi.mock('./components/PlanScopeSelector', () => ({
  PlanScopeSelector: ({
    onChange,
  }: {
    onChange: (value: { organizationId: string; projectId: string }) => void
  }) => (
    <button
      type="button"
      onClick={() =>
        onChange({ organizationId: 'organization-1', projectId: 'project-1' })
      }
    >
      選擇測試 Project
    </button>
  ),
}))

vi.mock('./components/PlanList', () => ({
  PlanList: () => <div>Plan 清單測試</div>,
}))

vi.mock('./components/PlanDetail', () => ({
  PlanDetail: ({
    onNewVersion,
    onLifecycle,
  }: {
    onNewVersion: () => void
    onLifecycle: (lifecycle: 'Published') => void
  }) => (
    <>
      <button type="button" onClick={onNewVersion}>
        建立測試版本
      </button>
      <button type="button" onClick={() => onLifecycle('Published')}>
        發布測試版本
      </button>
    </>
  ),
}))

vi.mock('./components/DeploymentBindingPanel', () => ({
  DeploymentBindingPanel: () => null,
}))

vi.mock('./components/PlanEditorWorkspace', () => ({
  PlanEditorWorkspace: ({
    title,
    onClose,
    onSubmit,
  }: {
    title: string
    onClose: () => void
    onSubmit: (value: {
      ownerKind: 'project'
      name: string
      description: string
      document: DeploymentPlanDocument
    }) => void
  }) => (
    <section data-testid="plan-editor-workspace">
      <h2>{title}</h2>
      <button type="button" onClick={onClose}>
        關閉測試編輯器
      </button>
      <button
        type="button"
        onClick={() =>
          onSubmit({
            ownerKind: 'project',
            name: 'Production plan',
            description: 'Production flow',
            document: planDocument,
          })
        }
      >
        提交測試 Plan
      </button>
    </section>
  ),
}))

describe('PlansPage editor mode', () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.createPlan.mockReset()
    api.createVersion.mockReset()
    api.lifecycle.mockReset()
    api.refetch.mockReset()
    feedback.error.mockReset()
    feedback.success.mockReset()
    queryClient.clear()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    api.useResources.mockReturnValue({
      isError: false,
      data: { status: 200, data: { data: [] } },
    })
    api.useWorkflows.mockReturnValue({
      isError: false,
      data: { status: 200, data: { data: [] } },
    })
    api.usePlans.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: { status: 200, data: { data: [deploymentPlan] } },
    })
  })

  afterEach(cleanup)

  it('switches to an inline workspace and cancel returns with scope preserved', () => {
    renderPage()
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: /建立 Plan/ }))

    expect(screen.getByTestId('plan-editor-workspace')).toBeInTheDocument()
    expect(screen.queryByText('Plan 清單測試')).not.toBeInTheDocument()
    expect(document.querySelector('.ant-drawer')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '關閉測試編輯器' }))

    expect(screen.getByText('Plan 清單測試')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /建立 Plan/ })).toBeEnabled()
  })

  it('preserves the create payload, CSRF header, success close, and refetch', async () => {
    api.createPlan.mockResolvedValue({ status: 201 })
    api.refetch.mockResolvedValue(undefined)
    renderPage()
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: /建立 Plan/ }))
    fireEvent.click(screen.getByRole('button', { name: '提交測試 Plan' }))

    await waitFor(() =>
      expect(api.createPlan).toHaveBeenCalledWith(
        {
          ownerKind: 'project',
          ownerProjectId: 'project-1',
          name: 'Production plan',
          description: 'Production flow',
          document: planDocument,
        },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    await waitFor(() => expect(api.refetch).toHaveBeenCalledOnce())
    expect(screen.getByText('Plan 清單測試')).toBeInTheDocument()
  })

  it('opens the same workspace and preserves the new-version payload', async () => {
    api.createVersion.mockResolvedValue({ status: 201 })
    api.refetch.mockResolvedValue(undefined)
    renderPage()
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: '建立測試版本' }))

    expect(
      screen.getByRole('heading', {
        name: '建立 Deployment Plan Version',
      }),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '提交測試 Plan' }))

    await waitFor(() =>
      expect(api.createVersion).toHaveBeenCalledWith(
        'plan-1',
        {
          expectedVersion: 1,
          document: planDocument,
        },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    await waitFor(() => expect(api.refetch).toHaveBeenCalledOnce())
  })

  it('explains a create name conflict and preserves the editor', async () => {
    api.createPlan.mockResolvedValue({ status: 409 })
    renderPage(queryClient)
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: /建立 Plan/ }))
    fireEvent.click(screen.getByRole('button', { name: '提交測試 Plan' }))

    await waitFor(() =>
      expect(feedback.error).toHaveBeenCalledWith(
        '目前作用域已存在相同名稱的 Plan，請使用其他名稱。',
      ),
    )
    expect(screen.getByTestId('plan-editor-workspace')).toBeInTheDocument()
    expect(api.refetch).not.toHaveBeenCalled()
  })

  it('distinguishes a new-version conflict from a create conflict', async () => {
    api.createVersion.mockResolvedValue({ status: 409 })
    renderPage(queryClient)
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: '建立測試版本' }))
    fireEvent.click(screen.getByRole('button', { name: '提交測試 Plan' }))

    await waitFor(() =>
      expect(feedback.error).toHaveBeenCalledWith(
        'Plan 版本已變更或已有 Draft，請重新整理後再試。',
      ),
    )
    expect(screen.getByTestId('plan-editor-workspace')).toBeInTheDocument()
  })

  it.each([
    [404, 'Plan 不存在，或你沒有操作權限。'],
    [422, '目前 Plan 版本狀態不允許這項操作。'],
    [500, '無法儲存 Deployment Plan。'],
  ])(
    'maps lifecycle status %s to an actionable message',
    async (status, message) => {
      api.lifecycle.mockResolvedValue({ status })
      renderPage(queryClient)
      selectScope()
      fireEvent.click(screen.getByRole('button', { name: '發布測試版本' }))

      await waitFor(() => expect(feedback.error).toHaveBeenCalledWith(message))
      expect(api.refetch).not.toHaveBeenCalled()
    },
  )

  it('revalidates Auth Session after a 401 response', async () => {
    api.createPlan.mockResolvedValue({ status: 401 })
    const invalidateQueries = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockResolvedValue()
    renderPage(queryClient)
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: /建立 Plan/ }))
    fireEvent.click(screen.getByRole('button', { name: '提交測試 Plan' }))

    await waitFor(() =>
      expect(invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['/api/v1/auth/session'],
        exact: true,
      }),
    )
    expect(feedback.error).toHaveBeenCalledWith('登入狀態已失效，請重新登入。')
  })

  it.each([404, 500])(
    'shows an unavailable state and blocks creation when the list returns %s',
    async (status) => {
      api.usePlans.mockReturnValue({
        isPending: false,
        isError: false,
        refetch: api.refetch,
        data: { status, data: {} },
      })
      renderPage(queryClient)
      selectScope()

      expect(
        await screen.findByText('無法載入 Deployment Plan 或資源範圍。'),
      ).toBeInTheDocument()
      expect(screen.queryByText('Plan 清單測試')).not.toBeInTheDocument()
      expect(screen.getByRole('button', { name: /建立 Plan/ })).toBeDisabled()
    },
  )

  it('revalidates Auth Session and blocks creation when the list returns 401', async () => {
    api.usePlans.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: { status: 401, data: {} },
    })
    const invalidateQueries = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockResolvedValue()
    renderPage(queryClient)
    selectScope()

    await waitFor(() =>
      expect(invalidateQueries).toHaveBeenCalledWith({
        queryKey: ['/api/v1/auth/session'],
        exact: true,
      }),
    )
    expect(screen.getByRole('button', { name: /建立 Plan/ })).toBeDisabled()
  })
})

function renderPage(queryClient = new QueryClient()) {
  return render(
    <QueryClientProvider client={queryClient}>
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <PlansPage />
        </I18nextProvider>
      </AntdApp>
    </QueryClientProvider>,
  )
}

function selectScope() {
  fireEvent.click(screen.getByRole('button', { name: '選擇測試 Project' }))
}

const planDocument: DeploymentPlanDocument = {
  nodes: [
    {
      key: 'application_1',
      applicationKey: 'application_1',
      order: 0,
      successCondition: {
        syncStatuses: ['Synced'],
        healthStatuses: ['Healthy'],
        stabilizationSeconds: 0,
      },
    },
  ],
  edges: [],
}

const deploymentPlan = {
  id: 'plan-1',
  ownerKind: 'project' as const,
  ownerProjectId: 'project-1',
  name: 'Production plan',
  description: 'Production flow',
  active: true,
  versions: [
    {
      id: 'version-1',
      planId: 'plan-1',
      versionNumber: 1,
      lifecycle: 'Published' as const,
      document: planDocument,
      lockVersion: 3,
      createdAt: '2026-09-10T00:00:00Z',
    },
  ],
}

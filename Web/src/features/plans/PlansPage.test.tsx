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

import type {
  CatalogOrganizationNode,
  DeploymentPlan,
  DeploymentPlanDocument,
  DeploymentPlanVersion,
} from '@/generated/model'
import type { ResourceScopeValue } from '@/shared/scope'
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
    onChange: (value: ResourceScopeValue) => void
  }) => (
    <>
      <button
        type="button"
        onClick={() =>
          onChange({ organizationId: 'organization-1', projectId: 'project-1' })
        }
      >
        選擇測試 Project
      </button>
      <button
        type="button"
        onClick={() =>
          onChange({ organizationId: 'organization-1', projectId: 'project-2' })
        }
      >
        切換測試 Project
      </button>
      <button
        type="button"
        onClick={() => onChange({ organizationId: 'organization-1' })}
      >
        清除測試 Project
      </button>
      <button
        type="button"
        onClick={() =>
          onChange({
            organizationId: 'organization-1',
            projectId: 'project-1',
            environmentId: 'global-1',
          })
        }
      >
        選擇測試 Global
      </button>
    </>
  ),
}))

vi.mock('./components/PlanList', () => ({
  PlanList: ({
    plans,
    selected,
    loading,
    onSelect,
  }: {
    plans: DeploymentPlan[]
    selected?: string
    loading: boolean
    onSelect: (id: string) => void
  }) => (
    <div
      data-testid="plan-list"
      data-selected={selected}
      data-loading={loading}
    >
      Plan 清單測試
      {plans.map((plan) => (
        <button key={plan.id} type="button" onClick={() => onSelect(plan.id)}>
          {plan.name}
        </button>
      ))}
    </div>
  ),
}))

vi.mock('./components/PlanDetail', () => ({
  PlanDetail: ({
    onNewVersion,
    onLifecycle,
    plan,
    version,
    versionID,
    onVersion,
  }: {
    onNewVersion: () => void
    onLifecycle: (lifecycle: 'Published') => void
    plan?: DeploymentPlan
    version?: DeploymentPlanVersion
    versionID?: string
    onVersion: (id: string) => void
  }) => (
    <section
      data-testid="plan-detail"
      data-plan={plan?.id}
      data-version={version?.id}
      data-selected-version={versionID}
    >
      <button type="button" onClick={onNewVersion}>
        建立測試版本
      </button>
      <button type="button" onClick={() => onLifecycle('Published')}>
        發布測試版本
      </button>
      <button type="button" onClick={() => onVersion('version-1')}>
        選擇測試舊版本
      </button>
    </section>
  ),
}))

vi.mock('./components/DeploymentBindingPanel', () => ({
  DeploymentBindingPanel: ({
    organizationId,
    projectId,
    environmentId,
  }: {
    organizationId: string
    projectId: string
    environmentId: string
  }) => (
    <div
      data-testid="binding"
      data-organization={organizationId}
      data-project={projectId}
      data-environment={environmentId}
    />
  ),
}))

vi.mock('./components/DeploymentSchedulePanel', () => ({
  DeploymentSchedulePanel: ({ environmentId }: { environmentId: string }) => (
    <div data-testid="schedule" data-environment={environmentId} />
  ),
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
    api.usePlans.mockReset()
    api.useResources.mockReset()
    feedback.error.mockReset()
    feedback.success.mockReset()
    queryClient.clear()
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: organizations } },
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

  it('Project 足以查詢，Global 只作 binding／schedule context', () => {
    renderPage()
    expect(api.usePlans.mock.lastCall).toEqual([
      { projectId: '00000000-0000-0000-0000-000000000000' },
      { query: { enabled: false } },
    ])
    selectScope()
    expect(api.usePlans.mock.lastCall).toEqual([
      { projectId: 'project-1' },
      { query: { enabled: true } },
    ])
    expect(screen.queryByTestId('binding')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '選擇測試 Global' }))
    expect(api.usePlans.mock.lastCall?.[0]).toEqual({ projectId: 'project-1' })
    expect(screen.getByTestId('binding')).toHaveAttribute(
      'data-environment',
      'global-1',
    )
    expect(screen.getByTestId('schedule')).toHaveAttribute(
      'data-environment',
      'global-1',
    )
    fireEvent.click(screen.getByRole('button', { name: '切換測試 Project' }))
    expect(screen.queryByTestId('binding')).toBeNull()
    expect(screen.queryByTestId('schedule')).toBeNull()
  })

  it('新 Project 載入時隔離舊 Plan、版本與環境，禁止依舊結果建立', () => {
    renderPage()
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: '選擇測試 Global' }))
    expect(screen.getByTestId('plan-detail')).toHaveAttribute(
      'data-plan',
      'plan-1',
    )
    api.usePlans.mockReturnValue({
      isPending: true,
      isError: false,
      data: { status: 200, data: { data: [deploymentPlan] } },
      refetch: api.refetch,
    })
    fireEvent.click(screen.getByRole('button', { name: '切換測試 Project' }))
    expect(api.usePlans.mock.lastCall?.[0]).toEqual({ projectId: 'project-2' })
    expect(screen.getByTestId('plan-list')).toHaveAttribute(
      'data-loading',
      'true',
    )
    expect(screen.queryByRole('button', { name: 'Production plan' })).toBeNull()
    expect(screen.getByTestId('plan-detail')).not.toHaveAttribute('data-plan')
    expect(screen.getByTestId('plan-detail')).not.toHaveAttribute(
      'data-version',
    )
    expect(screen.getByRole('button', { name: /建立 Plan/ })).toBeDisabled()
    expect(screen.queryByTestId('binding')).toBeNull()
  })

  it('即使兩個 Project 皆可見共用 Plan，切換時也清除舊 Plan／version 選取', () => {
    const shared = {
      ...deploymentPlan,
      ownerKind: 'platform',
      ownerProjectId: undefined,
      versions: [
        ...deploymentPlan.versions,
        { ...deploymentPlan.versions[0], id: 'version-2', versionNumber: 2 },
      ],
    }
    api.usePlans.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: [shared] } },
      refetch: api.refetch,
    })
    renderPage()
    selectScope()
    fireEvent.click(screen.getByRole('button', { name: 'Production plan' }))
    fireEvent.click(screen.getByRole('button', { name: '選擇測試舊版本' }))
    expect(screen.getByTestId('plan-detail')).toHaveAttribute(
      'data-version',
      'version-1',
    )
    fireEvent.click(screen.getByRole('button', { name: '切換測試 Project' }))
    expect(screen.getByTestId('plan-detail')).toHaveAttribute(
      'data-version',
      'version-2',
    )
    expect(screen.getByTestId('plan-detail')).not.toHaveAttribute(
      'data-selected-version',
    )
    fireEvent.click(screen.getByRole('button', { name: '清除測試 Project' }))
    expect(screen.queryByTestId('plan-detail')).toBeNull()
    expect(api.usePlans.mock.lastCall?.[1]).toEqual({
      query: { enabled: false },
    })
  })

  it('Catalog 移除 Project 後清除選取並停用查詢與 mutation 入口', () => {
    const page = renderPage()
    selectScope()
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 200, data: { data: [] } },
    })
    page.rerender(pageElement())
    expect(api.usePlans.mock.lastCall?.[1]).toEqual({
      query: { enabled: false },
    })
    expect(screen.queryByTestId('plan-detail')).toBeNull()
    expect(screen.getByRole('button', { name: /建立 Plan/ })).toBeDisabled()
    expect(
      screen.getByText(i18n.t('plans.scope.invalidated')),
    ).toBeInTheDocument()
  })

  it('Catalog 或 Workflow 的 HTTP 錯誤不呈現為空資料', () => {
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      data: { status: 500 },
    })
    renderPage()
    expect(screen.getByText(i18n.t('plans.unavailable'))).toBeInTheDocument()
    expect(screen.queryByText(i18n.t('plans.scope.select'))).toBeNull()
    expect(api.usePlans.mock.lastCall?.[1]).toEqual({
      query: { enabled: false },
    })
  })
})

function renderPage(queryClient = new QueryClient()) {
  return render(pageElement(queryClient))
}

function pageElement(queryClient = new QueryClient()) {
  return (
    <QueryClientProvider client={queryClient}>
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <PlansPage />
        </I18nextProvider>
      </AntdApp>
    </QueryClientProvider>
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

const organizations: CatalogOrganizationNode[] = [
  {
    id: 'organization-1',
    name: '測試組織',
    version: 1,
    isDefault: true,
    canRename: false,
    canDelete: false,
    canCreateProject: false,
    projects: [
      {
        id: 'project-1',
        name: 'Project A',
        canManage: false,
        environments: [
          {
            id: 'global-1',
            name: 'Global',
            type: 'Production',
            applications: [],
          },
        ],
      },
      {
        id: 'project-2',
        name: 'Project B',
        canManage: false,
        environments: [],
      },
    ],
  },
]

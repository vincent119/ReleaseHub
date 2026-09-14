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

vi.mock('@/generated/api', () => ({
  changeDeploymentPlanVersionLifecycle: api.lifecycle,
  createDeploymentPlan: api.createPlan,
  createDeploymentPlanVersion: api.createVersion,
  useGetCatalogResourceTree: api.useResources,
  useListDeploymentPlans: api.usePlans,
  useListReleaseWorkflows: api.useWorkflows,
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
  PlanDetail: ({ onNewVersion }: { onNewVersion: () => void }) => (
    <button type="button" onClick={onNewVersion}>
      建立測試版本
    </button>
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
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.createPlan.mockReset()
    api.createVersion.mockReset()
    api.lifecycle.mockReset()
    api.refetch.mockReset()
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
})

function renderPage() {
  return render(
    <AntdApp>
      <I18nextProvider i18n={i18n}>
        <PlansPage />
      </I18nextProvider>
    </AntdApp>,
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
      lifecycle: 'Draft' as const,
      document: planDocument,
      lockVersion: 3,
      createdAt: '2026-09-10T00:00:00Z',
    },
  ],
}

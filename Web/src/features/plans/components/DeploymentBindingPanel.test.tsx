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

import type { DeploymentPlan, ReleaseWorkflow } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { DeploymentBindingPanel } from './DeploymentBindingPanel'

const api = vi.hoisted(() => ({
  bind: vi.fn(),
  refetch: vi.fn(),
  useBinding: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  bindDeploymentDefinitions: api.bind,
  useGetDeploymentBinding: api.useBinding,
}))

describe('DeploymentBindingPanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    document.cookie = 'releasehub_csrf=csrf-token; path=/'
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.bind.mockReset()
    api.refetch.mockReset()
    api.bind.mockResolvedValue({ status: 200 })
    api.useBinding.mockReturnValue({
      isPending: false,
      isError: false,
      refetch: api.refetch,
      data: { status: 200, data: { data: binding() } },
    })
  })

  afterEach(cleanup)

  it('switches published definitions with the current optimistic version', async () => {
    render(
      <AntdApp>
        <I18nextProvider i18n={i18n}>
          <DeploymentBindingPanel
            organizationId="organization-1"
            projectId="project-1"
            environmentId="environment-1"
            workflows={[workflow()]}
            plans={[plan()]}
          />
        </I18nextProvider>
      </AntdApp>,
    )
    fireEvent.click(screen.getByRole('button', { name: '儲存 Binding' }))
    await waitFor(() =>
      expect(api.bind).toHaveBeenCalledWith(
        {
          organizationId: 'organization-1',
          projectId: 'project-1',
          environmentId: 'environment-1',
          workflowVersionId: 'workflow-version-1',
          planVersionId: 'plan-version-1',
          expectedVersion: 4,
        },
        { headers: { 'X-CSRF-Token': 'csrf-token' } },
      ),
    )
    expect(api.refetch).toHaveBeenCalledOnce()
  })
})

function binding() {
  return {
    id: 'binding-1',
    organizationId: 'organization-1',
    projectId: 'project-1',
    environmentId: 'environment-1',
    workflowVersionId: 'workflow-version-1',
    planVersionId: 'plan-version-1',
    version: 4,
  }
}

function workflow(): ReleaseWorkflow {
  return {
    id: 'workflow-1',
    name: 'Production approval',
    description: '',
    active: true,
    versions: [
      {
        id: 'workflow-version-1',
        workflowId: 'workflow-1',
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 2,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          initialState: 'done',
          states: [{ key: 'done', name: 'Done', type: 'Terminal' }],
          transitions: [],
        },
      },
    ],
  }
}

function plan(): DeploymentPlan {
  return {
    id: 'plan-1',
    ownerKind: 'project',
    ownerProjectId: 'project-1',
    name: 'Production',
    description: '',
    active: true,
    versions: [
      {
        id: 'plan-version-1',
        planId: 'plan-1',
        versionNumber: 1,
        lifecycle: 'Published',
        lockVersion: 2,
        createdAt: '2026-09-07T00:00:00Z',
        document: {
          nodes: [
            {
              key: 'api',
              applicationKey: 'api',
              order: 0,
              successCondition: {
                syncStatuses: ['Synced'],
                healthStatuses: ['Healthy'],
                stabilizationSeconds: 0,
              },
            },
          ],
          edges: [],
        },
      },
    ],
  }
}

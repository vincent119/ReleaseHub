import { cleanup, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import type { ReleaseWorkflowVersion } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { WorkflowProgress } from './WorkflowPlanProgress'

describe('WorkflowProgress', () => {
  beforeEach(async () => i18n.changeLanguage('zh-TW'))
  afterEach(cleanup)

  it('presents sibling terminal states as one pending result stage', () => {
    renderProgress('deploying', 'Deploying')

    expect(screen.getByText('部署結果')).toBeInTheDocument()
    expect(screen.getByText('等待部署結果')).toBeInTheDocument()
    expect(screen.queryByText('Succeeded')).toBeNull()
    expect(screen.queryByText('Failed')).toBeNull()
  })

  it('shows only the selected successful terminal result', () => {
    renderProgress('succeeded', 'Succeeded')

    expect(screen.getByText('Succeeded')).toBeInTheDocument()
    expect(screen.queryByText('Failed')).toBeNull()
    expect(screen.queryByText('部署結果')).toBeNull()
  })

  it('shows only the selected failed terminal result', () => {
    renderProgress('failed', 'Failed')

    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.queryByText('Succeeded')).toBeNull()
    expect(screen.queryByText('部署結果')).toBeNull()
  })
})

function renderProgress(
  current: string,
  status: 'Deploying' | 'Succeeded' | 'Failed',
) {
  return render(
    <I18nextProvider i18n={i18n}>
      <WorkflowProgress
        version={workflowVersion()}
        current={current}
        requestStatus={status}
      />
    </I18nextProvider>,
  )
}

function workflowVersion(): ReleaseWorkflowVersion {
  return {
    id: 'workflow-version-1',
    workflowId: 'workflow-1',
    versionNumber: 1,
    lifecycle: 'Published',
    lockVersion: 1,
    createdAt: '2026-09-23T00:00:00Z',
    document: {
      initialState: 'pending_review',
      states: [
        { key: 'pending_review', name: 'Pending Review', type: 'Review' },
        { key: 'approved', name: 'Approved', type: 'ManualAction' },
        { key: 'deploying', name: 'Deploying', type: 'Deployment' },
        { key: 'succeeded', name: 'Succeeded', type: 'Terminal' },
        { key: 'failed', name: 'Failed', type: 'Terminal' },
      ],
      transitions: [
        transition('approve', 'pending_review', 'approved', 'ReviewSatisfied'),
        transition('deploy', 'approved', 'deploying', 'Manual'),
        transition('succeed', 'deploying', 'succeeded', 'DeploymentResult'),
        transition('fail', 'deploying', 'failed', 'DeploymentResult'),
      ],
    },
  }
}

function transition(
  key: string,
  from: string,
  to: string,
  trigger: 'ReviewSatisfied' | 'Manual' | 'DeploymentResult',
) {
  return {
    key,
    from,
    to,
    trigger,
    permission: 'deployment_request.update',
    conditions: [],
  }
}

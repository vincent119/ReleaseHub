import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { ReleaseWorkflowDocument } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { WorkflowGraphEditor } from './WorkflowGraphEditor'

vi.mock('@/shared/graph/DefinitionGraphCanvas', () => ({
  DefinitionGraphCanvas: (props: GraphCanvasMockProps) => (
    <div data-testid="workflow-canvas">
      <button
        type="button"
        onClick={() => props.onSelectNode(props.nodes[0].id)}
      >
        Select workflow node
      </button>
    </div>
  ),
}))

interface GraphCanvasMockProps {
  nodes: Array<{ id: string }>
  onSelectNode: (id: string) => void
}

describe('WorkflowGraphEditor shared graph composition', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
  })

  afterEach(cleanup)

  it('keeps the Workflow toolbar and inspector around the shared canvas', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <WorkflowGraphEditor initialDocument={document} />
      </I18nextProvider>,
    )

    expect(
      screen.getByRole('toolbar', { name: 'Workflow 結構工具列' }),
    ).toBeVisible()
    expect(screen.getByTestId('workflow-canvas')).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', { name: 'Select workflow node' }),
    )
    expect(screen.getByLabelText('設定面板')).toHaveTextContent(
      'Pending Review',
    )
    expect(screen.getByLabelText('Key')).toHaveValue('pending_review')
  })
})

const document: ReleaseWorkflowDocument = {
  initialState: 'pending_review',
  states: [
    {
      key: 'pending_review',
      name: 'Pending Review',
      type: 'Review',
      reviewPolicy: {
        type: 'AnyApprover',
        requiredApprovals: 1,
        allowSelfReview: false,
        userIds: [],
        roleIds: [],
      },
    },
    { key: 'deploying', name: 'Deploying', type: 'Deployment' },
  ],
  transitions: [
    {
      key: 'approve',
      from: 'pending_review',
      to: 'deploying',
      trigger: 'ReviewSatisfied',
      permission: 'deployment_request.deploy',
      conditions: [],
    },
  ],
}

import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentPlanDocument } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import { PlanGraphEditor } from './PlanGraphEditor'

vi.mock('@/shared/graph/DefinitionGraphCanvas', () => ({
  DefinitionGraphCanvas: (props: GraphCanvasMockProps) => (
    <div data-testid="plan-canvas">
      <button
        type="button"
        onClick={() => props.onSelectNode(props.nodes[0].id)}
      >
        Select plan node
      </button>
    </div>
  ),
}))

interface GraphCanvasMockProps {
  nodes: Array<{ id: string }>
  onSelectNode: (id: string) => void
}

describe('PlanGraphEditor shared graph composition', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
  })

  afterEach(cleanup)

  it('keeps the Plan toolbar and inspector around the shared canvas', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <PlanGraphEditor initialDocument={document} />
      </I18nextProvider>,
    )

    expect(
      screen.getByRole('toolbar', { name: 'Deployment Plan 結構工具列' }),
    ).toBeVisible()
    expect(screen.getByTestId('plan-canvas')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Select plan node' }))
    const inspector = screen.getByLabelText('設定面板')
    expect(inspector).toHaveTextContent('application_1')
    expect(
      within(inspector).getAllByDisplayValue('application_1'),
    ).toHaveLength(2)
  })
})

const document: DeploymentPlanDocument = {
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

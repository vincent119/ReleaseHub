import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DeploymentPlanDocument } from '@/generated/model'
import i18n from '@/shared/i18n/config'

import {
  PlanEditorWorkspace,
  type PlanEditorValue,
} from './PlanEditorWorkspace'

vi.mock('./PlanGraphEditor', () => ({
  PlanGraphEditor: () => <div data-testid="plan-graph-editor" />,
}))

describe('PlanEditorWorkspace', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
  })

  afterEach(cleanup)

  it('renders a page workspace without Drawer and supports both close actions', () => {
    const onClose = vi.fn()
    renderEditor({ onClose })

    expect(
      screen.getByRole('heading', { name: '建立 Deployment Plan' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Plan 基本資料')).toBeInTheDocument()
    expect(screen.getByTestId('plan-graph-editor')).toBeInTheDocument()
    expect(document.querySelector('.ant-drawer')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '返回 Plans' }))
    fireEvent.click(screen.getByRole('button', { name: /取\s*消/ }))

    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('keeps document validation and disables save for an empty graph', () => {
    renderEditor({
      initial: { ...initialValue, document: { nodes: [], edges: [] } },
    })

    expect(screen.getByText('Plan 至少需要一個 Application。')).toBeVisible()
    expect(screen.getByRole('button', { name: /儲存 Plan/ })).toBeDisabled()
  })

  it('submits the existing value contract', async () => {
    const onSubmit = vi.fn()
    renderEditor({ onSubmit })

    fireEvent.change(screen.getByLabelText('Plan 名稱'), {
      target: { value: 'Production rollout' },
    })
    const save = screen.getByRole('button', { name: /儲存 Plan/ })
    fireEvent.click(save)

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({
        ...initialValue,
        name: 'Production rollout',
      }),
    )
  })

  it('exposes the save loading state while submitting', () => {
    renderEditor({ submitting: true })

    expect(screen.getByRole('button', { name: /儲存 Plan/ })).toHaveClass(
      'ant-btn-loading',
    )
  })
})

function renderEditor({
  initial = initialValue,
  submitting = false,
  onClose = vi.fn(),
  onSubmit = vi.fn(),
}: {
  initial?: PlanEditorValue
  submitting?: boolean
  onClose?: () => void
  onSubmit?: (value: PlanEditorValue) => void
} = {}) {
  return render(
    <I18nextProvider i18n={i18n}>
      <PlanEditorWorkspace
        title="建立 Deployment Plan"
        initial={initial}
        creating
        submitting={submitting}
        onClose={onClose}
        onSubmit={onSubmit}
      />
    </I18nextProvider>,
  )
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

const initialValue: PlanEditorValue = {
  ownerKind: 'project',
  name: 'Production plan',
  description: 'Production Application 部署流程。',
  document: planDocument,
}

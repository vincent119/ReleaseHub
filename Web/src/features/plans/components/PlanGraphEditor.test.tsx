import {
  act,
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
    <div data-testid="plan-canvas" data-node-id={props.nodes[0]?.id}>
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

  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('唯讀圖形於外框配置後初始化，保留目前版本節點', () => {
    let mount: FrameRequestCallback | undefined
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      mount = callback
      return 1
    })
    render(
      <I18nextProvider i18n={i18n}>
        <PlanGraphEditor initialDocument={document} readOnly />
      </I18nextProvider>,
    )
    expect(
      screen.getByLabelText('Deployment Plan 圖形編輯區'),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('plan-canvas')).toBeNull()
    act(() => mount?.(0))
    expect(screen.getByTestId('plan-canvas')).toHaveAttribute(
      'data-node-id',
      'application_1',
    )
    expect(
      screen.getByRole('button', { name: /新增 Application/ }),
    ).toBeDisabled()
  })

  it('切換版本取消舊掛載，只初始化新版本，不保留舊節點', () => {
    const callbacks = new Map<number, FrameRequestCallback>()
    let id = 0
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callbacks.set(++id, callback)
      return id
    })
    const cancel = vi
      .spyOn(window, 'cancelAnimationFrame')
      .mockImplementation((frame) => {
        callbacks.delete(frame)
      })
    const { rerender, unmount } = render(
      <PlanGraphEditor key="v1" initialDocument={document} readOnly />,
    )
    rerender(
      <PlanGraphEditor
        key="v2"
        initialDocument={{
          nodes: [{ ...document.nodes[0], key: 'new-node' }],
          edges: [],
        }}
        readOnly
      />,
    )
    expect(cancel).toHaveBeenCalledWith(1)
    expect(callbacks.size).toBe(1)
    act(() => callbacks.get(2)?.(0))
    expect(screen.getByTestId('plan-canvas')).toHaveAttribute(
      'data-node-id',
      'new-node',
    )
    unmount()
    expect(cancel).toHaveBeenCalledWith(2)
  })

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

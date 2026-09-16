import type { ReactNode } from 'react'
import { act, cleanup, render, screen } from '@testing-library/react'
import type { Edge, Node } from '@xyflow/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { DefinitionGraphCanvas } from './DefinitionGraphCanvas'

interface CapturedFlowProps {
  children?: ReactNode
  nodesDraggable?: boolean
  nodesConnectable?: boolean
  edgesReconnectable?: boolean
  fitView?: boolean
  fitViewOptions?: unknown
  minZoom?: number
  maxZoom?: number
  onNodeClick?: (event: unknown, node: { id: string }) => void
  onEdgeClick?: (event: unknown, edge: { id: string }) => void
  onPaneClick?: () => void
}

const flow = vi.hoisted(() => ({ props: {} as CapturedFlowProps }))

afterEach(cleanup)

vi.mock('@xyflow/react', () => ({
  ReactFlow: (props: CapturedFlowProps) => {
    flow.props = props
    return <div data-testid="react-flow">{props.children}</div>
  },
  Background: (props: { gap?: number; size?: number }) => (
    <div data-testid="background" data-gap={props.gap} data-size={props.size} />
  ),
  MiniMap: (props: { className?: string }) => (
    <div data-testid="mini-map" className={props.className} />
  ),
  Controls: () => <div data-testid="controls" />,
}))

describe('DefinitionGraphCanvas', () => {
  beforeEach(() => {
    flow.props = {}
  })

  it('applies the shared viewport and editable interaction contract', () => {
    renderCanvas(false)

    expect(flow.props.fitView).toBe(true)
    expect(flow.props.fitViewOptions).toEqual({
      padding: 0.16,
      minZoom: 0.68,
      maxZoom: 1.1,
    })
    expect(flow.props.minZoom).toBe(0.35)
    expect(flow.props.maxZoom).toBe(1.5)
    expect(flow.props.nodesDraggable).toBe(true)
    expect(flow.props.nodesConnectable).toBe(true)
    expect(flow.props.edgesReconnectable).toBe(true)
    expect(screen.getByTestId('background')).toHaveAttribute('data-gap', '20')
    expect(screen.getByTestId('mini-map')).toHaveClass('feature-mini-map')
    expect(screen.getByTestId('controls')).toBeInTheDocument()
  })

  it('disables graph mutations in read-only mode', () => {
    renderCanvas(true)

    expect(flow.props.nodesDraggable).toBe(false)
    expect(flow.props.nodesConnectable).toBe(false)
    expect(flow.props.edgesReconnectable).toBe(false)
  })

  it('forwards node, edge, and pane selection by identity', () => {
    const onSelectNode = vi.fn()
    const onSelectEdge = vi.fn()
    const onClearSelection = vi.fn()
    renderCanvas(false, { onSelectNode, onSelectEdge, onClearSelection })

    act(() => flow.props.onNodeClick?.({}, { id: 'node-a' }))
    act(() => flow.props.onEdgeClick?.({}, { id: 'edge-a' }))
    act(() => flow.props.onPaneClick?.())

    expect(onSelectNode).toHaveBeenCalledWith('node-a')
    expect(onSelectEdge).toHaveBeenCalledWith('edge-a')
    expect(onClearSelection).toHaveBeenCalledOnce()
  })
})

const nodes: Node[] = [
  { id: 'node-a', position: { x: 0, y: 0 }, data: { label: 'Node A' } },
]
const edges: Edge[] = []

function renderCanvas(
  readOnly: boolean,
  callbacks: {
    onSelectNode?: (id: string) => void
    onSelectEdge?: (id: string) => void
    onClearSelection?: () => void
  } = {},
) {
  return render(
    <DefinitionGraphCanvas
      nodes={nodes}
      edges={edges}
      readOnly={readOnly}
      miniMapClassName="feature-mini-map"
      backgroundProps={{ gap: 20, size: 1 }}
      onSelectNode={callbacks.onSelectNode ?? vi.fn()}
      onSelectEdge={callbacks.onSelectEdge ?? vi.fn()}
      onClearSelection={callbacks.onClearSelection ?? vi.fn()}
    />,
  )
}

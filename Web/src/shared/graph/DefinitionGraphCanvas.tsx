import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  type BackgroundProps,
  type Edge,
  type Node,
  type ReactFlowProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import { definitionFitViewOptions, definitionZoomRange } from './graphViewport'

type FlowProps<NodeType extends Node, EdgeType extends Edge> = Pick<
  ReactFlowProps<NodeType, EdgeType>,
  | 'nodes'
  | 'edges'
  | 'nodeTypes'
  | 'onNodesChange'
  | 'onEdgesChange'
  | 'onConnect'
  | 'onNodeDragStop'
>

interface Props<NodeType extends Node, EdgeType extends Edge> extends FlowProps<
  NodeType,
  EdgeType
> {
  readOnly: boolean
  miniMapClassName?: string
  backgroundProps?: BackgroundProps
  onSelectNode: (id: string) => void
  onSelectEdge: (id: string) => void
  onClearSelection: () => void
}

export function DefinitionGraphCanvas<
  NodeType extends Node,
  EdgeType extends Edge,
>({
  readOnly,
  miniMapClassName,
  backgroundProps,
  onSelectNode,
  onSelectEdge,
  onClearSelection,
  ...flowProps
}: Props<NodeType, EdgeType>) {
  return (
    <ReactFlow<NodeType, EdgeType>
      {...flowProps}
      fitView
      fitViewOptions={definitionFitViewOptions}
      minZoom={definitionZoomRange.min}
      maxZoom={definitionZoomRange.max}
      nodesDraggable={!readOnly}
      nodesConnectable={!readOnly}
      edgesReconnectable={!readOnly}
      onNodeClick={(_, node) => onSelectNode(node.id)}
      onEdgeClick={(_, edge) => onSelectEdge(edge.id)}
      onPaneClick={onClearSelection}
    >
      <Background {...backgroundProps} />
      <MiniMap className={miniMapClassName} pannable zoomable />
      <Controls />
    </ReactFlow>
  )
}

import dagre from '@dagrejs/dagre'
import { MarkerType, Position, type Edge, type Node } from '@xyflow/react'

import type { RuntimeResourceNode, RuntimeTopology } from '@/generated/model'

export interface RuntimeNodeData extends Record<string, unknown> {
  resource: RuntimeResourceNode
  active: boolean
}

export type RuntimeFlowNode = Node<RuntimeNodeData, 'runtime'>
export type RuntimeFlowEdge = Edge

const nodeWidth = 236
const nodeHeight = 78

export function runtimeTopologyToGraph(
  topology: RuntimeTopology,
  deploymentActive = false,
): {
  nodes: RuntimeFlowNode[]
  edges: RuntimeFlowEdge[]
} {
  const layout = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  layout.setGraph({ rankdir: 'LR', ranksep: 96, nodesep: 34 })
  topology.nodes.forEach((node) =>
    layout.setNode(node.id, { width: nodeWidth, height: nodeHeight }),
  )
  topology.edges.forEach((edge) => layout.setEdge(edge.source, edge.target))
  dagre.layout(layout)
  return {
    nodes: topology.nodes.map((resource) => {
      const position = layout.node(resource.id)
      return {
        id: resource.id,
        type: 'runtime',
        position: {
          x: position.x - nodeWidth / 2,
          y: position.y - nodeHeight / 2,
        },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        data: {
          resource,
          active:
            deploymentActive &&
            resource.healthStatus !== 'Healthy' &&
            resource.healthStatus !== 'Degraded',
        },
      }
    }),
    edges: topology.edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      type: 'smoothstep',
      animated: deploymentActive && edge.kind === 'network',
      markerEnd: { type: MarkerType.ArrowClosed },
    })),
  }
}

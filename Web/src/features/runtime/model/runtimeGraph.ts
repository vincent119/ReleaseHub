import dagre from '@dagrejs/dagre'
import { MarkerType, Position, type Edge, type Node } from '@xyflow/react'

import type { RuntimeResourceNode, RuntimeTopology } from '@/generated/model'

export interface RuntimeNodeData extends Record<string, unknown> {
  resource: RuntimeResourceNode
  active: boolean
}

export interface ApplicationNodeData extends Record<string, unknown> {
  name: string
}

export type RuntimeResourceFlowNode = Node<RuntimeNodeData, 'runtime'>
export type RuntimeFlowNode =
  RuntimeResourceFlowNode | Node<ApplicationNodeData, 'application'>
export type RuntimeFlowEdge = Edge

const nodeWidth = 236
const nodeHeight = 78
const applicationNodeId = (applicationId: string) =>
  `releasehub:application:${applicationId}`

export const runtimeFitViewOptions = {
  padding: 0.16,
  minZoom: 0.75,
  maxZoom: 1,
} as const

export function runtimeTopologyToGraph(
  topology: RuntimeTopology,
  deploymentActive = false,
  applicationName = topology.applicationId,
): {
  nodes: RuntimeFlowNode[]
  edges: RuntimeFlowEdge[]
} {
  const presentation =
    topology.view === 'resources' && topology.nodes.length > 0
  const rootId = applicationNodeId(topology.applicationId)
  const resourceIds = new Set(topology.nodes.map((node) => node.id))
  const children = new Set(
    topology.edges
      .filter(
        (edge) =>
          edge.kind === 'resource' &&
          resourceIds.has(edge.source) &&
          resourceIds.has(edge.target),
      )
      .map((edge) => edge.target),
  )
  const presentationEdges: RuntimeFlowEdge[] = presentation
    ? topology.nodes
        .filter((node) => !children.has(node.id))
        .map((node) => ({
          id: `presentation:${rootId}:${node.id}`,
          source: rootId,
          target: node.id,
          type: 'smoothstep',
          className: 'runtime-presentation-edge',
          selectable: false,
        }))
    : []
  const vendorEdges: RuntimeFlowEdge[] = topology.edges.map((edge) => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: 'smoothstep',
    animated: deploymentActive && edge.kind === 'network',
    markerEnd: { type: MarkerType.ArrowClosed },
  }))
  const layout = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  layout.setGraph({ rankdir: 'LR', ranksep: 72, nodesep: 28 })
  if (presentation) {
    layout.setNode(rootId, { width: nodeWidth, height: nodeHeight })
  }
  topology.nodes.forEach((node) =>
    layout.setNode(node.id, { width: nodeWidth, height: nodeHeight }),
  )
  for (const edge of [...presentationEdges, ...vendorEdges]) {
    layout.setEdge(edge.source, edge.target)
  }
  dagre.layout(layout)
  const placed = (id: string) => {
    const position = layout.node(id)
    return {
      x: position.x - nodeWidth / 2,
      y: position.y - nodeHeight / 2,
    }
  }
  return {
    nodes: [
      ...(presentation
        ? ([
            {
              id: rootId,
              type: 'application' as const,
              position: placed(rootId),
              sourcePosition: Position.Right,
              selectable: false,
              focusable: false,
              draggable: false,
              data: { name: applicationName },
            },
          ] satisfies RuntimeFlowNode[])
        : []),
      ...topology.nodes.map((resource): RuntimeResourceFlowNode => ({
        id: resource.id,
        type: 'runtime',
        position: placed(resource.id),
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        data: {
          resource,
          active:
            deploymentActive &&
            resource.healthStatus !== 'Healthy' &&
            resource.healthStatus !== 'Degraded',
        },
      })),
    ],
    edges: [...presentationEdges, ...vendorEdges],
  }
}

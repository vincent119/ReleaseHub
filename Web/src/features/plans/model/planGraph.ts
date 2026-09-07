import dagre from '@dagrejs/dagre'
import { Position, type Edge, type Node } from '@xyflow/react'

import type {
  DeploymentPlanDocument,
  DeploymentPlanEdge,
  DeploymentPlanNode,
} from '@/generated/model'

export interface PlanNodeData extends Record<string, unknown> {
  node: DeploymentPlanNode
  label: string
}

export interface PlanEdgeData extends Record<string, unknown> {
  edge: DeploymentPlanEdge
}

export type PlanNode = Node<PlanNodeData>
export type PlanEdge = Edge<PlanEdgeData> & { data: PlanEdgeData }
export interface PlanGraph {
  nodes: PlanNode[]
  edges: PlanEdge[]
  maxParallel?: number
}

const nodeWidth = 190
const nodeHeight = 68

export function documentToPlanGraph(
  document: DeploymentPlanDocument,
): PlanGraph {
  const layout = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  layout.setGraph({ rankdir: 'LR', ranksep: 90, nodesep: 40 })
  document.nodes.forEach((node) =>
    layout.setNode(node.key, { width: nodeWidth, height: nodeHeight }),
  )
  document.edges.forEach((edge) => layout.setEdge(edge.from, edge.to))
  dagre.layout(layout)
  return {
    nodes: document.nodes.map((node) => graphNode(node, layout.node(node.key))),
    edges: document.edges.map(graphEdge),
    maxParallel: document.maxParallel,
  }
}

export function planGraphToDocument(graph: PlanGraph): DeploymentPlanDocument {
  return {
    nodes: graph.nodes.map((node) => node.data.node),
    edges: graph.edges.map((edge) => edge.data.edge),
    maxParallel: graph.maxParallel,
  }
}

export function hasCycle(document: DeploymentPlanDocument): boolean {
  const adjacency = new Map(
    document.nodes.map((node) => [node.key, [] as string[]]),
  )
  document.edges.forEach((edge) => adjacency.get(edge.from)?.push(edge.to))
  const visiting = new Set<string>()
  const visited = new Set<string>()
  const visit = (key: string): boolean => {
    if (visiting.has(key)) return true
    if (visited.has(key)) return false
    visiting.add(key)
    if (adjacency.get(key)?.some(visit)) return true
    visiting.delete(key)
    visited.add(key)
    return false
  }
  return document.nodes.some((node) => visit(node.key))
}

export function emptyPlanDocument(): DeploymentPlanDocument {
  return {
    nodes: [defaultPlanNode('application_1', 0)],
    edges: [],
  }
}

export function defaultPlanNode(
  key: string,
  order: number,
): DeploymentPlanNode {
  return {
    key,
    applicationKey: key,
    order,
    successCondition: {
      syncStatuses: ['Synced'],
      healthStatuses: ['Healthy'],
      stabilizationSeconds: 0,
    },
  }
}

export function edgeIdentity(edge: DeploymentPlanEdge): string {
  return `${edge.from}::${edge.to}`
}

export function orderPlanNodes(nodes: PlanNode[]): PlanNode[] {
  const order = [...nodes]
    .sort((left, right) =>
      left.position.x === right.position.x
        ? left.position.y - right.position.y
        : left.position.x - right.position.x,
    )
    .map((node) => node.id)
  return nodes.map((node) => {
    const value = { ...node.data.node, order: order.indexOf(node.id) }
    return {
      ...node,
      data: { node: value, label: `${value.applicationKey}\n#${value.order}` },
    }
  })
}

function graphNode(
  node: DeploymentPlanNode,
  point: { x: number; y: number },
): PlanNode {
  return {
    id: node.key,
    position: { x: point.x - nodeWidth / 2, y: point.y - nodeHeight / 2 },
    data: { node, label: `${node.applicationKey}\n#${node.order}` },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
  }
}

function graphEdge(edge: DeploymentPlanEdge): PlanEdge {
  return {
    id: edgeIdentity(edge),
    source: edge.from,
    target: edge.to,
    label: edge.condition,
    data: { edge },
    type: 'smoothstep',
  }
}

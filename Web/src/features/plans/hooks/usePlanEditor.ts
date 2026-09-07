import {
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type EdgeChange,
  type NodeChange,
} from '@xyflow/react'
import { useMemo, useReducer } from 'react'

import type { DeploymentPlanEdge, DeploymentPlanNode } from '@/generated/model'

import {
  defaultPlanNode,
  documentToPlanGraph,
  edgeIdentity,
  orderPlanNodes,
  planGraphToDocument,
  type PlanEdge,
  type PlanGraph,
  type PlanNode,
} from '../model/planGraph'

type Action =
  | { type: 'nodes'; changes: NodeChange<PlanNode>[] }
  | { type: 'edges'; changes: EdgeChange<PlanEdge>[] }
  | { type: 'connect'; connection: Connection }
  | { type: 'addNode' }
  | { type: 'updateNode'; id: string; node: DeploymentPlanNode }
  | { type: 'updateEdge'; id: string; edge: DeploymentPlanEdge }
  | { type: 'removeNode'; id: string }
  | { type: 'removeEdge'; id: string }
  | { type: 'maxParallel'; value?: number }
  | { type: 'reorder' }

export function usePlanEditor(initial: ReturnType<typeof planGraphToDocument>) {
  const [graph, dispatch] = useReducer(
    reducePlanGraph,
    initial,
    documentToPlanGraph,
  )
  const document = useMemo(() => planGraphToDocument(graph), [graph])
  return { graph, document, dispatch }
}

export function reducePlanGraph(graph: PlanGraph, action: Action): PlanGraph {
  switch (action.type) {
    case 'nodes':
      return { ...graph, nodes: applyNodeChanges(action.changes, graph.nodes) }
    case 'edges':
      return { ...graph, edges: applyEdgeChanges(action.changes, graph.edges) }
    case 'connect':
      return connectNodes(graph, action.connection)
    case 'addNode':
      return addNode(graph)
    case 'updateNode':
      return updateNode(graph, action.id, action.node)
    case 'updateEdge':
      return updateEdge(graph, action.id, action.edge)
    case 'removeNode':
      return removeNode(graph, action.id)
    case 'removeEdge':
      return {
        ...graph,
        edges: graph.edges.filter((edge) => edge.id !== action.id),
      }
    case 'maxParallel':
      return { ...graph, maxParallel: action.value }
    case 'reorder':
      return { ...graph, nodes: orderPlanNodes(graph.nodes) }
  }
}

function connectNodes(graph: PlanGraph, connection: Connection): PlanGraph {
  if (
    !connection.source ||
    !connection.target ||
    connection.source === connection.target
  )
    return graph
  const edge: DeploymentPlanEdge = {
    from: connection.source,
    to: connection.target,
    condition: 'UpstreamSucceeded',
  }
  if (graph.edges.some((item) => item.id === edgeIdentity(edge))) return graph
  return { ...graph, edges: [...graph.edges, graphEdge(edge)] }
}

function addNode(graph: PlanGraph): PlanGraph {
  const key = uniqueNodeKey(graph)
  const node = defaultPlanNode(key, nextOrder(graph))
  return {
    ...graph,
    nodes: [
      ...graph.nodes,
      {
        id: key,
        position: {
          x: 80 + graph.nodes.length * 32,
          y: 80 + graph.nodes.length * 24,
        },
        data: { node, label: `${node.applicationKey}\n#${node.order}` },
      },
    ],
  }
}

function updateNode(
  graph: PlanGraph,
  id: string,
  node: DeploymentPlanNode,
): PlanGraph {
  return {
    ...graph,
    nodes: graph.nodes.map((item) =>
      item.id === id
        ? {
            ...item,
            id: node.key,
            data: { node, label: `${node.applicationKey}\n#${node.order}` },
          }
        : item,
    ),
    edges: graph.edges.map((edge) => renameEdgeNode(edge, id, node.key)),
  }
}

function updateEdge(
  graph: PlanGraph,
  id: string,
  edge: DeploymentPlanEdge,
): PlanGraph {
  return {
    ...graph,
    edges: graph.edges.map((item) => (item.id === id ? graphEdge(edge) : item)),
  }
}

function removeNode(graph: PlanGraph, id: string): PlanGraph {
  return {
    ...graph,
    nodes: graph.nodes.filter((node) => node.id !== id),
    edges: graph.edges.filter(
      (edge) => edge.source !== id && edge.target !== id,
    ),
  }
}

function renameEdgeNode(edge: PlanEdge, from: string, to: string): PlanEdge {
  return graphEdge({
    ...edge.data.edge,
    from: edge.source === from ? to : edge.source,
    to: edge.target === from ? to : edge.target,
  })
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

function uniqueNodeKey(graph: PlanGraph): string {
  let index = graph.nodes.length + 1
  while (graph.nodes.some((node) => node.id === `application_${index}`)) index++
  return `application_${index}`
}

function nextOrder(graph: PlanGraph): number {
  return Math.max(-1, ...graph.nodes.map((node) => node.data.node.order)) + 1
}

export type PlanEditorAction = Action

import {
  applyEdgeChanges,
  applyNodeChanges,
  type Connection,
  type EdgeChange,
  type NodeChange,
} from '@xyflow/react'
import { useMemo, useReducer } from 'react'

import type {
  ReleaseWorkflowState,
  ReleaseWorkflowTransition,
} from '@/generated/model'

import {
  documentToGraph,
  graphToDocument,
  type WorkflowEdge,
  type WorkflowGraph,
  type WorkflowNode,
} from '../model/workflowGraph'

type Action =
  | { type: 'nodes'; changes: NodeChange<WorkflowNode>[] }
  | { type: 'edges'; changes: EdgeChange<WorkflowEdge>[] }
  | { type: 'connect'; connection: Connection }
  | { type: 'addState' }
  | { type: 'updateState'; id: string; state: ReleaseWorkflowState }
  | {
      type: 'updateTransition'
      id: string
      transition: ReleaseWorkflowTransition
    }
  | { type: 'removeState'; id: string }
  | { type: 'removeTransition'; id: string }
  | { type: 'initial'; id: string }

export function useWorkflowEditor(initial: ReturnType<typeof graphToDocument>) {
  const [graph, dispatch] = useReducer(reducer, initial, documentToGraph)
  const document = useMemo(() => graphToDocument(graph), [graph])
  return { graph, document, dispatch }
}

function reducer(graph: WorkflowGraph, action: Action): WorkflowGraph {
  switch (action.type) {
    case 'nodes':
      return { ...graph, nodes: applyNodeChanges(action.changes, graph.nodes) }
    case 'edges':
      return { ...graph, edges: applyEdgeChanges(action.changes, graph.edges) }
    case 'connect':
      return connectStates(graph, action.connection)
    case 'addState':
      return addState(graph)
    case 'updateState':
      return updateState(graph, action.id, action.state)
    case 'updateTransition':
      return updateTransition(graph, action.id, action.transition)
    case 'removeState':
      return removeState(graph, action.id)
    case 'removeTransition':
      return {
        ...graph,
        edges: graph.edges.filter((edge) => edge.id !== action.id),
      }
    case 'initial':
      return { ...graph, initialState: action.id }
  }
}

function connectStates(
  graph: WorkflowGraph,
  connection: Connection,
): WorkflowGraph {
  if (!connection.source || !connection.target) return graph
  const key = uniqueKey(
    'transition',
    graph.edges.map((edge) => edge.id),
  )
  const transition: ReleaseWorkflowTransition = {
    key,
    from: connection.source,
    to: connection.target,
    trigger: 'Manual',
    permission: 'deployment_request.update',
    conditions: [],
  }
  return { ...graph, edges: [...graph.edges, edgeFromTransition(transition)] }
}

function addState(graph: WorkflowGraph): WorkflowGraph {
  const key = uniqueKey(
    'state',
    graph.nodes.map((node) => node.id),
  )
  const state: ReleaseWorkflowState = {
    key,
    name: 'New state',
    type: 'ManualAction',
  }
  const node: WorkflowNode = {
    id: key,
    type: 'workflowState',
    position: {
      x: 80 + graph.nodes.length * 32,
      y: 80 + graph.nodes.length * 24,
    },
    data: { state, label: `${state.name}\n${state.type}` },
  }
  return { ...graph, nodes: [...graph.nodes, node] }
}

function updateState(
  graph: WorkflowGraph,
  id: string,
  state: ReleaseWorkflowState,
): WorkflowGraph {
  const nodes = graph.nodes.map((node) =>
    node.id === id
      ? {
          ...node,
          id: state.key,
          data: { state, label: `${state.name}\n${state.type}` },
        }
      : node,
  )
  const edges = graph.edges.map((edge) => renameEdgeState(edge, id, state.key))
  return {
    ...graph,
    nodes,
    edges,
    initialState: graph.initialState === id ? state.key : graph.initialState,
  }
}

function updateTransition(
  graph: WorkflowGraph,
  id: string,
  transition: ReleaseWorkflowTransition,
): WorkflowGraph {
  return {
    ...graph,
    edges: graph.edges.map((edge) =>
      edge.id === id ? edgeFromTransition(transition) : edge,
    ),
  }
}

function removeState(graph: WorkflowGraph, id: string): WorkflowGraph {
  const nodes = graph.nodes.filter((node) => node.id !== id)
  const initialState =
    graph.initialState === id ? (nodes[0]?.id ?? '') : graph.initialState
  return {
    ...graph,
    initialState,
    nodes,
    edges: graph.edges.filter(
      (edge) => edge.source !== id && edge.target !== id,
    ),
  }
}

function renameEdgeState(
  edge: WorkflowEdge,
  from: string,
  to: string,
): WorkflowEdge {
  const transition = edge.data.transition
  return edgeFromTransition({
    ...transition,
    from: transition.from === from ? to : transition.from,
    to: transition.to === from ? to : transition.to,
  })
}

function edgeFromTransition(value: ReleaseWorkflowTransition): WorkflowEdge {
  return {
    id: value.key,
    source: value.from,
    target: value.to,
    label: value.key,
    data: { transition: value },
    type: 'smoothstep',
  }
}

function uniqueKey(prefix: string, values: string[]): string {
  let index = values.length + 1
  while (values.includes(`${prefix}_${index}`)) index += 1
  return `${prefix}_${index}`
}

export type WorkflowEditorAction = Action

import dagre from '@dagrejs/dagre'
import { Position, type Edge, type Node } from '@xyflow/react'

import type {
  ReleaseWorkflowDocument,
  ReleaseWorkflowState,
  ReleaseWorkflowTransition,
} from '@/generated/model'

export interface WorkflowNodeData extends Record<string, unknown> {
  state: ReleaseWorkflowState
  label: string
  terminalOutcome?: WorkflowTerminalOutcome
}

export type WorkflowTerminalOutcome = 'success' | 'failure'

export interface WorkflowEdgeData extends Record<string, unknown> {
  transition: ReleaseWorkflowTransition
}

export type WorkflowNode = Node<WorkflowNodeData>
export type WorkflowEdge = Edge<WorkflowEdgeData> & { data: WorkflowEdgeData }

export interface WorkflowGraph {
  nodes: WorkflowNode[]
  edges: WorkflowEdge[]
  initialState: string
}

const nodeWidth = 190
const nodeHeight = 68

export function documentToGraph(
  document: ReleaseWorkflowDocument,
): WorkflowGraph {
  const layout = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  layout.setGraph({ rankdir: 'LR', ranksep: 90, nodesep: 40 })
  document.states.forEach((state) =>
    layout.setNode(state.key, { width: nodeWidth, height: nodeHeight }),
  )
  document.transitions.forEach((transition) =>
    layout.setEdge(transition.from, transition.to),
  )
  dagre.layout(layout)
  return {
    initialState: document.initialState,
    nodes: document.states.map((state) =>
      graphNode(
        state,
        layout.node(state.key),
        terminalOutcome(state, document.transitions),
      ),
    ),
    edges: document.transitions.map(graphEdge),
  }
}

export function graphToDocument(graph: WorkflowGraph): ReleaseWorkflowDocument {
  return {
    initialState: graph.initialState,
    states: graph.nodes.map((node) => node.data.state),
    transitions: graph.edges.map((edge) => edge.data.transition),
  }
}

export function productionApprovalTemplate(): ReleaseWorkflowDocument {
  return {
    initialState: 'pending_review',
    states: [
      reviewState(),
      { key: 'approved', name: 'Approved', type: 'ManualAction' },
      { key: 'deploying', name: 'Deploying', type: 'Deployment' },
      { key: 'succeeded', name: 'Succeeded', type: 'Terminal' },
      { key: 'failed', name: 'Failed', type: 'Terminal' },
    ],
    transitions: templateTransitions(),
  }
}

function reviewState(): ReleaseWorkflowState {
  return {
    key: 'pending_review',
    name: 'Pending Review',
    type: 'Review',
    reviewPolicy: {
      type: 'AnyApprover',
      requiredApprovals: 1,
      allowSelfReview: false,
      userIds: [],
      roleIds: [],
    },
  }
}

function templateTransitions(): ReleaseWorkflowTransition[] {
  return [
    transition(
      'approve',
      'pending_review',
      'approved',
      'ReviewSatisfied',
      'deployment_request.review',
    ),
    transition(
      'deploy',
      'approved',
      'deploying',
      'Manual',
      'deployment_request.deploy',
    ),
    transition(
      'succeed',
      'deploying',
      'succeeded',
      'DeploymentResult',
      'deployment_request.update',
      [{ fact: 'deployment.status', operator: 'Equals', value: 'Succeeded' }],
    ),
    transition(
      'fail',
      'deploying',
      'failed',
      'DeploymentResult',
      'deployment_request.update',
      [
        {
          fact: 'deployment.status',
          operator: 'In',
          value: ['Failed', 'PartialFailed', 'Blocked', 'Terminated'],
        },
      ],
    ),
  ]
}

function transition(
  key: string,
  from: string,
  to: string,
  trigger: ReleaseWorkflowTransition['trigger'],
  permission: string,
  conditions: ReleaseWorkflowTransition['conditions'] = [],
): ReleaseWorkflowTransition {
  return { key, from, to, trigger, permission, conditions }
}

function graphNode(
  state: ReleaseWorkflowState,
  point: { x: number; y: number },
  outcome?: WorkflowTerminalOutcome,
): WorkflowNode {
  return {
    id: state.key,
    type: 'workflowState',
    position: { x: point.x - nodeWidth / 2, y: point.y - nodeHeight / 2 },
    data: {
      state,
      label: `${state.name}\n${state.type}`,
      ...(outcome ? { terminalOutcome: outcome } : {}),
    },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
  }
}

const successfulDeploymentStatuses = new Set(['Succeeded'])
const failedDeploymentStatuses = new Set([
  'Failed',
  'PartialFailed',
  'Blocked',
  'Terminated',
])

function terminalOutcome(
  state: ReleaseWorkflowState,
  transitions: ReleaseWorkflowTransition[],
): WorkflowTerminalOutcome | undefined {
  if (state.type !== 'Terminal') return undefined

  const candidates = transitions.filter(
    (transition) =>
      transition.to === state.key && transition.trigger === 'DeploymentResult',
  )
  if (candidates.length === 0) return undefined

  const outcomes = candidates.map((transition) => {
    const statusConditions = transition.conditions.filter(
      (condition) =>
        condition.fact === 'deployment.status' &&
        (condition.operator === 'Equals' || condition.operator === 'In'),
    )
    if (statusConditions.length === 0) return undefined
    return outcomeFromStatuses(
      statusConditions.flatMap((condition) =>
        Array.isArray(condition.value) ? condition.value : [condition.value],
      ),
    )
  })
  if (outcomes.some((outcome) => !outcome)) return undefined

  return new Set(outcomes).size === 1 ? outcomes[0] : undefined
}

function outcomeFromStatuses(
  value: string | string[],
): WorkflowTerminalOutcome | undefined {
  const statuses = Array.isArray(value) ? value : [value]
  if (
    statuses.length > 0 &&
    statuses.every((status) => successfulDeploymentStatuses.has(status))
  )
    return 'success'
  if (
    statuses.length > 0 &&
    statuses.every((status) => failedDeploymentStatuses.has(status))
  )
    return 'failure'
  return undefined
}

function graphEdge(value: ReleaseWorkflowTransition): WorkflowEdge {
  return {
    id: value.key,
    source: value.from,
    target: value.to,
    label: value.key,
    data: { transition: value },
    type: 'smoothstep',
  }
}

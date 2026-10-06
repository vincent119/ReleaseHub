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

export interface RuntimeGroupNodeData extends Record<string, unknown> {
  groupId: string
  parentId: string | null
  kind: string | null
  rootCount: number
  memberIds: string[]
  kindCounts: Record<string, number>
  healthCounts: Record<string, number>
}

export type RuntimeResourceFlowNode = Node<RuntimeNodeData, 'runtime'>
export type RuntimeFlowNode =
  | RuntimeResourceFlowNode
  | Node<ApplicationNodeData, 'application'>
  | Node<RuntimeGroupNodeData, 'runtime-group'>
export type RuntimeFlowEdge = Edge

export interface RuntimeLayeredGraph {
  nodes: RuntimeFlowNode[]
  edges: RuntimeFlowEdge[]
  groups: RuntimeGroupNodeData[]
  visibleResourceCount: number
  groupedResourceCount: number
  groupingAvailable: boolean
}

const nodeWidth = 236
const nodeHeight = 78
const applicationNodeId = (applicationId: string) =>
  `releasehub:application:${applicationId}`

export const runtimeFitViewOptions = {
  padding: 0.16,
  minZoom: 0.001,
  maxZoom: 1,
} as const

export function runtimeDisplayTopology(
  topology: RuntimeTopology,
): RuntimeTopology {
  if (topology.view !== 'network') return topology

  const resourceIds = new Set(topology.nodes.map((node) => node.id))
  const edges = topology.edges.filter(
    (edge) =>
      edge.kind === 'network' &&
      resourceIds.has(edge.source) &&
      resourceIds.has(edge.target),
  )
  const visibleIds = new Set(
    edges.flatMap((edge) => [edge.source, edge.target]),
  )
  for (const node of topology.nodes) {
    if (node.ingress.length > 0 || node.externalUrls.length > 0) {
      visibleIds.add(node.id)
    }
  }

  // 僅投影顯示，不能更動查詢快取、後端證據或資源詳情的原始識別。
  return {
    ...topology,
    nodes: topology.nodes.filter((node) => visibleIds.has(node.id)),
    edges,
  }
}

export function runtimeDisplayName(name: string): string {
  if (name.length <= 18) return name
  return `${name.slice(0, 8)}…${name.slice(-9)}`
}

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
              width: nodeWidth,
              height: nodeHeight,
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
        width: nodeWidth,
        height: nodeHeight,
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

const protectedKinds = new Set([
  'Deployment',
  'StatefulSet',
  'DaemonSet',
  'Job',
  'CronJob',
  'Rollout',
  'Ingress',
  'Service',
])

export function runtimeLayeredTopologyToGraph(
  topology: RuntimeTopology,
  expandedGroupIds: ReadonlySet<string>,
  deploymentActive = false,
  applicationName = topology.applicationId,
): RuntimeLayeredGraph {
  const full = runtimeTopologyToGraph(
    topology,
    deploymentActive,
    applicationName,
  )
  const fallback = (): RuntimeLayeredGraph => ({
    ...full,
    groups: [],
    visibleResourceCount: topology.nodes.length,
    groupedResourceCount: 0,
    groupingAvailable: false,
  })
  // 證據不完整時不能把未知關係折成看似健康的群組。
  if (
    topology.view !== 'resources' ||
    topology.partial ||
    topology.warnings.length > 0 ||
    topology.edges.some((edge) => edge.kind !== 'resource')
  )
    return fallback()

  const resources = new Map(topology.nodes.map((node) => [node.id, node]))
  if (resources.size !== topology.nodes.length) return fallback()
  const incoming = new Map<string, string[]>()
  const children = new Map<string, string[]>()
  for (const edge of topology.edges) {
    if (!resources.has(edge.source) || !resources.has(edge.target))
      return fallback()
    incoming.set(edge.target, [
      ...(incoming.get(edge.target) ?? []),
      edge.source,
    ])
    children.set(edge.source, [
      ...(children.get(edge.source) ?? []),
      edge.target,
    ])
  }

  const safeSubtree = (rootId: string): string[] | null => {
    const visited = new Set<string>()
    const visit = (id: string, parentId?: string): boolean => {
      const node = resources.get(id)
      if (
        !node ||
        visited.has(id) ||
        node.healthStatus !== 'Healthy' ||
        node.orphaned
      )
        return false
      const parents = incoming.get(id) ?? []
      if (parentId && (parents.length !== 1 || parents[0] !== parentId))
        return false
      visited.add(id)
      return (children.get(id) ?? []).every((childId) => visit(childId, id))
    }
    return visit(rootId) ? [...visited] : null
  }

  const candidates: Array<{
    parentId: string | null
    kind: string | null
    roots: string[]
    members: string[]
  }> = []
  const siblingBuckets = new Map<string, string[]>()
  for (const node of topology.nodes) {
    const parents = incoming.get(node.id) ?? []
    if (parents.length !== 1 || protectedKinds.has(node.kind)) continue
    const key = JSON.stringify([parents[0], node.kind])
    siblingBuckets.set(key, [...(siblingBuckets.get(key) ?? []), node.id])
  }
  for (const [key, rootIds] of [...siblingBuckets].sort(([a], [b]) =>
    a.localeCompare(b),
  )) {
    const [parentId, kind] = JSON.parse(key) as [string, string]
    const eligible = rootIds
      .map((id) => ({ id, members: safeSubtree(id) }))
      .filter(
        (entry): entry is { id: string; members: string[] } =>
          entry.members !== null,
      )
    if (eligible.length < 2) continue
    candidates.push({
      parentId,
      kind,
      roots: eligible.map((entry) => entry.id).sort(),
      members: eligible.flatMap((entry) => entry.members),
    })
  }
  const otherRoots = topology.nodes
    .filter(
      (node) =>
        (incoming.get(node.id) ?? []).length === 0 &&
        (children.get(node.id) ?? []).length === 0 &&
        node.healthStatus === 'Healthy' &&
        !node.orphaned &&
        !protectedKinds.has(node.kind),
    )
    .map((node) => node.id)
    .sort()
  if (otherRoots.length >= 2)
    candidates.unshift({
      parentId: null,
      kind: null,
      roots: otherRoots,
      members: otherRoots,
    })

  const assigned = new Set<string>()
  const occupied = new Set(resources.keys())
  occupied.add(applicationNodeId(topology.applicationId))
  const groups: RuntimeGroupNodeData[] = []
  for (const candidate of candidates.sort(
    (left, right) =>
      right.members.length - left.members.length ||
      left.roots[0].localeCompare(right.roots[0]),
  )) {
    if (
      assigned.has(candidate.parentId ?? '') ||
      candidate.members.some((id) => assigned.has(id))
    )
      continue
    const identity = JSON.stringify([
      topology.applicationId,
      candidate.parentId,
      candidate.kind,
      candidate.roots,
    ])
    let groupId = `releasehub:group:${hashGroupIdentity(identity)}`
    for (let suffix = 1; occupied.has(groupId); suffix += 1)
      groupId = `releasehub:group:${hashGroupIdentity(identity)}:${suffix}`
    occupied.add(groupId)
    const kindCounts: Record<string, number> = {}
    const healthCounts: Record<string, number> = {}
    for (const id of candidate.members) {
      const node = resources.get(id)!
      kindCounts[node.kind] = (kindCounts[node.kind] ?? 0) + 1
      const health = node.healthStatus || 'Unknown'
      healthCounts[health] = (healthCounts[health] ?? 0) + 1
      assigned.add(id)
    }
    groups.push({
      groupId,
      kind: candidate.kind,
      rootCount: candidate.roots.length,
      memberIds: [...candidate.members].sort(),
      kindCounts,
      healthCounts,
      parentId: candidate.parentId,
    })
  }

  const hidden = new Set(
    groups
      .filter((group) => !expandedGroupIds.has(group.groupId))
      .flatMap((group) => group.memberIds),
  )
  if (groups.length === 0)
    return {
      ...full,
      groups,
      visibleResourceCount: topology.nodes.length,
      groupedResourceCount: 0,
      groupingAvailable: false,
    }
  const nodes: RuntimeFlowNode[] = full.nodes.filter(
    (node) => node.type === 'application' || !hidden.has(node.id),
  )
  const edges: RuntimeFlowEdge[] = full.edges.filter(
    (edge) => !hidden.has(edge.source) && !hidden.has(edge.target),
  )
  for (const group of groups) {
    if (expandedGroupIds.has(group.groupId)) continue
    nodes.push({
      id: group.groupId,
      type: 'runtime-group',
      width: nodeWidth,
      height: nodeHeight,
      position: { x: 0, y: 0 },
      sourcePosition: Position.Right,
      targetPosition: Position.Left,
      draggable: false,
      data: group,
    })
    edges.push({
      id: `presentation:${group.groupId}`,
      source: group.parentId ?? applicationNodeId(topology.applicationId),
      target: group.groupId,
      type: 'smoothstep',
      className: 'runtime-presentation-edge',
      selectable: false,
    })
  }
  return {
    nodes: layoutFlowGraph(nodes, edges),
    edges,
    groups,
    visibleResourceCount: topology.nodes.length - hidden.size,
    groupedResourceCount: hidden.size,
    groupingAvailable: groups.length > 0,
  }
}

function hashGroupIdentity(value: string): string {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0).toString(36)
}

function layoutFlowGraph(
  nodes: RuntimeFlowNode[],
  edges: RuntimeFlowEdge[],
): RuntimeFlowNode[] {
  const layout = new dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}))
  layout.setGraph({ rankdir: 'LR', ranksep: 24, nodesep: 16 })
  for (const node of nodes)
    layout.setNode(node.id, { width: nodeWidth, height: nodeHeight })
  for (const edge of edges) layout.setEdge(edge.source, edge.target)
  dagre.layout(layout)
  return nodes.map((node) => {
    const position = layout.node(node.id)
    return {
      ...node,
      position: {
        x: position.x - nodeWidth / 2,
        y: position.y - nodeHeight / 2,
      },
    }
  })
}

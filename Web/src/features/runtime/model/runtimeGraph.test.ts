import { describe, expect, it } from 'vitest'

import type { RuntimeTopology } from '@/generated/model'

import {
  runtimeDisplayName,
  runtimeDisplayTopology,
  runtimeFitViewOptions,
  runtimeLayeredTopologyToGraph,
  runtimeTopologyToGraph,
} from './runtimeGraph'

describe('runtimeTopologyToGraph', () => {
  it('groups top-level resources under an inert Application node while preserving vendor edges', () => {
    const topology: RuntimeTopology = {
      applicationId: 'application-1',
      view: 'resources',
      observedAt: '2026-09-23T08:00:00Z',
      partial: false,
      warnings: [],
      nodes: [
        resource('deployment'),
        resource('replica-set'),
        resource('pod'),
        resource('service'),
      ],
      edges: [
        {
          id: 'resource:deployment:replica-set',
          source: 'deployment',
          target: 'replica-set',
          kind: 'resource',
        },
        {
          id: 'resource:replica-set:pod',
          source: 'replica-set',
          target: 'pod',
          kind: 'resource',
        },
      ],
    }

    const graph = runtimeTopologyToGraph(topology, false, 'payments')

    expect(graph.nodes).toHaveLength(5)
    // 受控節點必須提供尺寸，MiniMap 才能繪製節點。
    for (const node of graph.nodes) {
      expect(node).toMatchObject({ width: 236, height: 78 })
    }
    expect(graph.nodes[0]).toMatchObject({
      type: 'application',
      selectable: false,
      focusable: false,
      draggable: false,
      data: { name: 'payments' },
    })
    expect(graph.edges).toHaveLength(4)
    expect(
      graph.edges.filter((edge) => edge.id.startsWith('presentation:')),
    ).toEqual([
      expect.objectContaining({ target: 'deployment', selectable: false }),
      expect.objectContaining({ target: 'service', selectable: false }),
    ])
    expect(
      graph.edges.find((edge) => edge.id === 'resource:deployment:replica-set'),
    ).toMatchObject({
      source: 'deployment',
      target: 'replica-set',
    })
    expect(
      graph.edges.find((edge) => edge.id === 'resource:replica-set:pod'),
    ).toMatchObject({
      source: 'replica-set',
      target: 'pod',
    })
    expect(graph.nodes[0].position.x).toBeLessThan(graph.nodes[1].position.x)
    expect(runtimeFitViewOptions.minZoom).toBeLessThan(0.75)
  })

  it('keeps network relationships free of presentation nodes', () => {
    const topology: RuntimeTopology = {
      applicationId: 'application-1',
      view: 'network',
      observedAt: '2026-09-23T08:00:00Z',
      partial: false,
      warnings: [],
      nodes: [resource('service'), resource('pod')],
      edges: [
        {
          id: 'network:service:pod',
          source: 'service',
          target: 'pod',
          kind: 'network',
        },
      ],
    }

    const graph = runtimeTopologyToGraph(topology, true, 'payments')

    expect(graph.nodes).toHaveLength(2)
    expect(graph.edges).toHaveLength(1)
    expect(graph.edges[0]).toMatchObject({
      animated: true,
      source: 'service',
      target: 'pod',
    })
  })

  it('projects only valid network endpoints and explicit entry evidence without changing the response', () => {
    const topology: RuntimeTopology = {
      applicationId: 'application-1',
      view: 'network',
      observedAt: '2026-09-23T08:00:00Z',
      partial: true,
      warnings: ['network_evidence_unresolved'],
      nodes: [
        resource('ingress'),
        resource('service'),
        resource('pod-1'),
        resource('pod-2'),
        resource('unlinked-entry'),
        resource('secret'),
        resource('replica-set'),
      ],
      edges: [
        { id: 'edge-1', source: 'ingress', target: 'service', kind: 'network' },
        { id: 'edge-2', source: 'service', target: 'pod-1', kind: 'network' },
        { id: 'edge-3', source: 'service', target: 'pod-2', kind: 'network' },
        {
          id: 'invalid',
          source: 'service',
          target: 'missing',
          kind: 'network',
        },
        {
          id: 'owner',
          source: 'replica-set',
          target: 'pod-1',
          kind: 'resource',
        },
      ],
    }
    topology.nodes[4].externalUrls = ['https://entry.example']
    const projected = runtimeDisplayTopology(topology)

    expect(projected.nodes.map((node) => node.id)).toEqual([
      'ingress',
      'service',
      'pod-1',
      'pod-2',
      'unlinked-entry',
    ])
    expect(projected.edges.map((edge) => edge.id)).toEqual([
      'edge-1',
      'edge-2',
      'edge-3',
    ])
    expect(projected.warnings).toBe(topology.warnings)
    expect(projected.partial).toBe(true)
    expect(topology.nodes).toHaveLength(7)
    expect(topology.edges).toHaveLength(5)
    expect(runtimeTopologyToGraph(projected).nodes).toHaveLength(5)
  })

  it('keeps only explicit entrances when network edges are absent', () => {
    const topology: RuntimeTopology = {
      applicationId: 'application-1',
      view: 'network',
      observedAt: '2026-09-23T08:00:00Z',
      partial: false,
      warnings: ['network_evidence_unavailable'],
      nodes: [resource('ingress'), resource('secret')],
      edges: [],
    }
    topology.nodes[0].ingress = ['example.com']
    expect(
      runtimeDisplayTopology(topology).nodes.map((node) => node.id),
    ).toEqual(['ingress'])
    topology.nodes[0].ingress = []
    expect(runtimeDisplayTopology(topology).nodes).toEqual([])
    expect(
      runtimeDisplayTopology({ ...topology, view: 'resources' }).nodes,
    ).toBe(topology.nodes)
  })

  it('keeps differentiating long Pod and ReplicaSet suffixes', () => {
    const prefix = 'status-webhooks-deploy-b9cc74fc5-'
    expect(runtimeDisplayName(`${prefix}54kq9`)).not.toBe(
      runtimeDisplayName(`${prefix}64kq9`),
    )
    expect(runtimeDisplayName(`${prefix}54kq9`)).toMatch(/54kq9$/)
    expect(runtimeDisplayName('short-name')).toBe('short-name')
  })
})

describe('runtimeLayeredTopologyToGraph', () => {
  const topology = (): RuntimeTopology => {
    const kinds: Record<string, string> = {
      ingress: 'Ingress',
      service: 'Service',
      deployment: 'Deployment',
      namespace: 'Namespace',
      secret: 'Secret',
      account: 'ServiceAccount',
      policy: 'NetworkPolicy',
      budget: 'PodDisruptionBudget',
      binding: 'RoleBinding',
    }
    const nodes = [
      ...Object.entries(kinds).map(([id, kind]) => ({ ...resource(id), kind })),
      ...Array.from({ length: 4 }, (_, index) => ({
        ...resource(`replica-${index}`),
        kind: 'ReplicaSet',
      })),
      ...Array.from({ length: 2 }, (_, index) => ({
        ...resource(`pod-${index}`),
        kind: 'Pod',
      })),
    ]
    const edges = [
      ...Array.from({ length: 4 }, (_, index) => ({
        id: `e-deployment-${index}`,
        source: 'deployment',
        target: `replica-${index}`,
        kind: 'resource' as const,
      })),
      ...Array.from({ length: 2 }, (_, index) => ({
        id: `e-pod-${index}`,
        source: 'replica-0',
        target: `pod-${index}`,
        kind: 'resource' as const,
      })),
    ]
    return {
      applicationId: 'application-1',
      view: 'resources',
      observedAt: '2026-09-23T08:00:00Z',
      partial: false,
      warnings: [],
      nodes,
      edges,
    }
  }

  it('keeps the main path visible and collapses only healthy evidence-safe branches', () => {
    const original = topology()
    const graph = runtimeLayeredTopologyToGraph(original, new Set())

    expect(graph.groupingAvailable).toBe(true)
    expect(graph.groups).toHaveLength(2)
    expect(graph.visibleResourceCount).toBe(3)
    expect(graph.groupedResourceCount).toBe(12)
    expect(
      graph.nodes.filter((node) => node.type === 'runtime-group'),
    ).toHaveLength(2)
    expect(graph.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining(['ingress', 'service', 'deployment']),
    )
    expect(
      graph.edges.filter((edge) => edge.id.startsWith('presentation:')),
    ).toHaveLength(5)
    expect(original.nodes).toHaveLength(15)
    expect(original.edges).toHaveLength(6)
  })

  it('expands with original resource IDs and evidence edges, and keeps stable group identities', () => {
    const original = topology()
    const collapsed = runtimeLayeredTopologyToGraph(original, new Set())
    const replicaGroup = collapsed.groups.find(
      (group) => group.kind === 'ReplicaSet',
    )!
    const expanded = runtimeLayeredTopologyToGraph(
      original,
      new Set([replicaGroup.groupId]),
    )

    expect(expanded.groups.map((group) => group.groupId)).toEqual(
      collapsed.groups.map((group) => group.groupId),
    )
    expect(
      expanded.nodes.some((node) => node.id === replicaGroup.groupId),
    ).toBe(false)
    expect(expanded.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining(replicaGroup.memberIds),
    )
    expect(
      expanded.edges.filter((edge) => edge.id.startsWith('e-')),
    ).toHaveLength(6)
    expect(expanded.groupedResourceCount).toBe(6)
  })

  it('does not conceal abnormal resources or incomplete evidence', () => {
    const abnormal = topology()
    abnormal.nodes.find((node) => node.id === 'replica-1')!.healthStatus =
      'Progressing'
    const grouped = runtimeLayeredTopologyToGraph(abnormal, new Set())
    expect(grouped.nodes.some((node) => node.id === 'replica-1')).toBe(true)
    expect(
      grouped.groups.every((group) => !group.memberIds.includes('replica-1')),
    ).toBe(true)

    abnormal.partial = true
    const fallback = runtimeLayeredTopologyToGraph(abnormal, new Set())
    expect(fallback.groupingAvailable).toBe(false)
    expect(fallback.nodes).toHaveLength(16)
    expect(fallback.groupedResourceCount).toBe(0)
  })

  it('leaves orphaned and shared-child resources visible instead of inventing ownership', () => {
    const original = topology()
    original.nodes.find((node) => node.id === 'secret')!.orphaned = true
    original.edges.push({
      id: 'shared-child',
      source: 'replica-1',
      target: 'pod-0',
      kind: 'resource',
    })
    const graph = runtimeLayeredTopologyToGraph(original, new Set())
    expect(graph.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining(['secret', 'replica-0', 'replica-1', 'pod-0']),
    )
    expect(
      graph.groups.every(
        (group) =>
          !group.memberIds.includes('secret') &&
          !group.memberIds.includes('pod-0'),
      ),
    ).toBe(true)
  })

  it('keeps group IDs stable across response ordering changes', () => {
    const original = topology()
    const first = runtimeLayeredTopologyToGraph(original, new Set())
    const reordered = runtimeLayeredTopologyToGraph(
      {
        ...original,
        nodes: [...original.nodes].reverse(),
        edges: [...original.edges].reverse(),
      },
      new Set(),
    )
    expect(reordered.groups.map((group) => group.groupId)).toEqual(
      first.groups.map((group) => group.groupId),
    )
  })

  it('never groups the network evidence graph', () => {
    const network = topology()
    network.view = 'network'
    network.edges = [
      {
        id: 'network:service:pod-0',
        source: 'service',
        target: 'pod-0',
        kind: 'network',
      },
    ]
    const projected = runtimeDisplayTopology(network)
    const graph = runtimeLayeredTopologyToGraph(projected, new Set())
    expect(graph.groupingAvailable).toBe(false)
    expect(graph.nodes.map((node) => node.id)).toEqual(['service', 'pod-0'])
    expect(graph.edges.map((edge) => edge.id)).toEqual([
      'network:service:pod-0',
    ])
  })

  it('keeps a 100-resource response reversible without losing or repeating members', () => {
    const large = topology()
    large.nodes = [
      { ...resource('deployment'), kind: 'Deployment' },
      ...Array.from({ length: 99 }, (_, index) => ({
        ...resource(`replica-${index}`),
        kind: 'ReplicaSet',
      })),
    ]
    large.edges = Array.from({ length: 99 }, (_, index) => ({
      id: `edge-${index}`,
      source: 'deployment',
      target: `replica-${index}`,
      kind: 'resource',
    }))
    const collapsed = runtimeLayeredTopologyToGraph(large, new Set())
    expect(collapsed.groups).toHaveLength(1)
    expect(collapsed.groups[0].memberIds).toHaveLength(99)
    expect(new Set(collapsed.groups[0].memberIds).size).toBe(99)
    expect(
      collapsed.visibleResourceCount + collapsed.groupedResourceCount,
    ).toBe(100)
    const expanded = runtimeLayeredTopologyToGraph(
      large,
      new Set([collapsed.groups[0].groupId]),
    )
    expect(expanded.nodes).toHaveLength(101)
    expect(
      expanded.edges.filter((edge) => edge.id.startsWith('edge-')),
    ).toHaveLength(99)
    expect(large.nodes).toHaveLength(100)
  })
})

function resource(id: string) {
  return {
    id,
    group: 'apps',
    version: 'v1',
    kind:
      id === 'pod' ? 'Pod' : id === 'replica-set' ? 'ReplicaSet' : 'Deployment',
    namespace: 'payments',
    name: id,
    healthStatus: 'Healthy',
    healthMessage: '',
    orphaned: false,
    images: [],
    info: [],
    ingress: [],
    externalUrls: [],
  }
}

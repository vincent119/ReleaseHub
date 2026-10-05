import { describe, expect, it } from 'vitest'

import type { RuntimeTopology } from '@/generated/model'

import {
  runtimeDisplayName,
  runtimeDisplayTopology,
  runtimeFitViewOptions,
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

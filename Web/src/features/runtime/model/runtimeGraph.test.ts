import { describe, expect, it } from 'vitest'

import type { RuntimeTopology } from '@/generated/model'

import { runtimeFitViewOptions, runtimeTopologyToGraph } from './runtimeGraph'

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
    expect(runtimeFitViewOptions.minZoom).toBeGreaterThanOrEqual(0.75)
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

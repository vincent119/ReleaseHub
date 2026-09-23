import { describe, expect, it } from 'vitest'

import type { RuntimeTopology } from '@/generated/model'

import { runtimeTopologyToGraph } from './runtimeGraph'

describe('runtimeTopologyToGraph', () => {
  it('preserves only server-provided nodes and edges', () => {
    const topology: RuntimeTopology = {
      applicationId: 'application-1',
      view: 'resources',
      observedAt: '2026-09-23T08:00:00Z',
      partial: false,
      warnings: [],
      nodes: [resource('deployment'), resource('pod')],
      edges: [
        {
          id: 'resource:deployment:pod',
          source: 'deployment',
          target: 'pod',
          kind: 'resource',
        },
      ],
    }

    const graph = runtimeTopologyToGraph(topology)

    expect(graph.nodes.map((node) => node.id)).toEqual(['deployment', 'pod'])
    expect(graph.edges).toHaveLength(1)
    expect(graph.edges[0]).toMatchObject({
      source: 'deployment',
      target: 'pod',
    })
  })
})

function resource(id: string) {
  return {
    id,
    group: 'apps',
    version: 'v1',
    kind: id === 'pod' ? 'Pod' : 'Deployment',
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

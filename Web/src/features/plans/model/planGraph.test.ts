import { describe, expect, it } from 'vitest'

import type { DeploymentPlanDocument } from '@/generated/model'

import { reducePlanGraph } from '../hooks/usePlanEditor'

import {
  documentToPlanGraph,
  hasCycle,
  orderPlanNodes,
  planGraphToDocument,
} from './planGraph'

const document: DeploymentPlanDocument = {
  nodes: [
    {
      key: 'a',
      applicationKey: 'APP1',
      order: 0,
      successCondition: condition(),
    },
    {
      key: 'c',
      applicationKey: 'APP3',
      order: 1,
      successCondition: condition(),
    },
    {
      key: 'b',
      applicationKey: 'APP2',
      order: 2,
      successCondition: condition(),
    },
  ],
  edges: [
    { from: 'a', to: 'b', condition: 'UpstreamSucceeded' },
    { from: 'c', to: 'b', condition: 'PrerequisiteHealthy' },
  ],
  maxParallel: 2,
}

describe('Deployment Plan graph mapping', () => {
  it('preserves A/C to B dependencies, node order, and conditions', () => {
    expect(planGraphToDocument(documentToPlanGraph(document))).toEqual(document)
  })

  it('reports a cycle without changing the document', () => {
    expect(hasCycle(document)).toBe(false)
    expect(
      hasCycle({
        ...document,
        edges: [
          ...document.edges,
          { from: 'b', to: 'a', condition: 'UpstreamSucceeded' },
        ],
      }),
    ).toBe(true)
  })

  it('updates execution order from dragged node positions', () => {
    const graph = documentToPlanGraph(document)
    const positioned = graph.nodes.map((node) => ({
      ...node,
      position: {
        ...node.position,
        x: node.id === 'b' ? -100 : node.position.x,
      },
    }))
    expect(
      orderPlanNodes(positioned).find((node) => node.id === 'b')?.data.node
        .order,
    ).toBe(0)
  })

  it('supports node and edge create, update, and remove operations', () => {
    let graph = documentToPlanGraph(document)
    graph = reducePlanGraph(graph, { type: 'addNode' })
    expect(graph.nodes).toHaveLength(4)
    graph = reducePlanGraph(graph, {
      type: 'connect',
      connection: {
        source: 'a',
        target: 'c',
        sourceHandle: null,
        targetHandle: null,
      },
    })
    expect(graph.edges).toHaveLength(3)
    graph = reducePlanGraph(graph, {
      type: 'updateNode',
      id: 'a',
      node: { ...graph.nodes[0].data.node, key: 'alpha' },
    })
    expect(graph.edges.some((edge) => edge.source === 'alpha')).toBe(true)
    graph = reducePlanGraph(graph, { type: 'removeEdge', id: 'alpha::c' })
    expect(graph.edges).toHaveLength(2)
    graph = reducePlanGraph(graph, { type: 'removeNode', id: 'alpha' })
    expect(graph.nodes.some((node) => node.id === 'alpha')).toBe(false)
  })
})

function condition() {
  return {
    syncStatuses: ['Synced'],
    healthStatuses: ['Healthy'],
    stabilizationSeconds: 0,
    timeoutSeconds: 300,
  }
}

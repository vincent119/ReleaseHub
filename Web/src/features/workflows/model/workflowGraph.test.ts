import { describe, expect, it } from 'vitest'

import type { ReleaseWorkflowDocument } from '@/generated/model'

import {
  documentToGraph,
  graphToDocument,
  productionApprovalTemplate,
} from './workflowGraph'

describe('workflow graph mapping', () => {
  it('preserves the complete workflow document during a graph round trip', () => {
    const document: ReleaseWorkflowDocument = {
      initialState: 'review',
      states: [
        {
          key: 'review',
          name: 'Production Review',
          type: 'Review',
          reviewPolicy: {
            type: 'RoleMinimumOne',
            requiredApprovals: 2,
            allowSelfReview: false,
            userIds: ['user-a'],
            roleIds: ['sre-manager'],
          },
        },
        { key: 'deploy', name: 'Deploy', type: 'Deployment' },
      ],
      transitions: [
        {
          key: 'approved',
          from: 'review',
          to: 'deploy',
          trigger: 'ReviewSatisfied',
          permission: 'deployment_request.deploy',
          conditions: [
            {
              fact: 'request.classification',
              operator: 'Equals',
              value: 'Standard',
            },
          ],
        },
      ],
    }

    const graph = documentToGraph(document)

    expect(graphToDocument(graph)).toEqual(document)
    expect(graph.nodes).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          type: 'workflowState',
          data: expect.objectContaining({ label: 'Production Review\nReview' }),
        }),
        expect.objectContaining({
          type: 'workflowState',
          data: expect.objectContaining({ label: 'Deploy\nDeployment' }),
        }),
      ]),
    )
  })

  it('routes production deployment results by execution status', () => {
    const template = productionApprovalTemplate()
    const resultTransitions = template.transitions.filter(
      (transition) => transition.trigger === 'DeploymentResult',
    )
    const graph = documentToGraph(template)

    expect(resultTransitions).toHaveLength(2)
    expect(
      resultTransitions.every((transition) => transition.conditions.length > 0),
    ).toBe(true)
    expect(graph.nodes.find((node) => node.id === 'succeeded')?.data).toEqual(
      expect.objectContaining({ terminalOutcome: 'success' }),
    )
    expect(graph.nodes.find((node) => node.id === 'failed')?.data).toEqual(
      expect.objectContaining({ terminalOutcome: 'failure' }),
    )
    expect(graphToDocument(graph)).toEqual(template)
  })

  it('does not infer terminal outcomes from a state name or key', () => {
    const document: ReleaseWorkflowDocument = {
      initialState: 'deploying',
      states: [
        { key: 'deploying', name: 'Deploying', type: 'Deployment' },
        { key: 'failed', name: 'Succeeded', type: 'Terminal' },
      ],
      transitions: [
        {
          key: 'finish',
          from: 'deploying',
          to: 'failed',
          trigger: 'Manual',
          permission: 'deployment_request.update',
          conditions: [],
        },
      ],
    }

    const terminal = documentToGraph(document).nodes.find(
      (node) => node.id === 'failed',
    )
    expect(terminal?.data.terminalOutcome).toBeUndefined()
  })

  it('keeps mixed deployment outcomes visually neutral', () => {
    const document = productionApprovalTemplate()
    const succeeded = document.transitions.find(
      (transition) => transition.to === 'succeeded',
    )!
    succeeded.conditions[0].value = ['Succeeded', 'Failed']

    const terminal = documentToGraph(document).nodes.find(
      (node) => node.id === 'succeeded',
    )
    expect(terminal?.data.terminalOutcome).toBeUndefined()
  })
})

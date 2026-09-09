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
    const resultTransitions = productionApprovalTemplate().transitions.filter(
      (transition) => transition.trigger === 'DeploymentResult',
    )

    expect(resultTransitions).toHaveLength(2)
    expect(
      resultTransitions.every((transition) => transition.conditions.length > 0),
    ).toBe(true)
  })
})

import { describe, expect, it } from 'vitest'

import type { ReleaseWorkflowDocument } from '@/generated/model'

import { buildWorkflowProgress } from './workflowProgress'

const copy = {
  resultTitle: 'Deployment Result',
  resultPending: 'Awaiting deployment result',
}

describe('buildWorkflowProgress', () => {
  it('preserves a linear workflow and its current position', () => {
    const result = buildWorkflowProgress(linearDocument(), 'deploying', copy)

    expect(result.stages.map((stage) => stage.title)).toEqual([
      'Pending Review',
      'Approved',
      'Deploying',
      'Succeeded',
    ])
    expect(result.currentIndex).toBe(2)
  })

  it('does not invent history when multiple branches reach the current state', () => {
    const result = buildWorkflowProgress(ambiguousDocument(), 'joined', copy)

    expect(result.stages.map((stage) => stage.title)).toEqual(['Joined'])
    expect(result.currentIndex).toBe(0)
  })
})

function linearDocument(): ReleaseWorkflowDocument {
  return {
    initialState: 'review',
    states: [
      { key: 'review', name: 'Pending Review', type: 'Review' },
      { key: 'approved', name: 'Approved', type: 'ManualAction' },
      { key: 'deploying', name: 'Deploying', type: 'Deployment' },
      { key: 'succeeded', name: 'Succeeded', type: 'Terminal' },
    ],
    transitions: [
      transition('review', 'approved'),
      transition('approved', 'deploying'),
      transition('deploying', 'succeeded'),
    ],
  }
}

function ambiguousDocument(): ReleaseWorkflowDocument {
  return {
    initialState: 'start',
    states: [
      { key: 'start', name: 'Start', type: 'Start' },
      { key: 'left', name: 'Left', type: 'ManualAction' },
      { key: 'right', name: 'Right', type: 'ManualAction' },
      { key: 'joined', name: 'Joined', type: 'Deployment' },
    ],
    transitions: [
      transition('start', 'left'),
      transition('start', 'right'),
      transition('left', 'joined'),
      transition('right', 'joined'),
    ],
  }
}

function transition(from: string, to: string) {
  return {
    key: `${from}-${to}`,
    from,
    to,
    trigger: 'Manual' as const,
    permission: 'deployment_request.update',
    conditions: [],
  }
}

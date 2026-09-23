import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RuntimeTopologyPanel } from './RuntimeTopologyPanel'

const api = vi.hoisted(() => ({
  topology: vi.fn(),
  resource: vi.fn(),
  events: vi.fn(),
  logs: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  useGetCatalogApplicationRuntimeTopology: api.topology,
  useGetCatalogApplicationRuntimeResource: api.resource,
  useListCatalogApplicationRuntimeEvents: api.events,
  useGetCatalogApplicationRuntimePodLogs: api.logs,
}))

vi.mock('@xyflow/react', () => ({
  Background: () => null,
  Controls: () => null,
  MiniMap: () => null,
  Handle: () => null,
  Position: { Left: 'left', Right: 'right' },
  MarkerType: { ArrowClosed: 'arrowclosed' },
  ReactFlow: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="runtime-flow">{children}</div>
  ),
}))

describe('RuntimeTopologyPanel', () => {
  beforeEach(() => {
    api.topology.mockReturnValue({
      data: {
        status: 200,
        data: {
          data: {
            applicationId: 'application-1',
            view: 'resources',
            observedAt: '2026-09-23T08:00:00Z',
            nodes: [],
            edges: [],
            warnings: [],
            partial: false,
          },
        },
      },
      isPending: false,
      isFetching: false,
      isError: false,
      refetch: vi.fn(),
    })
    const idle = { isPending: false, data: undefined }
    api.resource.mockReturnValue(idle)
    api.events.mockReturnValue(idle)
    api.logs.mockReturnValue(idle)
  })

  it('polls every five seconds only while deployment observation is active', () => {
    const { rerender } = render(
      <RuntimeTopologyPanel applicationId="application-1" active />,
    )

    expect(api.topology.mock.calls.at(-1)?.[2].query).toMatchObject({
      refetchInterval: 5000,
      refetchIntervalInBackground: false,
    })

    rerender(
      <RuntimeTopologyPanel applicationId="application-1" currentLiveState />,
    )
    expect(api.topology.mock.calls.at(-1)?.[2].query).toMatchObject({
      refetchInterval: false,
    })
    expect(screen.getByText('runtimeTopology.current')).toBeInTheDocument()
  })
})

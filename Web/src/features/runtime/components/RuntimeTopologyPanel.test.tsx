import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { RuntimeTopologyPanel } from './RuntimeTopologyPanel'

const api = vi.hoisted(() => ({
  topology: vi.fn(),
  resource: vi.fn(),
  events: vi.fn(),
  logs: vi.fn(),
}))
const flow = vi.hoisted(() => ({ render: vi.fn() }))

vi.mock('@/shared/theme/useThemePreference', () => ({
  useThemePreference: () => ({ resolvedTheme: 'dark' }),
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
  ReactFlow: (props: {
    children: React.ReactNode
    colorMode: string
    minZoom: number
    nodes: unknown[]
    fitView: boolean
    defaultViewport?: { x: number; y: number; zoom: number }
    onMoveEnd?: (
      event: MouseEvent | TouchEvent | null,
      viewport: { x: number; y: number; zoom: number },
    ) => void
  }) => {
    flow.render(props)
    return <div data-testid="runtime-flow">{props.children}</div>
  },
}))

describe('RuntimeTopologyPanel', () => {
  beforeEach(() => {
    flow.render.mockClear()
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

  afterEach(cleanup)

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

  it('disables topology fetching when the active panel is hidden', () => {
    render(
      <RuntimeTopologyPanel
        applicationId="application-1"
        active
        visible={false}
      />,
    )

    expect(api.topology.mock.lastCall?.[2].query).toMatchObject({
      enabled: false,
      refetchInterval: false,
      refetchIntervalInBackground: false,
    })
  })

  it('renders unresolved network evidence separately from unavailable evidence', () => {
    const result = api.topology()
    result.data.data.data.nodes = [
      {
        id: 'v1/Pod/payments/api-1',
        group: '',
        version: 'v1',
        kind: 'Pod',
        namespace: 'payments',
        name: 'api-1',
        healthStatus: 'Healthy',
        healthMessage: '',
        orphaned: false,
        images: [],
        info: [],
        ingress: [],
        externalUrls: [],
      },
    ]
    result.data.data.data.warnings = ['network_evidence_unresolved']

    render(<RuntimeTopologyPanel applicationId="application-1" />)

    expect(
      screen.getByText('runtimeTopology.warnings.network_evidence_unresolved'),
    ).toBeInTheDocument()
    expect(
      screen.queryByText(
        'runtimeTopology.warnings.network_evidence_unavailable',
      ),
    ).toBeNull()
  })

  it.each([
    'network_evidence_unavailable',
    'network_evidence_unresolved',
    'node_limit',
    'edge_limit',
  ])('keeps %s visible when the topology has no nodes', (warning) => {
    const result = api.topology()
    result.data.data.data.warnings = [warning]
    result.data.data.data.partial = true

    render(<RuntimeTopologyPanel applicationId="application-1" />)

    expect(screen.getByText('runtimeTopology.empty')).toBeInTheDocument()
    expect(
      screen.getByText(`runtimeTopology.warnings.${warning}`),
    ).toBeInTheDocument()
    expect(screen.queryByTestId('runtime-flow')).not.toBeInTheDocument()
  })

  it('shows only the empty state when there are no nodes or warnings', () => {
    render(<RuntimeTopologyPanel applicationId="application-1" />)

    expect(screen.getByText('runtimeTopology.empty')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('keeps API failures distinct from an empty topology', () => {
    api.topology.mockReturnValue({
      data: undefined,
      isPending: false,
      isFetching: false,
      isError: true,
      refetch: vi.fn(),
    })

    render(<RuntimeTopologyPanel applicationId="application-1" />)

    expect(screen.getByText('runtimeTopology.unavailable')).toBeInTheDocument()
    expect(screen.queryByText('runtimeTopology.empty')).not.toBeInTheDocument()
  })

  it('passes the resolved theme and a readable zoom floor to the canvas', () => {
    const result = api.topology()
    result.data.data.data.nodes = [
      {
        id: 'v1/Pod/payments/api-1',
        group: '',
        version: 'v1',
        kind: 'Pod',
        namespace: 'payments',
        name: 'api-1',
        healthStatus: 'Healthy',
        healthMessage: '',
        orphaned: false,
        images: [],
        info: [],
        ingress: [],
        externalUrls: [],
      },
    ]

    render(
      <RuntimeTopologyPanel
        applicationId="application-1"
        applicationName="payments"
      />,
    )

    expect(flow.render.mock.lastCall?.[0]).toMatchObject({
      colorMode: 'dark',
      minZoom: 0.75,
      nodes: [
        expect.objectContaining({ type: 'application', selectable: false }),
        expect.objectContaining({ type: 'runtime' }),
      ],
    })
    expect(
      screen.getAllByText('runtimeTopology.legend.presentation').length,
    ).toBeGreaterThan(0)
  })

  it('restores each Application viewport after switching between Applications', () => {
    const result = api.topology()
    result.data.data.data.nodes = [
      {
        id: 'v1/Pod/payments/api-1',
        group: '',
        version: 'v1',
        kind: 'Pod',
        namespace: 'payments',
        name: 'api-1',
        healthStatus: 'Healthy',
        healthMessage: '',
        orphaned: false,
        images: [],
        info: [],
        ingress: [],
        externalUrls: [],
      },
    ]
    const { rerender } = render(
      <RuntimeTopologyPanel applicationId="application-1" />,
    )
    const first = flow.render.mock.lastCall?.[0]
    expect(first.fitView).toBe(true)
    first.onMoveEnd?.(null, { x: 120, y: -30, zoom: 1.2 })

    rerender(<RuntimeTopologyPanel applicationId="application-2" />)
    const second = flow.render.mock.lastCall?.[0]
    expect(api.topology.mock.lastCall?.[0]).toBe('application-2')
    expect(second.fitView).toBe(true)
    second.onMoveEnd?.(null, { x: -80, y: 50, zoom: 0.9 })

    rerender(<RuntimeTopologyPanel applicationId="application-1" />)
    expect(api.topology.mock.lastCall?.[0]).toBe('application-1')
    expect(flow.render.mock.lastCall?.[0]).toMatchObject({
      fitView: false,
      defaultViewport: { x: 120, y: -30, zoom: 1.2 },
    })

    rerender(<RuntimeTopologyPanel applicationId="application-2" />)
    expect(flow.render.mock.lastCall?.[0]).toMatchObject({
      fitView: false,
      defaultViewport: { x: -80, y: 50, zoom: 0.9 },
    })
  })

  it('keeps resource and network viewports separate for the same Application', () => {
    const result = api.topology()
    result.data.data.data.nodes = [
      {
        id: 'v1/Pod/payments/api-1',
        group: '',
        version: 'v1',
        kind: 'Pod',
        namespace: 'payments',
        name: 'api-1',
        healthStatus: 'Healthy',
        healthMessage: '',
        orphaned: false,
        images: [],
        info: [],
        ingress: [],
        externalUrls: [],
      },
    ]

    render(<RuntimeTopologyPanel applicationId="application-1" />)
    flow.render.mock.lastCall?.[0].onMoveEnd?.(null, {
      x: 75,
      y: -25,
      zoom: 1.1,
    })

    fireEvent.click(screen.getByText('runtimeTopology.views.network'))
    expect(api.topology.mock.lastCall?.[1]).toEqual({ view: 'network' })
    expect(flow.render.mock.lastCall?.[0].fitView).toBe(true)
    flow.render.mock.lastCall?.[0].onMoveEnd?.(null, {
      x: -40,
      y: 45,
      zoom: 0.85,
    })

    fireEvent.click(screen.getByText('runtimeTopology.views.resources'))
    expect(api.topology.mock.lastCall?.[1]).toEqual({ view: 'resources' })
    expect(flow.render.mock.lastCall?.[0]).toMatchObject({
      fitView: false,
      defaultViewport: { x: 75, y: -25, zoom: 1.1 },
    })
  })
})

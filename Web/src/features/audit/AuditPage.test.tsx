import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'
import tagStyles from '@/shared/tag/SemanticTag.module.css'

import { AuditPage } from './AuditPage'

const api = vi.hoisted(() => ({
  capabilities: vi.fn(),
  filterOptions: vi.fn(),
  events: vi.fn(),
  detail: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  getGetAuditCapabilitiesQueryKey: () => ['/api/v1/audit/capabilities'],
  getGetAuditEventQueryKey: (id: string) => [
    '/api/v1/audit/events/{auditEventId}',
    id,
  ],
  getListAuditEventsQueryKey: (params: unknown) => [
    '/api/v1/audit/events',
    params,
  ],
  getListAuditFilterOptionsQueryKey: (params: unknown) => [
    '/api/v1/audit/filter-options',
    params,
  ],
  useGetAuditCapabilities: api.capabilities,
  useListAuditFilterOptions: api.filterOptions,
  useListAuditEvents: api.events,
  useGetAuditEvent: api.detail,
}))

const event = {
  id: '019d0000-0000-7000-8000-000000000001',
  occurredAt: '2026-09-17T01:00:00Z',
  actor: { kind: 'user', displayName: 'admin' },
  action: 'workflow.version.published',
  resource: { type: 'release_workflow', id: 'workflow-1' },
  scope: { kind: 'project', resolution: 'resolved' },
  requestId: 'request-1',
  hasMetadata: true,
}

function query(data: unknown, meta?: unknown) {
  return {
    isPending: false,
    isFetching: false,
    isError: false,
    refetch: vi.fn(),
    data: { status: 200, data: { data, meta } },
  }
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <App>
          <AuditPage principalID="audit-user" />
        </App>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AuditPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.capabilities.mockReturnValue(
      query({
        visible: true,
        scopeRoots: [
          {
            kind: 'project',
            id: '019d0000-0000-7000-8000-000000000010',
            label: 'Project A',
          },
        ],
      }),
    )
    api.events.mockReturnValue(
      query([event], {
        requestId: 'request',
        timestamp: '2026-09-17T01:00:00Z',
        nextCursor: 'next-page',
        hasMore: true,
      }),
    )
    api.filterOptions.mockImplementation((params: { field: string }) =>
      query(
        params.field === 'action'
          ? [
              {
                value: 'workflow.version.published',
                label: 'workflow.version.published',
              },
            ]
          : [],
      ),
    )
    api.detail.mockImplementation((id: string) =>
      id
        ? query({
            ...event,
            metadata: { safe: 'value' },
            metadataTruncated: true,
          })
        : { isPending: false, isError: false },
    )
  })

  afterEach(cleanup)

  it('renders only server-visible scope roots and keeps metadata out of the list', () => {
    renderPage()

    expect(screen.getByText('Project A')).toBeInTheDocument()
    expect(screen.getAllByText('workflow.version.published')).not.toHaveLength(
      0,
    )
    expect(screen.queryByText('value')).not.toBeInTheDocument()
    expect(api.capabilities).toHaveBeenCalledWith({
      query: expect.objectContaining({
        queryKey: ['/api/v1/audit/capabilities', 'audit-user'],
      }),
    })
    expect(api.events).toHaveBeenCalledWith(
      expect.objectContaining({
        scopeId: '019d0000-0000-7000-8000-000000000010',
      }),
      expect.objectContaining({
        query: expect.objectContaining({
          queryKey: expect.arrayContaining(['audit-user']),
        }),
      }),
    )
  })

  it('renders desktop and mobile Scope labels with the neutral semantic tone', () => {
    renderPage()

    const scopeTags = screen.getAllByText('project')
    expect(scopeTags).toHaveLength(2)
    for (const tag of scopeTags) expect(tag).toHaveClass(tagStyles.neutral)
  })

  it('opens safe metadata detail and advances with the server cursor', async () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: /查看/ }))
    expect(await screen.findByText('安全 Metadata')).toBeInTheDocument()
    expect(screen.getByText(/"safe": "value"/)).toBeInTheDocument()
    expect(screen.getByText('部分 Metadata 已遮罩或截斷。')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '下一頁' }))
    await waitFor(() =>
      expect(api.events).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: 'next-page' }),
        expect.anything(),
      ),
    )
  })

  it('returns to the first cursor when refreshing a later page', async () => {
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: '下一頁' }))
    await waitFor(() =>
      expect(api.events).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: 'next-page' }),
        expect.anything(),
      ),
    )

    fireEvent.click(screen.getByRole('button', { name: '重新整理' }))
    await waitFor(() =>
      expect(api.events).toHaveBeenLastCalledWith(
        expect.objectContaining({ cursor: undefined }),
        expect.anything(),
      ),
    )
  })

  it('shows an empty state without inventing client-side permissions', () => {
    api.events.mockReturnValue(
      query([], {
        requestId: 'request',
        timestamp: '2026-09-17T01:00:00Z',
        hasMore: false,
      }),
    )
    renderPage()

    expect(screen.getAllByText('此範圍目前沒有稽核事件。')).not.toHaveLength(0)
  })

  it('shows a dependency error and retries the same server query', () => {
    const refetch = vi.fn()
    api.events.mockReturnValue({
      isPending: false,
      isFetching: false,
      isError: true,
      refetch,
    })
    renderPage()

    expect(screen.getByText('無法取得稽核紀錄')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '重試' }))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('normalizes filters before requesting the first page', async () => {
    renderPage()

    fireEvent.change(screen.getByLabelText('操作名稱'), {
      target: { value: '  workflow.version.published  ' },
    })
    fireEvent.click(screen.getByRole('button', { name: '套用篩選' }))

    await waitFor(() =>
      expect(api.events).toHaveBeenLastCalledWith(
        expect.objectContaining({
          action: 'workflow.version.published',
          cursor: undefined,
        }),
        expect.anything(),
      ),
    )
  })

  it('searches authorized dropdown options and keeps the cache principal-aware', async () => {
    renderPage()

    fireEvent.change(screen.getByLabelText('操作名稱'), {
      target: { value: 'workflow' },
    })

    await waitFor(
      () =>
        expect(api.filterOptions).toHaveBeenCalledWith(
          expect.objectContaining({
            field: 'action',
            search: 'workflow',
            scopeId: '019d0000-0000-7000-8000-000000000010',
          }),
          expect.objectContaining({
            query: expect.objectContaining({
              queryKey: expect.arrayContaining(['audit-user']),
            }),
          }),
        ),
      { timeout: 1_000 },
    )
  })

  it('renders a loading state while capabilities are being resolved', () => {
    api.capabilities.mockReturnValue({ isPending: true })
    const { container } = renderPage()

    expect(container.querySelector('.ant-skeleton')).toBeInTheDocument()
    expect(screen.queryByText('稽核紀錄')).not.toBeInTheDocument()
  })

  it('does not expose Audit Trail when the server capability is hidden', () => {
    api.capabilities.mockReturnValue(query({ visible: false, scopeRoots: [] }))
    renderPage()

    expect(screen.getByText('找不到稽核紀錄')).toBeInTheDocument()
    expect(
      screen.queryByText('workflow.version.published'),
    ).not.toBeInTheDocument()
  })
})

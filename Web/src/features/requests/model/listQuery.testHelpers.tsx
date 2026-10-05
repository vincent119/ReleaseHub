import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfigProvider } from 'antd'
import { HttpResponse, http } from 'msw'
import type { PropsWithChildren } from 'react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'

import { DeploymentRequestStatus } from '@/generated/model'
import type {
  CatalogOrganizationNode,
  DeploymentRequestSummary,
} from '@/generated/model'
import i18n from '@/shared/i18n/config'

export const testScope = {
  organizationId: '00000000-0000-4000-8000-000000000001',
  projectId: '00000000-0000-4000-8000-000000000002',
  environmentId: '00000000-0000-4000-8000-000000000003',
}

export const otherScope = {
  ...testScope,
  environmentId: '00000000-0000-4000-8000-000000000004',
}

export const testCatalog: CatalogOrganizationNode[] = [
  {
    id: testScope.organizationId,
    name: '測試組織',
    version: 1,
    isDefault: true,
    canRename: false,
    canDelete: false,
    canCreateProject: false,
    projects: [
      {
        id: testScope.projectId,
        name: 'Project A',
        canManage: false,
        environments: [testScope, otherScope].map((scope, index) => ({
          id: scope.environmentId,
          name: index ? 'Global' : 'Production',
          type: 'Production',
          applications: [],
        })),
      },
    ],
  },
]

export function requestRows(count = 137): DeploymentRequestSummary[] {
  return Array.from({ length: count }, (_, index) => ({
    ...testScope,
    id: `00000000-0000-4000-8000-${String(index + 100).padStart(12, '0')}`,
    title:
      index === 131
        ? '後頁部署 %_\\特殊'
        : `部署 ${String(index).padStart(3, '0')}`,
    status: Object.values(DeploymentRequestStatus)[index % 10],
    classification: 'Standard',
    activeVersionNumber: 1,
    applicationCount: 1,
    scheduleState: 'Ready',
    scheduleReason: 'Ready',
    nextEligibleAt: '2026-10-02T00:00:00Z',
    updatedAt: '2026-10-02T00:00:00Z',
  }))
}

export function listTestWrapper() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={client}>
        <I18nextProvider i18n={i18n}>
          <ConfigProvider theme={{ token: { motion: false } }}>
            <MemoryRouter>{children}</MemoryRouter>
          </ConfigProvider>
        </I18nextProvider>
      </QueryClientProvider>
    )
  }
  return { client, Wrapper }
}

export function listTestAPI(rows = requestRows()) {
  const calls: URLSearchParams[] = []
  const cursors = new Map<string, { offset: number; context: string }>()
  const handler = http.get('/api/v1/deployment-requests', ({ request }) => {
    const params = new URL(request.url).searchParams
    calls.push(params)
    const limit = Number(params.get('limit'))
    const search = params.get('search') ?? ''
    const status = params.get('status')
    const context = JSON.stringify([
      params.get('organizationId'),
      params.get('projectId'),
      params.get('environmentId'),
      limit,
      search,
      status,
    ])
    const cursor = params.get('cursor')
    const position = cursor ? cursors.get(cursor) : { offset: 0, context }
    if (!position || position.context !== context)
      return HttpResponse.json(
        { code: 'INVALID_REQUEST', retryable: false, category: 'validation' },
        { status: 400 },
      )
    const filtered = rows.filter(
      (row) => row.title.includes(search) && (!status || row.status === status),
    )
    const data = filtered.slice(position.offset, position.offset + limit)
    const hasMore = position.offset + limit < filtered.length
    // 測試端游標含保留字元，確認前端只轉送 Server 原值，不解碼或組合。
    const nextCursor = hasMore ? `opaque+?&/${calls.length}` : undefined
    if (nextCursor)
      cursors.set(nextCursor, { offset: position.offset + limit, context })
    return HttpResponse.json({
      data,
      meta: {
        requestId: 'list-test',
        timestamp: '2026-10-02T00:00:00Z',
        hasMore,
        ...(nextCursor ? { nextCursor } : {}),
      },
    })
  })
  return { calls, handler }
}

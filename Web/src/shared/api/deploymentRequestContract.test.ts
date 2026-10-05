import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import {
  getListDeploymentRequestsQueryKey,
  getListDeploymentRequestsUrl,
  listDeploymentRequests,
} from '@/generated/api'
import type {
  DeploymentRequestListResponse,
  ErrorResponse,
  ListDeploymentRequestsParams,
} from '@/generated/model'

const params: ListDeploymentRequestsParams = {
  organizationId: '00000000-0000-4000-8000-000000000001',
  projectId: '00000000-0000-4000-8000-000000000002',
  environmentId: '00000000-0000-4000-8000-000000000003',
  limit: 20,
  search: '部署 %_\\?&',
  status: 'Succeeded',
  cursor: 'server-issued-cursor',
}

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('Deployment Request 產生碼契約', () => {
  it('保留 scope、filter 與 cursor 的原值並正確編碼 URL', () => {
    const url = new URL(
      getListDeploymentRequestsUrl(params),
      'https://test.local',
    )
    expect(url.pathname).toBe('/api/v1/deployment-requests')
    for (const [key, value] of Object.entries(params)) {
      expect(url.searchParams.get(key)).toBe(String(value))
      expect(url.searchParams.getAll(key)).toHaveLength(1)
    }
  })

  it('query key 隔離完整 scope、filter、limit 與 cursor', () => {
    const key = getListDeploymentRequestsQueryKey(params)
    expect(key).toEqual(['/api/v1/deployment-requests', params])
    for (const changes of [
      { organizationId: 'other-organization' },
      { projectId: 'other-project' },
      { environmentId: 'other-environment' },
      { limit: 50 },
      { search: 'other-search' },
      { status: 'Failed' as const },
      { cursor: 'other-cursor' },
    ]) {
      expect(
        getListDeploymentRequestsQueryKey({ ...params, ...changes }),
      ).not.toEqual(key)
    }
  })

  it('成功回應保留 Server 的 page meta，不產生總數', async () => {
    const page: DeploymentRequestListResponse = {
      data: [],
      meta: {
        requestId: 'query-page',
        timestamp: '2026-10-01T00:00:00Z',
        hasMore: true,
        nextCursor: 'next-server-cursor',
      },
    }
    server.use(
      http.get('/api/v1/deployment-requests', () => HttpResponse.json(page)),
    )
    const response = await listDeploymentRequests(params)
    expect(response.status).toBe(200)
    expect(response.data).toEqual(page)
    expect(response.data).not.toHaveProperty('total')
  })

  it('400 保留 INVALID_REQUEST 錯誤，不轉成空陣列成功回應', async () => {
    const error: ErrorResponse = {
      code: 'INVALID_REQUEST',
      category: 'validation',
      message: '查詢條件無效',
      requestId: 'query-error',
      retryable: false,
    }
    server.use(
      http.get('/api/v1/deployment-requests', () =>
        HttpResponse.json(error, { status: 400 }),
      ),
    )
    const response = await listDeploymentRequests(params)
    expect(response.status).toBe(400)
    expect(response.data).toEqual(error)
    expect(response.data).not.toHaveProperty('data')
    expect(response.data).not.toHaveProperty('meta')
  })
})

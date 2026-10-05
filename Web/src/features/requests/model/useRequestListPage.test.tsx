import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import { HttpResponse, http } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { getListDeploymentRequestsQueryKey } from '@/generated/api'
import {
  listTestAPI,
  listTestWrapper,
  otherScope,
  requestRows,
  testScope,
} from './listQuery.testHelpers'
import { useRequestListPage } from './useRequestListPage'
import {
  initialRequestListCriteria,
  type RequestListCriteria,
} from './listQuery'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => {
  cleanup()
  server.resetHandlers()
})
afterAll(() => server.close())

describe('Request 列表查詢與游標隔離', () => {
  const contextCases: [string, typeof testScope, RequestListCriteria][] = [
    [
      'organization',
      { ...testScope, organizationId: '00000000-0000-4000-8000-000000000005' },
      { search: '', limit: 20 },
    ],
    [
      'project',
      { ...testScope, projectId: '00000000-0000-4000-8000-000000000006' },
      { search: '', limit: 20 },
    ],
    ['environment', otherScope, { search: '', limit: 20 }],
    ['search', testScope, { search: '部署', limit: 20 }],
    ['status', testScope, { search: '', limit: 20, status: 'Failed' }],
    ['limit', testScope, { search: '', limit: 50 }],
  ]
  it.each(contextCases)(
    '第 2 頁改變 %s，第一個新條件查詢不混入舊游標',
    async (_, scope, criteria) => {
      const api = listTestAPI()
      server.use(api.handler)
      const { Wrapper } = listTestWrapper()
      const { result, rerender } = renderHook(
        ({ scope, criteria }) => useRequestListPage(scope, criteria),
        {
          wrapper: Wrapper,
          initialProps: {
            scope: testScope,
            criteria: initialRequestListCriteria,
          },
        },
      )
      await waitFor(() =>
        expect(result.current.requests.data?.status).toBe(200),
      )
      const first = result.current.requests.data
      if (first?.status !== 200 || !first.data.meta.nextCursor)
        throw new Error('缺少游標')
      const cursor = first.data.meta.nextCursor
      act(() => result.current.dispatch({ type: 'next', cursor }))
      await waitFor(() =>
        expect(result.current.requests.isFetching).toBe(false),
      )
      expect(result.current.position.index).toBe(1)
      const callCount = api.calls.length
      rerender({ scope, criteria })
      await waitFor(() =>
        expect(result.current.requests.isFetching).toBe(false),
      )
      expect(result.current.position).toMatchObject({
        cursors: [undefined],
        index: 0,
      })
      expect(api.calls.slice(callCount)).toHaveLength(1)
      expect(api.calls.at(-1)?.has('cursor')).toBe(false)
      expect(api.calls.at(-1)?.get('limit')).toBe(String(criteria.limit))
      expect(api.calls.at(-1)?.get('status')).toBe(criteria.status ?? null)
      expect(api.calls.at(-1)?.get('search')).toBe(criteria.search || null)
      expect(api.calls.at(-1)?.get('environmentId')).toBe(scope.environmentId)
    },
  )

  it.each([20, 50, 100] as const)(
    '%s 筆遍歷 137 筆，只使用 Server 游標且沒有重複漏筆',
    async (limit) => {
      const api = listTestAPI()
      server.use(api.handler)
      const { Wrapper } = listTestWrapper()
      const { result } = renderHook(
        () => useRequestListPage(testScope, { search: '', limit }),
        { wrapper: Wrapper },
      )
      const seen: string[] = []
      let page = 0
      while (true) {
        await waitFor(() =>
          expect(result.current.requests.isFetching).toBe(false),
        )
        const response = result.current.requests.data
        expect(response?.status).toBe(200)
        if (response?.status !== 200) throw new Error('未取得列表')
        seen.push(...response.data.data.map((row) => row.id))
        expect(response.data.data.length).toBeLessThanOrEqual(limit)
        expect(result.current.position.index).toBe(page)
        if (!response.data.meta.hasMore) {
          expect(response.data.meta.nextCursor).toBeUndefined()
          break
        }
        const cursor = response.data.meta.nextCursor
        if (!cursor) throw new Error('Server 缺少下一頁游標')
        act(() => result.current.dispatch({ type: 'next', cursor }))
        page++
      }
      expect(seen).toEqual(requestRows().map((row) => row.id))
      expect(new Set(seen).size).toBe(137)
      expect(api.calls).toHaveLength(Math.ceil(137 / limit))
      expect(api.calls[0].has('cursor')).toBe(false)
      for (const call of api.calls) {
        expect(call.get('limit')).toBe(String(limit))
        expect(call.get('environmentId')).toBe(testScope.environmentId)
        expect(call.has('total')).toBe(false)
      }
    },
  )

  it('上一頁後可再次前進；刷新與 query invalidation 回第一頁並重新取得資料', async () => {
    const api = listTestAPI()
    server.use(api.handler)
    const { Wrapper, client } = listTestWrapper()
    const { result } = renderHook(
      () => useRequestListPage(testScope, { search: '', limit: 20 }),
      { wrapper: Wrapper },
    )
    await waitFor(() => expect(result.current.requests.data?.status).toBe(200))
    const first = result.current.requests.data
    if (first?.status !== 200 || !first.data.meta.nextCursor)
      throw new Error('缺少游標')
    const cursor = first.data.meta.nextCursor
    act(() => result.current.dispatch({ type: 'next', cursor }))
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    act(() => result.current.dispatch({ type: 'previous' }))
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    expect(result.current.position.index).toBe(0)
    act(() => result.current.dispatch({ type: 'next', cursor }))
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    expect(result.current.position.index).toBe(1)
    act(() => result.current.dispatch({ type: 'refresh', updated: true }))
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    expect(result.current.position).toMatchObject({
      cursors: [undefined],
      index: 0,
      generation: 1,
      updated: true,
    })
    const fresh = result.current.requests.data
    if (fresh?.status !== 200 || !fresh.data.meta.nextCursor)
      throw new Error('缺少游標')
    const freshCursor = fresh.data.meta.nextCursor
    act(() =>
      result.current.dispatch({
        type: 'next',
        cursor: freshCursor,
      }),
    )
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    await act(async () => {
      await client.invalidateQueries({
        queryKey: getListDeploymentRequestsQueryKey(),
      })
    })
    await waitFor(() => expect(result.current.requests.isFetching).toBe(false))
    expect(result.current.position).toMatchObject({
      cursors: [undefined],
      index: 0,
      generation: 2,
      updated: true,
    })
    expect(api.calls.at(-1)?.has('cursor')).toBe(false)
  })

  it('不同 scope／條件的遲到回應不覆蓋目前列表，重新整理世代不送 API', async () => {
    let release: (() => void) | undefined
    const delayed = new Promise<void>((resolve) => {
      release = resolve
    })
    let oldResponded = false
    const calls: URLSearchParams[] = []
    server.use(
      http.get('/api/v1/deployment-requests', async ({ request }) => {
        const params = new URL(request.url).searchParams
        calls.push(params)
        const old = params.get('environmentId') === testScope.environmentId
        if (old) {
          await delayed
          oldResponded = true
        }
        return HttpResponse.json({
          data: [{ ...requestRows(1)[0], title: old ? '舊列表' : '新列表' }],
          meta: {
            requestId: 'race',
            timestamp: '2026-10-02T00:00:00Z',
            hasMore: false,
          },
        })
      }),
    )
    const { Wrapper } = listTestWrapper()
    const criteria: RequestListCriteria = { search: '', limit: 20 }
    const { result, rerender } = renderHook(
      ({ scope, criteria }) => useRequestListPage(scope, criteria),
      {
        wrapper: Wrapper,
        initialProps: { scope: testScope, criteria },
      },
    )
    await waitFor(() => expect(calls).toHaveLength(1))
    rerender({
      scope: otherScope,
      criteria: { search: '新', status: 'Failed', limit: 50 },
    })
    await waitFor(() => expect(result.current.requests.data?.status).toBe(200))
    act(() => release?.())
    await waitFor(() => expect(oldResponded).toBe(true))
    const response = result.current.requests.data
    expect(response?.status === 200 && response.data.data[0].title).toBe(
      '新列表',
    )
    expect(calls[1].get('search')).toBe('新')
    expect(calls[1].get('status')).toBe('Failed')
    expect(calls[1].get('limit')).toBe('50')
    expect(calls[1].has('generation')).toBe(false)
  })
})

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import type { PropsWithChildren } from 'react'
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest'

import { getGetSystemStatusQueryKey, useGetSystemStatus } from '@/generated/api'

const server = setupServer(
  http.get('/api/v1/system/status', () =>
    HttpResponse.json({
      data: { name: 'ReleaseHub', version: 'test', tenancyMode: 'single' },
    }),
  ),
)

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

describe('generated API hooks', () => {
  it('preserves the generated query key and response envelope', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const wrapper = ({ children }: PropsWithChildren) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )

    const { result } = renderHook(() => useGetSystemStatus(), { wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.queryKey).toEqual(getGetSystemStatusQueryKey())
    expect(result.current.data).toMatchObject({
      status: 200,
      data: { data: { name: 'ReleaseHub', tenancyMode: 'single' } },
    })
  })
})

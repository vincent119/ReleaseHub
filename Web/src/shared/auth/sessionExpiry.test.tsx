import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, waitFor } from '@testing-library/react'
import type { PropsWithChildren } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { getGetAuthSessionQueryKey } from '@/generated/api'

import {
  createSessionAwareQueryClient,
  isSessionExpiryResponse,
  useSessionExpiryCheck,
} from './sessionExpiry'

afterEach(() => {
  vi.useRealTimers()
})

describe('Session expiry response classifier', () => {
  it.each(['SESSION_REQUIRED', 'SESSION_INVALID'])(
    'accepts %s only with status 401',
    (code) => {
      expect(isSessionExpiryResponse({ status: 401, data: { code } })).toBe(
        true,
      )
    },
  )

  it.each([
    { status: 401, data: { code: 'INVALID_CREDENTIALS' } },
    { status: 403, data: { code: 'SESSION_INVALID' } },
    { status: 503, data: { code: 'SESSION_STATE_UNAVAILABLE' } },
    { status: 401, data: { error: { code: 'SESSION_INVALID' } } },
    new Error('network failure'),
  ])('rejects non-session response %#', (response) => {
    expect(isSessionExpiryResponse(response)).toBe(false)
  })
})

describe('Session-aware QueryClient', () => {
  it.each(['query', 'mutation'] as const)(
    'synchronizes Auth Session after a protected %s response',
    async (source) => {
      const queryClient = createSessionAwareQueryClient()
      const invalidateQueries = vi
        .spyOn(queryClient, 'invalidateQueries')
        .mockResolvedValue()

      if (source === 'query') {
        await queryClient.fetchQuery({
          queryKey: ['protected-query'],
          queryFn: async () => ({
            status: 401,
            data: { code: 'SESSION_REQUIRED' },
          }),
        })
      } else {
        const mutation = queryClient.getMutationCache().build(queryClient, {
          mutationFn: async () => ({
            status: 401,
            data: { code: 'SESSION_REQUIRED' },
          }),
        })
        await mutation.execute(undefined)
      }

      await waitFor(() =>
        expect(invalidateQueries).toHaveBeenCalledWith({
          queryKey: getGetAuthSessionQueryKey(),
          exact: true,
        }),
      )
    },
  )

  it('does not recursively synchronize the Auth Session query itself', async () => {
    const queryClient = createSessionAwareQueryClient()
    const authSession = vi.fn().mockResolvedValue({
      status: 401,
      data: { code: 'SESSION_INVALID' },
    })

    await queryClient.fetchQuery({
      queryKey: getGetAuthSessionQueryKey(),
      queryFn: authSession,
    })
    await Promise.resolve()

    expect(authSession).toHaveBeenCalledTimes(1)
  })

  it('ignores non-session responses', async () => {
    const queryClient = createSessionAwareQueryClient()
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries')

    await queryClient.fetchQuery({
      queryKey: ['login'],
      queryFn: async () => ({
        status: 401,
        data: { code: 'INVALID_CREDENTIALS' },
      }),
    })

    expect(invalidateQueries).not.toHaveBeenCalled()
  })

  it('deduplicates concurrent session expiry responses', async () => {
    const queryClient = createSessionAwareQueryClient()
    let finishSynchronization: (() => void) | undefined
    const synchronization = new Promise<void>((resolve) => {
      finishSynchronization = resolve
    })
    const invalidateQueries = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockReturnValue(synchronization)

    await Promise.all([
      queryClient.fetchQuery({
        queryKey: ['protected-query-one'],
        queryFn: async () => ({
          status: 401,
          data: { code: 'SESSION_INVALID' },
        }),
      }),
      queryClient.fetchQuery({
        queryKey: ['protected-query-two'],
        queryFn: async () => ({
          status: 401,
          data: { code: 'SESSION_REQUIRED' },
        }),
      }),
    ])

    expect(invalidateQueries).toHaveBeenCalledTimes(1)
    finishSynchronization?.()
    await synchronization
  })
})

describe('Session expiry deadline', () => {
  it('checks the session once at the earlier deadline', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-14T08:00:00Z'))
    const queryClient = createSessionAwareQueryClient()
    const invalidateQueries = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockResolvedValue()

    function Probe() {
      useSessionExpiryCheck('2026-09-14T08:01:00Z', '2026-09-14T09:00:00Z')
      return null
    }

    render(<Probe />, { wrapper: createWrapper(queryClient) })

    act(() => vi.advanceTimersByTime(60_249))
    expect(invalidateQueries).not.toHaveBeenCalled()

    act(() => vi.advanceTimersByTime(1))
    expect(invalidateQueries).toHaveBeenCalledWith({
      queryKey: getGetAuthSessionQueryKey(),
      exact: true,
    })
  })

  it('replaces the old deadline when the session changes', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-14T08:00:00Z'))
    const queryClient = createSessionAwareQueryClient()
    const invalidateQueries = vi
      .spyOn(queryClient, 'invalidateQueries')
      .mockResolvedValue()

    function Probe({ idleExpiresAt }: { idleExpiresAt: string }) {
      useSessionExpiryCheck(idleExpiresAt, '2026-09-14T09:00:00Z')
      return null
    }

    const view = render(<Probe idleExpiresAt="2026-09-14T08:01:00Z" />, {
      wrapper: createWrapper(queryClient),
    })
    view.rerender(<Probe idleExpiresAt="2026-09-14T08:02:00Z" />)

    act(() => vi.advanceTimersByTime(60_250))
    expect(invalidateQueries).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(60_000))
    expect(invalidateQueries).toHaveBeenCalledTimes(1)
  })
})

function createWrapper(
  queryClient: ReturnType<typeof createSessionAwareQueryClient>,
) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
  }
}

import {
  MutationCache,
  QueryCache,
  QueryClient,
  useQueryClient,
} from '@tanstack/react-query'
import { useEffect } from 'react'

import { getGetAuthSessionQueryKey } from '@/generated/api'
import { parseAPIErrorResponse } from '@/shared/api/apiError'

const sessionErrorCodes = new Set(['SESSION_REQUIRED', 'SESSION_INVALID'])
const maximumTimerDelay = 2_147_483_647
const deadlineTolerance = 250

export function isSessionExpiryResponse(value: unknown): boolean {
  const error = parseAPIErrorResponse(value)
  return error.status === 401 && sessionErrorCodes.has(error.code)
}

function isAuthSessionQuery(queryKey: readonly unknown[]): boolean {
  const authSessionQueryKey = getGetAuthSessionQueryKey()
  return (
    queryKey.length === authSessionQueryKey.length &&
    queryKey.every((part, index) => part === authSessionQueryKey[index])
  )
}

export function createSessionAwareQueryClient(): QueryClient {
  const clientReference: { current?: QueryClient } = {}
  let synchronization: Promise<void> | undefined

  const synchronizeSession = (response: unknown) => {
    const queryClient = clientReference.current
    if (!queryClient || !isSessionExpiryResponse(response) || synchronization)
      return

    synchronization = queryClient
      .invalidateQueries({ queryKey: getGetAuthSessionQueryKey(), exact: true })
      .finally(() => {
        synchronization = undefined
      })
  }

  const queryClient = new QueryClient({
    queryCache: new QueryCache({
      onSuccess: (response, query) => {
        if (!isAuthSessionQuery(query.queryKey)) {
          synchronizeSession(response)
        }
      },
    }),
    mutationCache: new MutationCache({
      onSuccess: (response) => synchronizeSession(response),
    }),
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  clientReference.current = queryClient

  return queryClient
}

export function useSessionExpiryCheck(
  idleExpiresAt?: string,
  absoluteExpiresAt?: string,
) {
  const queryClient = useQueryClient()

  useEffect(() => {
    if (!idleExpiresAt || !absoluteExpiresAt) return

    const deadline = Math.min(
      Date.parse(idleExpiresAt),
      Date.parse(absoluteExpiresAt),
    )
    if (!Number.isFinite(deadline)) return

    let timer: ReturnType<typeof setTimeout> | undefined
    const schedule = () => {
      const remaining = deadline + deadlineTolerance - Date.now()
      if (remaining <= 0) {
        void queryClient.invalidateQueries({
          queryKey: getGetAuthSessionQueryKey(),
          exact: true,
        })
        return
      }

      timer = setTimeout(schedule, Math.min(remaining, maximumTimerDelay))
    }

    schedule()
    return () => {
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [absoluteExpiresAt, idleExpiresAt, queryClient])
}

import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

import { getListNotificationsQueryKey } from '@/generated/api'

export function useNotificationEvents(enabled: boolean) {
  const queryClient = useQueryClient()
  useEffect(() => {
    if (!enabled || typeof EventSource === 'undefined') return
    const source = new EventSource('/api/v1/notifications/events')
    source.onmessage = () => void refreshNotificationQueries(queryClient)
    source.addEventListener(
      'deployment',
      () => void refreshNotificationQueries(queryClient),
    )
    const refresh = () => void refreshNotificationQueries(queryClient)
    source.addEventListener('deployment.request.created', refresh)
    source.addEventListener('deployment.execution.completed', refresh)
    source.addEventListener(
      'argocd.application.onboarding_state_changed',
      refresh,
    )
    return () => source.close()
  }, [enabled, queryClient])
}

async function refreshNotificationQueries(
  queryClient: ReturnType<typeof useQueryClient>,
) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: getListNotificationsQueryKey() }),
    queryClient.invalidateQueries({
      predicate: (query) =>
        String(query.queryKey[0]).startsWith('/api/v1/deployment'),
    }),
  ])
}

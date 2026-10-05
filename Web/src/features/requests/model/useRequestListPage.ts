import { hashKey, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useReducer } from 'react'

import {
  getListDeploymentRequestsQueryKey,
  useListDeploymentRequests,
} from '@/generated/api'

import type { RequestScope } from '../components/RequestScopeSelector'
import {
  initialRequestListPosition,
  requestListPositionReducer,
  type RequestListCriteria,
  type RequestListPosition,
  type RequestListPositionAction,
} from './listQuery'

const emptyID = '00000000-0000-0000-0000-000000000000'

function queryStateReducer(
  state: { context: string; position: RequestListPosition },
  action: RequestListPositionAction | { type: 'context'; value: string },
) {
  return action.type === 'context'
    ? { context: action.value, position: initialRequestListPosition }
    : { ...state, position: requestListPositionReducer(state.position, action) }
}

export function useRequestListPage(
  scope: RequestScope | undefined,
  criteria: RequestListCriteria,
) {
  const queryClient = useQueryClient()
  const context = hashKey([scope, criteria])
  const [state, updateState] = useReducer(queryStateReducer, {
    context,
    position: initialRequestListPosition,
  })
  // 在同次 render 排除舊條件的游標；不重掛元件，避免 StrictMode 取消後重發。
  const position =
    state.context === context ? state.position : initialRequestListPosition
  if (state.context !== context)
    updateState({ type: 'context', value: context })
  const dispatch = useCallback(
    (action: RequestListPositionAction) => updateState(action),
    [],
  )
  const cursor = position.cursors[position.index]
  const params = useMemo(
    () => ({
      organizationId: scope?.organizationId ?? emptyID,
      projectId: scope?.projectId ?? emptyID,
      environmentId: scope?.environmentId ?? emptyID,
      limit: criteria.limit,
      ...(criteria.search ? { search: criteria.search } : {}),
      ...(criteria.status ? { status: criteria.status } : {}),
      ...(cursor ? { cursor } : {}),
    }),
    [scope, criteria, cursor],
  )
  const queryKey = useMemo(
    () => [...getListDeploymentRequestsQueryKey(params), position.generation],
    [params, position.generation],
  )
  const currentHash = hashKey(queryKey)
  const requests = useListDeploymentRequests(params, {
    query: {
      enabled: Boolean(scope),
      queryKey,
      retry: false,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    },
  })

  useEffect(
    () =>
      queryClient.getQueryCache().subscribe((event) => {
        if (
          scope &&
          event.type === 'updated' &&
          event.action.type === 'invalidate' &&
          event.query.queryHash === currentHash
        ) {
          // 更新會移動排序位置；刷新世代不送往 Server，也不沿用舊頁快取。
          dispatch({ type: 'refresh', updated: true })
        }
      }),
    [queryClient, currentHash, scope, dispatch],
  )

  return { requests, position, dispatch }
}

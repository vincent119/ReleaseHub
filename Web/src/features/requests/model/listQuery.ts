import type { DeploymentRequestStatus } from '@/generated/model'

export type RequestListCriteria = {
  search: string
  status?: DeploymentRequestStatus
  limit: 20 | 50 | 100
}

export const initialRequestListCriteria: RequestListCriteria = {
  search: '',
  limit: 20,
}

export type RequestListCriteriaAction =
  | { type: 'search'; value: string }
  | { type: 'status'; value?: DeploymentRequestStatus }
  | { type: 'limit'; value: RequestListCriteria['limit'] }
  | { type: 'clear' }

export function requestListCriteriaReducer(
  state: RequestListCriteria,
  action: RequestListCriteriaAction,
): RequestListCriteria {
  switch (action.type) {
    case 'search':
      return { ...state, search: action.value.trim() }
    case 'status':
      return { ...state, status: action.value }
    case 'limit':
      return { ...state, limit: action.value }
    case 'clear':
      return { search: '', limit: state.limit }
  }
}

export type RequestListPosition = {
  cursors: (string | undefined)[]
  index: number
  generation: number
  updated: boolean
}

export const initialRequestListPosition: RequestListPosition = {
  cursors: [undefined],
  index: 0,
  generation: 0,
  updated: false,
}

export type RequestListPositionAction =
  | { type: 'next'; cursor: string }
  | { type: 'previous' }
  | { type: 'refresh'; updated: boolean }

export function requestListPositionReducer(
  state: RequestListPosition,
  action: RequestListPositionAction,
): RequestListPosition {
  switch (action.type) {
    case 'next':
      if (
        !action.cursor ||
        state.cursors.slice(0, state.index + 1).includes(action.cursor)
      )
        return state
      return {
        ...state,
        cursors: [...state.cursors.slice(0, state.index + 1), action.cursor],
        index: state.index + 1,
      }
    case 'previous':
      return { ...state, index: Math.max(0, state.index - 1) }
    case 'refresh':
      return {
        ...initialRequestListPosition,
        generation: state.generation + 1,
        updated: action.updated,
      }
  }
}

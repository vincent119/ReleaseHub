import { describe, expect, it } from 'vitest'

import type { APIErrorView } from '@/shared/api/apiError'

import { accessMutationErrorKey } from './mutationError'

function apiError(overrides: Partial<APIErrorView> = {}): APIErrorView {
  return {
    code: 'UNKNOWN_ERROR',
    category: 'internal',
    message: 'unknown error',
    requestId: '',
    retryable: false,
    status: 500,
    ...overrides,
  }
}

describe('accessMutationErrorKey', () => {
  it.each([
    ['ACCESS_SELF_DISABLE_FORBIDDEN', 'access.mutation.selfDisable'],
    ['ACCESS_LAST_PLATFORM_MANAGER', 'access.mutation.lastManager'],
    ['ACCESS_RESOURCE_PROTECTED', 'access.mutation.protected'],
  ])('maps stable code %s to %s', (code, expected) => {
    expect(accessMutationErrorKey(apiError({ code, status: 409 }))).toBe(
      expected,
    )
  })

  it('falls back to the category-compatible HTTP status mapping', () => {
    expect(accessMutationErrorKey(apiError({ status: 409 }))).toBe(
      'access.mutation.conflict',
    )
  })
})

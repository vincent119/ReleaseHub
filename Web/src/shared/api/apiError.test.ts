import { describe, expect, it } from 'vitest'

import {
  parseAPIError,
  parseAPIErrorResponse,
  resolveAPIErrorFeedback,
} from './apiError'

describe('parseAPIError', () => {
  it('parses the unified envelope without changing contract fields', () => {
    expect(
      parseAPIError(
        {
          code: 'WORKFLOW_VERSION_CONFLICT',
          category: 'conflict',
          message: 'Release workflow version changed',
          requestId: 'request-1',
          retryable: false,
          details: { currentVersion: 3 },
        },
        409,
      ),
    ).toEqual({
      code: 'WORKFLOW_VERSION_CONFLICT',
      category: 'conflict',
      message: 'Release workflow version changed',
      requestId: 'request-1',
      retryable: false,
      details: { currentVersion: 3 },
      status: 409,
    })
  })

  it('accepts a legacy envelope and derives safe fallback metadata', () => {
    expect(
      parseAPIError(
        {
          code: 'WORKFLOW_DRAFT_EXISTS',
          message: 'Release workflow already has a draft',
          requestId: 'request-2',
        },
        409,
      ),
    ).toMatchObject({
      code: 'WORKFLOW_DRAFT_EXISTS',
      category: 'conflict',
      requestId: 'request-2',
      retryable: false,
      status: 409,
    })
  })

  it('parses the generated client response shape', () => {
    expect(
      parseAPIErrorResponse({
        status: 401,
        data: {
          code: 'SESSION_INVALID',
          message: 'Authentication session is invalid',
          requestId: 'request-3',
        },
      }),
    ).toMatchObject({
      code: 'SESSION_INVALID',
      category: 'authentication',
      status: 401,
    })
  })

  it.each([
    { status: 503, category: 'dependency', retryable: true },
    { status: 429, category: 'rate_limit', retryable: true },
    { status: 404, category: 'not_found', retryable: false },
    { status: 422, category: 'validation', retryable: false },
    { status: 500, category: 'internal', retryable: false },
  ] as const)(
    'derives $category for malformed status $status responses',
    ({ status, category, retryable }) => {
      expect(parseAPIError('upstream response', status)).toEqual({
        code: 'UNKNOWN_ERROR',
        category,
        message: '',
        requestId: '',
        retryable,
        status,
      })
    },
  )
})

describe('resolveAPIErrorFeedback', () => {
  const options = {
    byCode: { WORKFLOW_VERSION_CONFLICT: '版本已變動' },
    byCategory: { conflict: '操作發生衝突' },
    fallback: '操作失敗',
  }

  it('prefers a stable code mapping over the category fallback', () => {
    const error = parseAPIError(
      { code: 'WORKFLOW_VERSION_CONFLICT', message: 'ignored' },
      409,
    )
    expect(resolveAPIErrorFeedback(error, options)).toBe('版本已變動')
  })

  it('uses category fallback for an unknown code without parsing message', () => {
    const first = parseAPIError(
      { code: 'FUTURE_CONFLICT', message: 'first internal wording' },
      409,
    )
    const second = parseAPIError(
      { code: 'FUTURE_CONFLICT', message: 'different wording' },
      409,
    )
    expect(resolveAPIErrorFeedback(first, options)).toBe('操作發生衝突')
    expect(resolveAPIErrorFeedback(second, options)).toBe('操作發生衝突')
  })

  it('uses the generic fallback for a transport failure', () => {
    expect(resolveAPIErrorFeedback(parseAPIError(undefined), options)).toBe(
      '操作失敗',
    )
  })
})

export const apiErrorCategories = [
  'authentication',
  'authorization',
  'validation',
  'not_found',
  'conflict',
  'rate_limit',
  'dependency',
  'internal',
] as const

export type APIErrorCategory = (typeof apiErrorCategories)[number]

export interface APIErrorView {
  code: string
  category: APIErrorCategory
  message: string
  requestId: string
  retryable: boolean
  status?: number
  details?: Readonly<Record<string, unknown>>
}

export interface APIErrorFeedbackOptions {
  byCode?: Readonly<Record<string, string>>
  byCategory?: Partial<Readonly<Record<APIErrorCategory, string>>>
  fallback: string
}

export function parseAPIError(data: unknown, status?: number): APIErrorView {
  const envelope = isRecord(data) ? data : undefined
  const category =
    readCategory(envelope?.category) ?? categoryFromStatus(status)

  return {
    code: readString(envelope?.code) || 'UNKNOWN_ERROR',
    category,
    message: readString(envelope?.message),
    requestId: readString(envelope?.requestId),
    retryable:
      typeof envelope?.retryable === 'boolean'
        ? envelope.retryable
        : category === 'dependency' || category === 'rate_limit',
    ...(status === undefined ? {} : { status }),
    ...(isRecord(envelope?.details) ? { details: envelope.details } : {}),
  }
}

export function parseAPIErrorResponse(response: unknown): APIErrorView {
  if (!isRecord(response)) return parseAPIError(undefined)
  return parseAPIError(
    response.data,
    typeof response.status === 'number' ? response.status : undefined,
  )
}

export function resolveAPIErrorFeedback(
  error: APIErrorView,
  options: APIErrorFeedbackOptions,
) {
  return (
    options.byCode?.[error.code] ??
    options.byCategory?.[error.category] ??
    options.fallback
  )
}

function categoryFromStatus(status?: number): APIErrorCategory {
  switch (status) {
    case 400:
    case 422:
      return 'validation'
    case 401:
      return 'authentication'
    case 403:
      return 'authorization'
    case 404:
      return 'not_found'
    case 409:
      return 'conflict'
    case 429:
      return 'rate_limit'
    case 502:
    case 503:
    case 504:
      return 'dependency'
    default:
      return 'internal'
  }
}

function readCategory(value: unknown): APIErrorCategory | undefined {
  return isAPIErrorCategory(value) ? value : undefined
}

function isAPIErrorCategory(value: unknown): value is APIErrorCategory {
  return apiErrorCategories.some((category) => category === value)
}

function readString(value: unknown) {
  return typeof value === 'string' ? value : ''
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

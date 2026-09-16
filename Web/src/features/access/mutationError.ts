import type { APIErrorView } from '@/shared/api/apiError'

export function accessMutationErrorKey(error: APIErrorView): string {
  if (error.status === 401) return 'access.mutation.unauthenticated'
  if (error.status === 400) return 'access.mutation.invalid'
  if (error.status === 404) return 'access.mutation.notAuthorized'
  if (error.code === 'ACCESS_SELF_DISABLE_FORBIDDEN')
    return 'access.mutation.selfDisable'
  if (error.code === 'ACCESS_LAST_PLATFORM_MANAGER')
    return 'access.mutation.lastManager'
  if (error.code === 'ACCESS_RESOURCE_PROTECTED')
    return 'access.mutation.protected'
  if (error.status === 409) return 'access.mutation.conflict'
  return 'access.mutation.error'
}

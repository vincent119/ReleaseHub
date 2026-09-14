import type { ListAccessMembershipCandidatesParams } from '@/generated/model'

export function membershipCandidateParams(
  search: string,
  cursor?: string,
): ListAccessMembershipCandidatesParams {
  return { query: search.trim() || undefined, cursor, limit: 20 }
}

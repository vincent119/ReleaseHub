import { describe, expect, it } from 'vitest'

import { membershipCandidateParams } from './membershipCandidates'

describe('membershipCandidateParams', () => {
  it('loads the first candidate page without requiring a search term', () => {
    expect(membershipCandidateParams('')).toEqual({
      query: undefined,
      cursor: undefined,
      limit: 20,
    })
  })

  it('preserves optional filtering and cursor continuation', () => {
    expect(membershipCandidateParams('  amy ', 'next')).toEqual({
      query: 'amy',
      cursor: 'next',
      limit: 20,
    })
  })
})

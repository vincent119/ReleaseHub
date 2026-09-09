import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  parseSidebarCollapsed,
  readSidebarCollapsed,
  SIDEBAR_STORAGE_KEY,
  useSidebarPreference,
  writeSidebarCollapsed,
} from './useSidebarPreference'

describe('Sidebar preference parsing', () => {
  it.each([
    ['true', true],
    ['false', false],
    [null, false],
    ['invalid', false],
  ])('parses %s as %s', (stored, expected) => {
    expect(parseSidebarCollapsed(stored)).toBe(expected)
  })

  it('falls back to expanded when storage cannot be read', () => {
    expect(
      readSidebarCollapsed({
        getItem: () => {
          throw new Error('storage unavailable')
        },
      }),
    ).toBe(false)
  })

  it('does not throw when storage cannot be written', () => {
    expect(() =>
      writeSidebarCollapsed(
        {
          setItem: () => {
            throw new Error('storage unavailable')
          },
        },
        true,
      ),
    ).not.toThrow()
  })
})

describe('useSidebarPreference', () => {
  beforeEach(() => localStorage.clear())

  it('restores and persists the desktop collapsed state', () => {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, 'true')
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    const { result } = renderHook(() => useSidebarPreference())

    expect(result.current.collapsed).toBe(true)

    act(() => result.current.setCollapsed(false))

    expect(result.current.collapsed).toBe(false)
    expect(setItem).toHaveBeenLastCalledWith(SIDEBAR_STORAGE_KEY, 'false')
  })
})

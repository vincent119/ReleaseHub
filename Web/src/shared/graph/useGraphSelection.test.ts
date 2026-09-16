import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { useGraphSelection } from './useGraphSelection'

afterEach(cleanup)

describe('useGraphSelection', () => {
  it('keeps node and edge selection mutually exclusive', () => {
    const { result } = renderHook(() => useGraphSelection())

    act(() => result.current.selectNode('node-a'))
    expect(result.current.selectedNodeID).toBe('node-a')
    expect(result.current.selectedEdgeID).toBeUndefined()

    act(() => result.current.selectEdge('edge-a'))
    expect(result.current.selectedNodeID).toBeUndefined()
    expect(result.current.selectedEdgeID).toBe('edge-a')

    act(() => result.current.clearSelection())
    expect(result.current.selectedNodeID).toBeUndefined()
    expect(result.current.selectedEdgeID).toBeUndefined()
  })
})

import { describe, expect, it } from 'vitest'

import {
  initialRequestListCriteria,
  initialRequestListPosition,
  requestListCriteriaReducer,
  requestListPositionReducer,
} from './listQuery'

describe('部署申請查詢狀態', () => {
  it('預設 20 筆、只 trim 名稱，清除條件保留筆數', () => {
    let state = requestListCriteriaReducer(initialRequestListCriteria, {
      type: 'limit',
      value: 50,
    })
    state = requestListCriteriaReducer(state, {
      type: 'search',
      value: '  服務 %_\\  ',
    })
    state = requestListCriteriaReducer(state, {
      type: 'status',
      value: 'Succeeded',
    })
    expect(state).toEqual({
      search: '服務 %_\\',
      status: 'Succeeded',
      limit: 50,
    })
    expect(requestListCriteriaReducer(state, { type: 'clear' })).toEqual({
      search: '',
      limit: 50,
    })
    expect(initialRequestListCriteria).toEqual({ search: '', limit: 20 })
  })

  it('只保存 Server 游標，上一頁可再走向已知下一頁', () => {
    let state = requestListPositionReducer(initialRequestListPosition, {
      type: 'next',
      cursor: 'server-2',
    })
    state = requestListPositionReducer(state, {
      type: 'next',
      cursor: 'server-3',
    })
    expect(state.index).toBe(2)
    state = requestListPositionReducer(state, { type: 'previous' })
    expect(state.index).toBe(1)
    state = requestListPositionReducer(state, {
      type: 'next',
      cursor: 'server-3',
    })
    expect(state.index).toBe(2)
    expect(state.cursors).toEqual([undefined, 'server-2', 'server-3'])
    expect(
      requestListPositionReducer(state, { type: 'next', cursor: 'server-2' }),
    ).toBe(state)
    expect(
      requestListPositionReducer(state, { type: 'next', cursor: '' }),
    ).toBe(state)
  })

  it('重新整理清除位置並換世代，不保留舊頁游標', () => {
    const state = requestListPositionReducer(initialRequestListPosition, {
      type: 'next',
      cursor: 'opaque',
    })
    const fresh = requestListPositionReducer(state, {
      type: 'refresh',
      updated: true,
    })
    expect(fresh).toEqual({
      cursors: [undefined],
      index: 0,
      generation: 1,
      updated: true,
    })
    expect(requestListPositionReducer(fresh, { type: 'previous' }).index).toBe(
      0,
    )
    expect(
      requestListPositionReducer(fresh, { type: 'refresh', updated: false })
        .generation,
    ).toBe(2)
    expect(state.cursors).toEqual([undefined, 'opaque'])
  })
})

import { describe, expect, it } from 'vitest'

import { definitionFitViewOptions, definitionZoomRange } from './graphViewport'

describe('definition Graph viewport contract', () => {
  it('keeps representative nodes readable while preserving pan and zoom', () => {
    expect(definitionFitViewOptions).toEqual({
      padding: 0.16,
      minZoom: 0.68,
      maxZoom: 1.1,
    })
    expect(definitionZoomRange).toEqual({ min: 0.35, max: 1.5 })
  })
})

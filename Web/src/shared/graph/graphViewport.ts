import type { FitViewOptions } from '@xyflow/react'

export const definitionFitViewOptions = {
  padding: 0.16,
  minZoom: 0.68,
  maxZoom: 1.1,
} as const satisfies FitViewOptions

export const definitionZoomRange = { min: 0.35, max: 1.5 } as const
